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
	// with [Loader.Register] once Load had started.
	ErrRegisteredAfterLoad = errors.New("config registered after Load started; register every config before loading")
)

// Loader loads a set of configs together: configs are registered with
// [Loader.Register], loaded with [Loader.Load], and read with [Handle.Value].
//
// A Loader is safe for concurrent use and holds no lock while user code runs:
// SetDefaults, Validate, unmarshalers and the dump writer may call
// Register on that Loader or Value on its handles. They must not call Load
// recursively: it would wait for the load already in progress.
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

// registration is one Register call: the compiled config, or why it could not be
// compiled, and its load callback. typeName identifies a registration whose
// name could not be resolved, so its error still sorts to a stable place.
type registration struct {
	descriptor
	typeName string
	name     string
	err      error
	fill     func(any, environment, *ConfigReport) error
}

// descriptor carries immutable schema and a factory for fresh defaults.
// Loading and manifest generation use independent instances from that factory.
type descriptor struct {
	instanceName string
	prefix       string
	fields       []fieldSpec
	defaults     func() any
}

// MakeLoader returns an empty [Loader]. opts configure the whole load: [WithEnv],
// [WithDump], [AllowUnknown], [WithDiagnostics] and [WithDiagnosticHandler].
// A nil option is reported by [Loader.Load].
func MakeLoader(opts ...EnvOption) *Loader {
	l := &Loader{}
	for _, opt := range opts {
		if opt == nil {
			l.envErr = makeConfigError(ErrorDeclaration, "", "", errors.New("a load option must not be nil"))
			continue
		}

		opt.applyEnv(&l.env)
	}

	if l.env.diagnostics != nil {
		if err := l.env.diagnostics.bindLoader(l); err != nil {
			l.envErr = errors.Join(l.envErr, err)
			l.env.diagnostics = nil
		}
	}
	l.envErr = errors.Join(l.envErr, l.env.diagnosticErr)

	return l
}

// Register registers a struct config of type T and returns its [Handle].
// The required name labels errors and determines the default ENV prefix:
// "read-replica" reads READ_REPLICA_*. [WithPrefix] overrides only the prefix.
// Names must be non-empty, contain only lowercase letters, digits, _ - .,
// and start and end with a letter or digit.
//
// An invalid registration, such as an empty name or bad tag, is reported by
// [Loader.Load] or [Loader.Manifest]. Once Load has started, Register
// registers nothing and calls no method of T; the handle's Value returns
// [ErrRegisteredAfterLoad].
func (l *Loader) Register[T any](name string, opts ...ConfigOption) *Handle[T] {
	// A late registration runs no user code.
	l.mu.Lock()
	open := l.state == registering
	l.mu.Unlock()
	if !open {
		return &Handle[T]{err: ErrRegisteredAfterLoad}
	}

	// Resolve the registration without holding the lock.
	r := &registration{typeName: reflect.TypeFor[T]().String(), name: name}
	r.descriptor, r.err = makeDescriptor[T](name, opts)
	// Handles, including copies, share typed storage. Value exposes it only
	// after Load publishes completion under the loader mutex.
	value := new(T)
	if r.err == nil {
		r.fill = func(defaults any, env environment, report *ConfigReport) error {
			cfg := defaults.(*T)
			if err := applyAndValidate(cfg, r.fields, r.instanceName, env, report); err != nil {
				return err
			}

			*value = *cfg

			return nil
		}
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	// Load may have started while the registration was resolved.
	if l.state != registering {
		return &Handle[T]{err: ErrRegisteredAfterLoad}
	}

	l.registrations = append(l.registrations, r)

	return &Handle[T]{loader: l, value: value, name: name}
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

	sortRegistrations(registrations)
	var report *LoadReport
	if l.env.diagnostics != nil || l.env.diagnosticHandler != nil {
		report = makeLoadReport(registrations)
		if l.env.diagnostics != nil {
			l.env.diagnostics.publishReport(LoadReport{State: LoadInProgress})
		}
	}

	err := errLoadPanicked
	defer func() {
		if report != nil {
			setLoadReportState(report, err)
		}
		l.mu.Lock()
		l.err = err
		l.state = loaded
		if report != nil && l.env.diagnostics != nil {
			l.env.diagnostics.publishReport(*report)
		}
		close(l.done)
		l.mu.Unlock()
		if report != nil && err != errLoadPanicked && l.env.diagnosticHandler != nil {
			l.env.diagnosticHandler(cloneLoadReport(*report))
		}
	}()

	err = l.load(registrations, report)

	return err
}

// Load loads one config of type T, as a [Loader] with a single registration would.
// The required name determines the default ENV prefix. Options include
// [ConfigOption] ([WithPrefix]) and [EnvOption]
// ([WithEnv], [WithDump], [AllowUnknown], [WithDiagnostics],
// [WithDiagnosticHandler]). Unknown variables are looked for under
// T's own prefix only. On error it returns the zero T.
//
//	cfg, err := confmaker.Load[store.Config]("store")
//	cfg, err := confmaker.Load[store.Config]("replica", confmaker.WithEnv(vars))
func Load[T any](name string, opts ...LoadOption) (T, error) {
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
	handle := loader.Register[T](name, configOpts...)

	if err := loader.Load(); err != nil {
		var cfg T
		return cfg, err
	}

	return handle.Value()
}

// Handle is a config registered with [Loader.Register], read with [Handle.Value]. The
// zero Handle belongs to no loader, and its Value returns an error.
type Handle[T any] struct {
	loader *Loader
	value  *T
	name   string
	err    error
}

// Name returns the explicit registration name, without reading ENV or loading.
// A nil or zero handle has no name.
func (h *Handle[T]) Name() string {
	if h == nil {
		return ""
	}
	return h.name
}

// BelongsTo reports whether this handle was registered with loader. Nil, zero
// and rejected late handles belong to no loader. This does not load or validate.
func (h *Handle[T]) BelongsTo(loader *Loader) bool {
	return h != nil && loader != nil && h.loader == loader
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
		return cfg, errors.New("handle belongs to no loader; get one from Loader.Register")
	}

	h.loader.mu.Lock()
	defer h.loader.mu.Unlock()

	switch {
	case h.loader.state != loaded:
		return cfg, ErrNotLoaded
	case h.loader.err != nil:
		return cfg, h.loader.err
	}

	return *h.value, nil
}

