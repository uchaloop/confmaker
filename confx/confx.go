package confx

import (
	"errors"
	"fmt"
	"reflect"
	"strings"

	"go.uber.org/fx"
)

// settings holds the resolved options for a single Provide call.
type settings struct {
	prefix string
}

// Option customizes how a config is built.
type Option func(*settings)

// WithPrefix overrides the environment-variable prefix derived from the instance
// name, for when the two should not follow each other. The prefix is written the
// way the variables are - upper case, digits and underscores - and ends with the
// underscore that separates it from a field's own name.
func WithPrefix(prefix string) Option {
	return func(s *settings) {
		s.prefix = prefix
	}
}

// Provide builds a T from the environment, validates it if T has a Validate
// method, and provides it into the container untagged - the default single
// instance. The env prefix is derived from name, so Provide[Config]("store")
// reads variables such as STORE_HOST.
//
// Use it for the common one-instance case (paired with a no-argument library
// module); reach for ProvideNamed only when a second instance is needed.
func Provide[T any](name string, opts ...Option) fx.Option {
	return provide[T](name, ``, opts)
}

// ProvideNamed is Provide for an additional instance: it provides the config
// tagged name:"<name>" and derives the env prefix from the same name. Use it for
// replicas and shards, e.g. ProvideNamed[Config]("replica") reads variables such
// as REPLICA_HOST.
func ProvideNamed[T any](name string, opts ...Option) fx.Option {
	return provide[T](name, nameTag(name), opts)
}

// provide registers the constructor of one instance under the given result tag -
// empty for the default instance - together with the manifest of the variables
// it reads. Fx builds a group member only when the group is consumed, so the
// manifest costs nothing in an application without Module.
func provide[T any](name, tag string, opts []Option) fx.Option {
	prefix, err := resolvePrefix(name, opts)
	if err != nil {
		return fx.Error(err)
	}

	return fx.Options(
		fx.Provide(
			fx.Annotate(build[T](prefix, name), fx.ResultTags(tag)),
		),
		fx.Provide(
			fx.Annotate(describe[T](prefix, name), fx.ResultTags(descriptorGroup)),
		),
	)
}

// resolvePrefix returns the environment prefix an instance reads with: the one
// derived from its name, unless an option replaces it. Both the name and the
// resulting prefix are checked, because a prefix that does not look like one
// reads nothing and says nothing.
func resolvePrefix(name string, opts []Option) (string, error) {
	if err := checkName(name); err != nil {
		return "", err
	}

	set := settings{prefix: defaultPrefix(name)}
	for _, opt := range opts {
		opt(&set)
	}

	if err := checkPrefix(set.prefix); err != nil {
		return "", err
	}

	return set.prefix, nil
}

// build returns the constructor that fills a T's env-tagged fields with the
// given prefix and validates the result. name identifies the instance in errors.
func build[T any](prefix, name string) func() (T, error) {
	return func() (T, error) {
		var cfg T

		err := fillEnv(&cfg, prefix, name)

		return cfg, err
	}
}

// describe returns the constructor of one instance's manifest.
func describe[T any](prefix, name string) func() (descriptor, error) {
	return func() (descriptor, error) {
		variables, err := manifestOf[T](prefix)
		if err != nil {
			return descriptor{}, makeConfigError(name, err)
		}

		return descriptor{label: name, prefix: prefix, variables: variables}, nil
	}
}

// fillEnv builds cfg in three optional steps: the config establishes its own
// defaults, the environment overrides what it sets, and the result is validated.
// A variable that is not set leaves its field as the defaults left it.
func fillEnv[T any](cfg *T, prefix, name string) error {
	setDefaults(cfg)

	bindings, err := bind(reflect.ValueOf(cfg).Elem(), prefix)
	if err != nil {
		return makeConfigError(name, err)
	}
	if err := apply(bindings); err != nil {
		return makeConfigError(name, err)
	}
	// Check cfg (a *T), not *cfg: *T's method set includes both value- and
	// pointer-receiver Validate methods, so a library that declares Validate on a
	// pointer receiver is still validated.
	if v, ok := any(cfg).(interface{ Validate() error }); ok {
		if err := v.Validate(); err != nil {
			return makeConfigError(name, err)
		}
	}

	return nil
}

