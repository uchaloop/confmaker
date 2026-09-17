package confmaker

import (
	"cmp"
	"errors"
	"reflect"
	"slices"
	"sync"
)

var (
	// ErrNotLoaded is returned by [Handle.Value] until [Loader.Load] has finished.
	ErrNotLoaded = errors.New("configuration is not loaded; call Load first")
	// ErrRegisteredAfterLoad is returned by [Handle.Value] for a config added
	// with [Loader.Add] once Load had started.
	ErrRegisteredAfterLoad = errors.New("config registered after Load started; register every config before loading")
)

// Loader loads a set of configs together: configs are registered with
// [Loader.Add], loaded with [Loader.Load], and read with [Handle.Value].
//
// A Loader is safe for concurrent use and holds no lock while user code runs:
// ConfigName, SetDefaults, Validate, unmarshalers and the dump writer may call
// Add or Value on the same Loader. They must not call its Load, which would wait
// for the load it is part of.
type Loader struct {
	env    envSettings
	envErr error

	mu            sync.Mutex
	state         loaderState
	registrations []*registration
	done          chan struct{}
	err           error
}

// loaderState is where a Loader is in its one pass: registrations are accepted
// until Load starts, values are published once it finishes.
type loaderState int

const (
	registering loaderState = iota
	loading
	loaded
)

// errLoadPanicked is the result of a load that user code panicked out of. The
// panic itself propagates from the Load call that ran it.
var errLoadPanicked = errors.New("configuration loading panicked")

// registration is one Add call: the compiled config, or why it could not be
// compiled, and the value once loaded. typeName identifies a registration whose
// name could not be resolved, so its error still sorts to a stable place.
type registration struct {
	descriptor
	typeName string
	err      error
	value    any
}

// descriptor carries a registration's schema and the two steps of loading it.
// defaults returns a fresh *T after SetDefaults; fill applies the environment to
// that pointer, validates it and returns the T. Between the two a dump can
// describe the defaults of the very instance that is loaded.
type descriptor struct {
	label    string
	prefix   string
	fields   []fieldSpec
	defaults func() any
	fill     func(any, environment) (any, error)
}

// MakeLoader returns an empty [Loader]. opts configure the whole load: [WithEnv],
// [WithDump], [AllowUnknown]. A nil option is reported by [Loader.Load].
func MakeLoader(opts ...EnvOption) *Loader {
	l := &Loader{}
	for _, opt := range opts {
		if opt == nil {
			l.envErr = errors.New("a load option must not be nil")
			continue
		}

		opt.applyEnv(&l.env)
	}

	return l
}

// Add registers a config of type T and returns the [Handle] to read it from. The
// instance name comes from [WithName] or T's ConfigName method.
//
// An invalid registration - no name, a bad tag, a secret inside a JSON value - is
// reported by [Loader.Load] with every other problem. Once Load has started, Add
// registers nothing and calls no method of T; the handle's Value returns
// [ErrRegisteredAfterLoad].
func (l *Loader) Add[T any](opts ...ConfigOption) *Handle[T] {
	// A late registration runs no user code, not even ConfigName. The lock is
	// released before ConfigName runs.
	l.mu.Lock()
	open := l.state == registering
	l.mu.Unlock()
	if !open {
		return &Handle[T]{err: ErrRegisteredAfterLoad}
	}

	// ConfigName is user code: resolve the registration without the lock.
	r := &registration{typeName: reflect.TypeFor[T]().String()}
	set, err := resolveSettings[T](opts)
	if err == nil {
		r.descriptor, err = makeDescriptor[T](set)
	}

	r.err = err

	l.mu.Lock()
	defer l.mu.Unlock()

	// Load may have started while the registration was resolved.
	if l.state != registering {
		return &Handle[T]{err: ErrRegisteredAfterLoad}
	}

	l.registrations = append(l.registrations, r)

	return &Handle[T]{loader: l, registration: r}
}

// Load loads every registered config and returns every problem as one error:
// invalid options and registrations, conflicts between registrations, unknown
// variables, variables that did not parse, and the Validate errors of every
// config whose variables parsed. The steps are listed in the package
// documentation.
//
// Load runs once. Calling it again returns the same error, or nil, without
// reading the environment again; a call made while another goroutine loads waits
// for that result. If user code panics during the load, the panic propagates from
// the Load that ran it, and later calls return an error.
func (l *Loader) Load() error {
	l.mu.Lock()
	switch l.state {
	case loaded:
		defer l.mu.Unlock()
		return l.err
	case loading:
		done := l.done
		l.mu.Unlock()
		<-done

		l.mu.Lock()
		defer l.mu.Unlock()

		return l.err
	}

	l.state = loading
	l.done = make(chan struct{})
	registrations := slices.Clone(l.registrations)
	l.mu.Unlock()

	err := errLoadPanicked
	defer func() {
		l.mu.Lock()
		defer l.mu.Unlock()

		l.err = err
		l.state = loaded
		close(l.done)
	}()

	err = l.load(registrations)

	return err
}