// makeDescriptor compiles T's schema and binds its defaults factory.
// Defaults are not evaluated until the factory is called.
func makeDescriptor[T any](name string, opts []ConfigOption) (descriptor, error) {
	set, fields, err := prepareConfig[T](name, opts)
	if err != nil {
		return descriptor{}, err
	}

	return descriptor{instanceName: set.name, prefix: set.prefix, fields: fields,
		defaults: func() any {
			cfg := new(T)
			setDefaults(cfg)

			return cfg
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
func (l *Loader) load(registrations []*registration, report *LoadReport) error {
	var errs []error
	if report != nil {
		defer func() {
			for _, err := range errs {
				recordLoadProblems(report, err)
			}
		}()
	}
	if l.envErr != nil {
		errs = append(errs, l.envErr)
	}

	if l.env.envRepeated {
		errs = append(errs, makeConfigError(ErrorDeclaration, "", "", errors.New("WithEnv is given more than once; pass one complete environment")))
	}

	ready := make([]*registration, 0, len(registrations))
	var readyReports []*ConfigReport
	for index, r := range registrations {
		if r.err != nil {
			errs = append(errs, r.err)
			continue
		}

		ready = append(ready, r)
		if report != nil {
			readyReports = append(readyReports, &report.Configs[index])
		}
	}

	descriptors := make([]descriptor, len(ready))
	for i, r := range ready {
		descriptors[i] = r.descriptor
	}

	known, err := checkRegistrations(descriptors)
	if err != nil {
		errs = append(errs, err)
		return errors.Join(errs...)
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
		var configReport *ConfigReport
		if report != nil {
			configReport = readyReports[i]
		}
		err := r.fill(configs[i], env, configReport)
		if configReport != nil {
			configReport.Status = ConfigSucceeded
			if err != nil {
				configReport.Status = ConfigFailed
			}
		}
		if err != nil {
			errs = append(errs, err)
		}
	}

	return errors.Join(errs...)
}

// sortRegistrations orders a private snapshot without changing the loader's list.
func sortRegistrations(registrations []*registration) {
	// Sort every registration, failed ones included, so the report does not
	// depend on the order of Register calls. The errors themselves are kept as they
	// are for errors.Is and errors.As.
	slices.SortStableFunc(registrations, func(a, b *registration) int {
		return cmp.Or(
			cmp.Compare(a.instanceName, b.instanceName),
			cmp.Compare(a.prefix, b.prefix),
			cmp.Compare(a.typeName, b.typeName),
			cmp.Compare(errorText(a.err), errorText(b.err)),
		)
	})
}

func errorText(err error) string {
	if err == nil {
		return ""
	}

	return err.Error()
}
