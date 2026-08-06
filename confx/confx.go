// Package confx wires configuration into Uber Fx. It loads a TOML file once,
// then, for each named instance a library needs, decodes one section of that
// file into the library's own typed config and provides it into the container
// under a name tag - so a library's Fx module consumes a ready, validated config
// without ever touching the config or env-parsing stack itself.
//
// A library declares its config as a plain struct with two tag namespaces:
//
//	type Config struct {
//		Host     string        `koanf:"host" env:"HOST"`          // file, env overrides
//		Database string        `koanf:"database"`                 // file only
//		Password secret.Secret `koanf:"-"    env:"PASSWORD,required"` // env only
//	}
//
// Both tags are inert strings, so the library depends only on the secret type
// (github.com/uchaloop/secret/v2), not on this package. confx reads the file (koanf
// tags) and the environment (env tags) and hands the library the filled struct.
package confx

import (
	"fmt"
	"strings"

	"github.com/caarlos0/env/v11"
	"github.com/go-viper/mapstructure/v2"
	"github.com/knadh/koanf/parsers/toml/v2"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/v2"
	"github.com/uchaloop/confmaker/internal/dirconfig"
	"github.com/uchaloop/confmaker/internal/filedecode"
	"github.com/uchaloop/utilfx"
	"go.uber.org/fx"
)

// Source is the loaded configuration tree. LoadModule provides it into the
// container; Provide reads sections out of it.
type Source struct {
	k *koanf.Koanf
}

// LoadModule reads the given TOML files in order (later files override earlier
// ones) and provides the resulting Source into the container. With no paths, it
// reads config.toml from the current working directory. Pair it with one Provide
// call per named instance.
func LoadModule(paths ...string) fx.Option {
	return fx.Module(
		"confmaker",

		fx.Provide(
			func() (*Source, error) {
				return openSource(paths...)
			},
		),
	)
}

// LoadDir provides a Source using the confmaker directory convention.
//
// ENVIRONMENT is required and accepts dev, stage, prod, or prd (an alias for
// prod). An optional common.toml is loaded first; the required canonical
// environment file (dev.toml, stage.toml, or prod.toml) is loaded second and
// overrides common values. Pair it with ProvideDefault or Provide.
func LoadDir(dir string) fx.Option {
	return fx.Module(
		"confmaker",

		fx.Provide(
			func() (*Source, error) {
				paths, err := dirconfig.Resolve(dir)
				if err != nil {
					return nil, err
				}

				return openSource(paths...)
			},
		),
	)
}

// defaultConfigPath is read when LoadModule is given no paths: config.toml in the
// current working directory.
const defaultConfigPath = "config.toml"

func openSource(paths ...string) (*Source, error) {
	if len(paths) == 0 {
		paths = []string{defaultConfigPath}
	}

	k := koanf.New(".")

	for _, path := range paths {
		if err := k.Load(file.Provider(path), toml.Parser()); err != nil {
			return nil, fmt.Errorf("load config %q: %w", path, err)
		}
	}

	return &Source{k: k}, nil
}

// section decodes the sub-tree at path into dst using the `koanf:"..."` tags.
// Decoding is strict: unknown keys fail instead of being ignored. Existing
// exported secret.Value fields are cleared before decoding, and mapped file
// values targeting them are discarded.
func (s *Source) section(path string, dst any) error {
	sub := s.k.Cut(path)
	if len(sub.Keys()) == 0 {
		return fmt.Errorf("config section %q is missing or empty", path)
	}

	filedecode.ZeroSecrets(dst)

	if err := sub.UnmarshalWithConf("", dst, koanf.UnmarshalConf{
		Tag: "koanf",
		DecoderConfig: &mapstructure.DecoderConfig{
			Result:           dst,
			ErrorUnused:      true,
			WeaklyTypedInput: false,
			DecodeHook:       filedecode.Hooks(),
		},
	}); err != nil {
		return fmt.Errorf("decode section %q: %w", path, err)
	}

	return nil
}

// settings holds the resolved options for a single Provide call.
type settings struct {
	envPrefix string
}

// Option customizes how Provide builds a config.
type Option func(*settings)

// WithEnvPrefix overrides the environment-variable prefix applied to the env
// tags. By default the prefix is derived from name ("main" -> "MAIN_"); pass an
// empty string to read env tags with no prefix (shared, global variables).
func WithEnvPrefix(prefix string) Option {
	return func(s *settings) {
		s.envPrefix = prefix
	}
}

