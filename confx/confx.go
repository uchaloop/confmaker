// Package confx wires configuration into Uber Fx. For every instance a library
// needs, it builds that library's own typed config from the environment,
// validates it, and provides it into the container - untagged for the single
// default instance, under a name tag for additional ones - so a library's Fx
// module consumes a ready config without touching the environment itself.
//
// A library declares its config as a plain struct with `env` tags:
//
//	type Config struct {
//		Host     string        `env:"HOST"`
//		Database string        `env:"DATABASE"`
//		Password secret.Secret `env:"PASSWORD,required"`
//	}
//
// The tags are inert strings, so the library depends only on the secret type
// (github.com/uchaloop/secret/v2), not on this package. The instance name gives
// the prefix, so Provide[Config]("postgres") reads POSTGRES_HOST,
// POSTGRES_PASSWORD and so on; a field without an env tag stays at its zero
// value, which is where a library keeps its defaults.
package confx

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/caarlos0/env/v11"
	"github.com/uchaloop/utilfx"
	"go.uber.org/fx"
)

// settings holds the resolved options for a single Provide call.
type settings struct {
	prefix string
}

// Option customizes how a config is built.
type Option func(*settings)

// WithPrefix overrides the environment-variable prefix derived from the instance
// name. Pass an empty string to read env tags with no prefix at all (shared,
// global variables); such an instance owns no prefix of its own and so is left
// out of Module's strict check, which would otherwise claim the whole
// environment.
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
	prefix, label := resolve(name, opts)

	return fx.Options(
		fx.Provide(build[T](prefix, label)),
		provideDescriptor[T](prefix, label),
	)
}

// ProvideNamed is Provide for an additional instance: it provides the config
// tagged name:"<name>" and derives the env prefix from the same name. Use it for
// replicas and shards, e.g. ProvideNamed[Config]("replica") reads variables such
// as REPLICA_HOST.
func ProvideNamed[T any](name string, opts ...Option) fx.Option {
	prefix, label := resolve(name, opts)

	return fx.Options(
		fx.Provide(
			fx.Annotate(
				build[T](prefix, label),
				fx.ResultTags(utilfx.NameTag(name)),
			),
		),
		provideDescriptor[T](prefix, label),
	)
}

// provideDescriptor feeds the manifest of one instance into the group Module
// reads. Fx builds a group member only when the group is consumed, so this costs
// nothing in an application that does not register Module.
func provideDescriptor[T any](prefix, label string) fx.Option {
	return fx.Provide(
		fx.Annotate(
			func() descriptor {
				variables, open := manifest(reflect.TypeFor[T](), prefix)

				return descriptor{
					label:     label,
					prefix:    prefix,
					variables: variables,
					open:      open,
				}
			},
			fx.ResultTags(descriptorGroup),
		),
	)
}

// resolve applies opts over the prefix derived from name and returns the prefix
// to read with and the label to name the instance in errors.
func resolve(name string, opts []Option) (prefix, label string) {
	set := settings{prefix: defaultPrefix(name)}
	for _, opt := range opts {
		opt(&set)
	}

	return set.prefix, name
}

// build returns the constructor that fills a T's env-tagged fields with the
// given prefix and validates the result. label names the instance in errors.
func build[T any](prefix, label string) func() (T, error) {
	return func() (T, error) {
		var cfg T

		if err := fillEnv(&cfg, prefix, label); err != nil {
			return cfg, err
		}

		return cfg, nil
	}
}

// fillEnv fills the env-tagged fields of cfg with the given prefix and validates
// it if T has a Validate method.
func fillEnv[T any](cfg *T, prefix, label string) error {
	if err := env.ParseWithOptions(cfg, env.Options{Prefix: prefix}); err != nil {
		return cleanEnvError(err)
	}
	// Check cfg (a *T), not *cfg: *T's method set includes both value- and
	// pointer-receiver Validate methods, so a library that declares Validate on a
	// pointer receiver is still validated.
	if v, ok := any(cfg).(interface{ Validate() error }); ok {
		if err := v.Validate(); err != nil {
			return fmt.Errorf("config %q: %w", label, err)
		}
	}

	return nil
}

// defaultPrefix turns an instance name into an env prefix: "main" -> "MAIN_",
// "read-replica" -> "READ_REPLICA_", "db.postgres" -> "DB_POSTGRES_".
func defaultPrefix(name string) string {
	return strings.ToUpper(strings.NewReplacer("-", "_", ".", "_").Replace(name)) + "_"
}

// cleanEnvError strips the "env: " prefix the env parser puts on its messages so
// the error states only the problem (the caller adds package context). Missing
// required variables are named without a value; conversion errors for ordinary,
// non-secret fields may include their source value.
func cleanEnvError(err error) error {
	return fmt.Errorf("resolve environment config: %s", strings.TrimPrefix(err.Error(), "env: "))
}