// configError attributes an error to the instance it came from. Every stage
// reports all of its problems at once, and a joined error renders one per line -
// so the name goes on each of them, not only on the first. A line that scrolls
// past on its own still says which config it is about, which matters when one
// process builds several.
//
// The name is put on the rendered lines rather than on the errors behind them,
// because a stage is free to wrap its join in context of its own: labelling the
// parts would report "config: max_conns must be positive" and lose the "pool:"
// that said where in the config it is.
type configError struct {
	name string
	err  error
}

func makeConfigError(name string, err error) error {
	return configError{name: name, err: err}
}

func (e configError) Error() string {
	lines := strings.Split(e.err.Error(), "\n")
	for i, line := range lines {
		lines[i] = fmt.Sprintf("config %q: %s", e.name, line)
	}

	return strings.Join(lines, "\n")
}

// Unwrap keeps errors.Is and errors.As reaching what the stage reported, each
// error inside a join included.
func (e configError) Unwrap() error { return e.err }

// checkName rejects an instance name that would not make a sensible prefix or a
// sensible Fx tag. The name is used for both, so one written two ways - or with
// a space in it, which no variable can carry - would silently read nothing and
// name a dependency nobody asks for.
func checkName(name string) error {
	if len(name) == 0 {
		return errors.New("an instance name is required; it gives both the variable prefix and the Fx tag")
	}

	for _, r := range name {
		valid := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' || r == '-' || r == '.'
		if !valid {
			return fmt.Errorf(
				"instance name %q may hold only lowercase letters, digits, and _ - . (found %q)",
				name, r,
			)
		}
	}

	if strings.ContainsAny(name[:1]+name[len(name)-1:], "_-.") {
		return fmt.Errorf("instance name %q may not start or end with a separator", name)
	}

	return nil
}

// checkPrefix rejects a prefix that would not read the variables it is meant to.
// An empty one would claim every variable in the environment and could never be
// checked for typos; one that does not end in an underscore would run into the
// field's own name, turning HOST into REPORTINGHOST.
func checkPrefix(prefix string) error {
	if len(prefix) == 0 {
		return errors.New("an environment prefix is required; every variable an application reads is prefixed")
	}

	for _, r := range prefix {
		if valid := (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_'; !valid {
			return fmt.Errorf(
				"environment prefix %q may hold only upper-case letters, digits, and _ (found %q)",
				prefix, r,
			)
		}
	}

	if !strings.HasSuffix(prefix, "_") {
		return fmt.Errorf("environment prefix %q must end with _, which separates it from the field's own name", prefix)
	}

	return nil
}

// defaultPrefix turns an instance name into an env prefix. A variable is written
// with underscores whatever the name uses, so "main" -> "MAIN_",
// "read-replica" -> "READ_REPLICA_", "db.main" -> "DB_MAIN_".
//
// The name is walked a byte at a time because checkName has already established
// its alphabet: lower-case letters, digits, and the three separators. That makes
// the mapping the whole rule rather than a replacer and a case fold, neither of
// which could say what a name is allowed to hold.
func defaultPrefix(name string) string {
	prefix := make([]byte, len(name)+1)

	for i := range len(name) {
		switch c := name[i]; {
		case c >= 'a' && c <= 'z':
			prefix[i] = c - ('a' - 'A')
		case c == '-' || c == '.':
			prefix[i] = '_'
		default:
			prefix[i] = c
		}
	}

	prefix[len(name)] = '_'

	return string(prefix)
}

// nameTag is the Fx tag of a named value. %q escapes the name, so a tag stays
// well formed whatever the instance is called.
func nameTag(name string) string {
	return fmt.Sprintf(`name:%q`, name)
}