// Load loads one config of type T, as a [Loader] with a single registration would.
// It takes both a [ConfigOption] ([WithName], [WithPrefix]) and an [EnvOption]
// ([WithEnv], [WithDump], [AllowUnknown]). Unknown variables are looked for under
// T's own prefix only. On error it returns the zero T.
//
//	cfg, err := confmaker.Load[store.Config]()
//	cfg, err := confmaker.Load[store.Config](confmaker.WithName("replica"), confmaker.WithEnv(vars))
func Load[T any](opts ...LoadOption) (T, error) {
	var (
		configOpts []ConfigOption
		envOpts    []EnvOption
	)

	for _, opt := range opts {
		switch opt := opt.(type) {
		case ConfigOption:
			configOpts = append(configOpts, opt)
		case EnvOption:
			envOpts = append(envOpts, opt)
		default:
			// Only nil gets here: the kinds are sealed. MakeLoader reports it.
			envOpts = append(envOpts, nil)
		}
	}

	loader := MakeLoader(envOpts...)
	handle := loader.Add[T](configOpts...)

	if err := loader.Load(); err != nil {
		var cfg T
		return cfg, err
	}

	return handle.Value()
}

// Handle is a config registered with [Loader.Add], read with [Handle.Value]. The
// zero Handle belongs to no loader, and its Value returns an error.
type Handle[T any] struct {
	loader       *Loader
	registration *registration
	err          error
}

// Value returns the loaded config, or an error:
//
//   - [ErrNotLoaded] until [Loader.Load] has finished;
//   - the error Load returned, when any config of the set failed - even if this
//     config loaded on its own;
//   - [ErrRegisteredAfterLoad] for a config added once Load had started.
//
// The struct is returned by value, but maps, slices and pointers inside it are
// shared by every caller. Treat a loaded config as read-only.
func (h *Handle[T]) Value() (T, error) {
	var cfg T
	switch {
	case h.err != nil:
		return cfg, h.err
	case h.loader == nil:
		return cfg, errors.New("handle belongs to no loader; get one from Loader.Add")
	}

	h.loader.mu.Lock()
	defer h.loader.mu.Unlock()

	switch {
	case h.loader.state != loaded:
		return cfg, ErrNotLoaded
	case h.loader.err != nil:
		return cfg, h.loader.err
	}

	return h.registration.value.(T), nil
}

// makeDescriptor compiles T's schema and binds the functions that load it.
// Nothing is evaluated until one of them is called.
func makeDescriptor[T any](set configSettings) (descriptor, error) {
	fields, err := compileSchema(reflect.TypeFor[T](), set.prefix)
	if err != nil {
		return descriptor{}, makeConfigError(set.name, err)
	}

	return descriptor{label: set.name, prefix: set.prefix, fields: fields,
		defaults: func() any {
			cfg := new(T)
			setDefaults(cfg)

			return cfg
		},
		fill: func(ptr any, env environment) (any, error) {
			cfg := ptr.(*T)
			err := applyAndValidate(cfg, fields, set.name, env)

			return *cfg, err
		},
	}, nil
}

// load fills every valid registration from one environment snapshot. It runs
// without the lock, so user code in it may use the Loader. Invalid options and
// invalid registrations are reported, and the other configs still load, so one
// bad tag does not hide another config's missing variable. Registrations in
// conflict end loading before SetDefaults and the dump: no value could be
// attributed. Otherwise each config is given its defaults once, the dump
// describes that same instance, and a failing dump is one more problem in the
// report rather than a reason to skip the checks.
func (l *Loader) load(registrations []*registration) error {
	var errs []error
	if l.envErr != nil {
		errs = append(errs, l.envErr)
	}

	if l.env.envRepeated {
		errs = append(errs, errors.New("WithEnv is given more than once; pass one complete environment"))
	}

	// Sort every registration, failed ones included, so the report does not
	// depend on the order of Add calls. The errors themselves are kept as they
	// are for errors.Is and errors.As.
	slices.SortStableFunc(registrations, func(a, b *registration) int {
		return cmp.Or(
			cmp.Compare(a.label, b.label),
			cmp.Compare(a.prefix, b.prefix),
			cmp.Compare(a.typeName, b.typeName),
			cmp.Compare(errorText(a.err), errorText(b.err)),
		)
	})

	ready := make([]*registration, 0, len(registrations))
	for _, r := range registrations {
		if r.err != nil {
			errs = append(errs, r.err)
			continue
		}

		ready = append(ready, r)
	}

	descriptors := make([]descriptor, len(ready))
	for i, r := range ready {
		descriptors[i] = r.descriptor
	}

	known, err := checkRegistrations(descriptors)
	if err != nil {
		return errors.Join(append(errs, err)...)
	}

	env := l.env.env
	if env == nil {
		env = osEnvironment()
	}

	configs := make([]any, len(ready))
	for i, r := range ready {
		configs[i] = r.defaults()
	}

	if l.env.dump != nil {
		errs = append(errs, dumpDefaults(l.env.dump, descriptors, configs, env)...)
	}

	if err := checkUnknown(descriptors, known, l.env.allowed, env); err != nil {
		errs = append(errs, err)
	}

	for i, r := range ready {
		value, err := r.fill(configs[i], env)
		if err != nil {
			errs = append(errs, err)
			continue
		}

		r.value = value
	}

	return errors.Join(errs...)
}

func errorText(err error) string {
	if err == nil {
		return ""
	}

	return err.Error()
}