// ProvideDefault decodes section of the loaded file into a T, fills its
// env-tagged fields from the environment, validates it if T has a Validate
// method, and provides it into the container untagged - the default single
// instance. The env prefix is derived from the last segment of section, so
// ProvideDefault[Config]("postgres") reads the [postgres] table and variables
// such as POSTGRES_PASSWORD.
//
// Use it for the common one-connection case (paired with a no-argument library
// module); reach for Provide only when a second, named instance is needed.
func ProvideDefault[T any](section string, opts ...Option) fx.Option {
	set := settings{envPrefix: defaultEnvPrefix(lastSegment(section))}
	for _, opt := range opts {
		opt(&set)
	}

	return fx.Provide(build[T](section, set.envPrefix, section))
}

// Provide is ProvideDefault for a named instance: it provides the config tagged
// name:"<name>" and derives the env prefix from name. Use it for replicas and
// additional shards, e.g. Provide[Config]("postgres.replica", "replica") reads
// the [postgres.replica] table and variables such as REPLICA_PASSWORD.
func Provide[T any](section, name string, opts ...Option) fx.Option {
	set := settings{envPrefix: defaultEnvPrefix(name)}
	for _, opt := range opts {
		opt(&set)
	}

	return fx.Provide(
		fx.Annotate(
			build[T](section, set.envPrefix, name),
			fx.ResultTags(utilfx.NameTag(name)),
		),
	)
}

// ProvideNoFileDefault provides a T without loading a configuration file or
// requiring a Source. Starting from T's zero value, it fills fields declared
// with env tags, validates the result, and provides it untagged. Fields without
// env tags remain zero-valued. The env prefix is derived from name
// (WithEnvPrefix overrides it), so ProvideNoFileDefault[Config]("postgres")
// reads POSTGRES_* variables.
func ProvideNoFileDefault[T any](name string, opts ...Option) fx.Option {
	set := settings{envPrefix: defaultEnvPrefix(name)}
	for _, opt := range opts {
		opt(&set)
	}

	return fx.Provide(buildEnv[T](set.envPrefix, name))
}

// ProvideNoFile is ProvideNoFileDefault for a named instance: it provides the
// resulting config tagged name:"<name>" without loading a file or requiring a
// Source.
func ProvideNoFile[T any](name string, opts ...Option) fx.Option {
	set := settings{envPrefix: defaultEnvPrefix(name)}
	for _, opt := range opts {
		opt(&set)
	}

	return fx.Provide(
		fx.Annotate(
			buildEnv[T](set.envPrefix, name),
			fx.ResultTags(utilfx.NameTag(name)),
		),
	)
}

// build returns the constructor that decodes one section from the loaded file,
// fills env fields with the given prefix, and validates the result. label names
// the instance in the validation error.
func build[T any](section, envPrefix, label string) func(*Source) (T, error) {
	return func(src *Source) (T, error) {
		var cfg T

		if err := src.section(section, &cfg); err != nil {
			return cfg, err
		}
		if err := fillEnv(&cfg, envPrefix, label); err != nil {
			return cfg, err
		}

		return cfg, nil
	}
}

// buildEnv returns a constructor that starts with T's zero value and fills its
// env-tagged fields without a file or Source dependency.
func buildEnv[T any](envPrefix, label string) func() (T, error) {
	return func() (T, error) {
		var cfg T

		if err := fillEnv(&cfg, envPrefix, label); err != nil {
			return cfg, err
		}

		return cfg, nil
	}
}

// fillEnv fills the env-tagged fields of cfg with the given prefix and validates
// it if T has a Validate method.
func fillEnv[T any](cfg *T, envPrefix, label string) error {
	if err := env.ParseWithOptions(cfg, env.Options{Prefix: envPrefix}); err != nil {
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

// defaultEnvPrefix turns an instance name into an env prefix: "main" -> "MAIN_",
// "read-replica" -> "READ_REPLICA_".
func defaultEnvPrefix(name string) string {
	return strings.ToUpper(strings.NewReplacer("-", "_", ".", "_").Replace(name)) + "_"
}

// lastSegment returns the part of a dotted config path after the final dot, so
// the env prefix of a default instance follows its table ("postgres" ->
// "postgres", "db.postgres" -> "postgres").
func lastSegment(path string) string {
	if i := strings.LastIndex(path, "."); i >= 0 {
		return path[i+1:]
	}

	return path
}

// cleanEnvError strips the "env: " prefix the env parser puts on its messages so
// the error states only the problem (the caller adds package context). Missing
// required variables are named without a value; conversion errors for ordinary,
// non-secret fields may include their source value.
func cleanEnvError(err error) error {
	return fmt.Errorf("resolve environment config: %s", strings.TrimPrefix(err.Error(), "env: "))
}
