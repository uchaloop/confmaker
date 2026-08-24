// Package confx builds a library's typed config from the environment and
// provides it into an Uber Fx container, so the library consumes a ready config
// without reading the environment itself.
//
// A config is a plain struct with `env` tags. The tags are inert strings, so the
// library depends only on the secret type (github.com/uchaloop/secret/v2), not
// on this package:
//
//	type Config struct {
//		Host     string        `env:"HOST"`
//		Timeout  time.Duration `env:"TIMEOUT"`
//		Password secret.Secret `env:"PASSWORD,notEmpty"`
//	}
//
//	func (c *Config) SetDefaults() { c.Timeout = 30 * time.Second }
//
// The instance name gives the prefix, so Provide[Config]("postgres") reads
// POSTGRES_HOST and the rest. Building one runs three optional steps: SetDefaults
// establishes the config's own defaults, the environment overrides what it sets,
// and Validate checks the result. A variable that is not set leaves its field as
// SetDefaults left it; a variable that is set assigns it, an empty value
// included.
//
// Module adds the check for the environment: a variable under a prefix the
// application owns that matches no field fails the start. What a declaration
// itself can get wrong - a default in a tag, an unknown option, two fields on one
// variable, a type that cannot be read - is refused when the config is bound,
// before any value is looked at.
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
// instance. The env prefix is derived from name, so Provide[Config]("postgres")
// reads variables such as POSTGRES_HOST.
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
			return descriptor{}, configError(name, err)
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
		return configError(name, err)
	}
	if err := apply(bindings); err != nil {
		return configError(name, err)
	}
	// Check cfg (a *T), not *cfg: *T's method set includes both value- and
	// pointer-receiver Validate methods, so a library that declares Validate on a
	// pointer receiver is still validated.
	if v, ok := any(cfg).(interface{ Validate() error }); ok {
		if err := v.Validate(); err != nil {
			return configError(name, err)
		}
	}

	return nil
}

// configError attributes err to the instance it came from. Every stage reports
// all of its problems at once, and a joined error renders one per line - so the
// name goes on each of them, not only on the first. A line that scrolls past on
// its own still says which config it is about, which matters when one process
// builds several.
func configError(name string, err error) error {
	joined, ok := err.(interface{ Unwrap() []error })
	if !ok {
		return fmt.Errorf("config %q: %w", name, err)
	}

	parts := joined.Unwrap()
	if len(parts) == 0 {
		return fmt.Errorf("config %q: %w", name, err)
	}

	labelled := make([]error, len(parts))
	for i, part := range parts {
		labelled[i] = configError(name, part)
	}

	return errors.Join(labelled...)
}

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
// "read-replica" -> "READ_REPLICA_", "db.postgres" -> "DB_POSTGRES_".
func defaultPrefix(name string) string {
	return strings.ToUpper(strings.NewReplacer("-", "_", ".", "_").Replace(name)) + "_"
}

// nameTag is the Fx tag of a named value. %q escapes the name, so a tag stays
// well formed whatever the instance is called.
func nameTag(name string) string {
	return fmt.Sprintf(`name:%q`, name)
}
