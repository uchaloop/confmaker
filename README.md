# confmaker

[![CI](https://github.com/uchaloop/confmaker/actions/workflows/ci.yml/badge.svg)](https://github.com/uchaloop/confmaker/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/uchaloop/confmaker.svg)](https://pkg.go.dev/github.com/uchaloop/confmaker)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

Application-level configuration for Go services. A library declares its config as
a plain struct with two tag namespaces - `koanf` (from the file) and `env` (from
the environment); confmaker loads both and fills the struct, then hands it to the
library.

The idea: **the service owns the config source, the libraries do not.** A library
receives a ready-made typed `Config` and never reads files or the environment
itself - the tags it declares are inert strings only the app-level loader reads.

## Layering (important)

The masked `Secret` type lives in the separate zero-dependency module
[`github.com/uchaloop/secret/v2`](https://github.com/uchaloop/secret). This keeps
configuration types declared by libraries independent of confmaker: a library
can expose a `secret.Secret` field without taking a dependency on the
application-level loading stack.

The packages are layered as follows:

| Package | For | Dependencies |
|---|---|---|
| `github.com/uchaloop/secret/v2` | **libraries and apps** — masked secret values | standard library only; separate module |
| `github.com/uchaloop/confmaker/validate` | apps — error accumulator for `Validate()` | standard library only |
| `github.com/uchaloop/confmaker` | **apps only** — TOML loader and helpers | secret, koanf, mapstructure |
| `github.com/uchaloop/confmaker/confx` | **apps only** — Fx wiring from file and environment to typed config | confmaker, env11, fx |

An infrastructure library such as a Postgres or Kafka client imports
`github.com/uchaloop/secret/v2` only when its config contains a secret. It does not
need confmaker: the `koanf` and `env` struct tags it declares are inert strings.
The application imports the root package or `confx` and is responsible for
interpreting those tags, loading values, and validating the resulting config.

## Install

```bash
go get github.com/uchaloop/confmaker
```

## Usage

### Fx wiring: one typed config per named instance (`confx`)

The recommended path. A library declares its config as a plain struct with two
tag namespaces - `koanf` (from the file) and `env` (from the environment) - and
`confx` fills it and provides it into the container under a name tag:

```go
// declared by a library (e.g. otherlib); both tags are inert strings, so the
// library depends only on github.com/uchaloop/secret/v2.
type Config struct {
	Addr     string        `koanf:"addr" env:"ADDR"`           // file, env overrides
	Timeout  time.Duration `koanf:"timeout"`                   // file only
	Password secret.Secret `koanf:"-" env:"PASSWORD,notEmpty"` // env only (present + non-empty)
}
```

The common case is a single instance - `[otherlib]` in the file, `OTHERLIB_` in
the environment, no tags:

```toml
# config/local.toml
[otherlib]
addr    = "localhost:9000"
timeout = "5s"
```

```go
fx.New(
	confx.LoadModule("config/local.toml"),
	confx.ProvideDefault[otherlib.Config]("otherlib"), // untagged, -> OTHERLIB_PASSWORD
	otherlib.Module(),                                 // untagged client
)
```

Add named instances only when a service needs a second one - each is its own
top-level section, with its own tag and env prefix:

```toml
[otherlib]          # default instance
addr = "localhost:9000"

[analytics]         # a second instance of the same library
addr = "analytics:9000"
```

```go
fx.New(
	confx.LoadModule("config/local.toml"),
	confx.ProvideDefault[otherlib.Config]("otherlib"),        // untagged, OTHERLIB_*
	confx.Provide[otherlib.Config]("analytics", "analytics"), // tagged, ANALYTICS_*
)
```

- `ProvideDefault[T](section)` decodes `[section]` into a `T`, fills its `env`
  fields, runs `T.Validate()` if present, and provides `T` **untagged**. The env
  prefix comes from the section's last segment (`otherlib` -> `OTHERLIB_`).
- `Provide[T](section, name)` does the same but provides `T` tagged
  `name:"<name>"`, with the prefix derived from `name` (`analytics` ->
  `ANALYTICS_`). Override either with `confx.WithEnvPrefix(...)`.
- Named instances are separate top-level tables. Nesting one under another (a
  `[otherlib.analytics]` sub-table while also reading `[otherlib]`) makes the
  child a key of the parent, which strict decoding rejects.
- Precedence for a field with both tags: **file first, env overrides.**
- A file value accidentally mapped to `secret.Secret` is ignored; secrets can
  only be populated separately, such as from env. With `koanf:"-"`, the key is
  excluded entirely and strict decoding rejects it as unknown.
- `LoadModule()` with no paths reads `config.toml` from the current directory.

### Configuration without a file

Some configurations do not need a file or `LoadModule`. Use `ProvideNoFile` /
`ProvideNoFileDefault` to start with the zero value of the config and fill the
fields that declare `env` tags:

```go
fx.New(
	confx.ProvideNoFileDefault[otherlib.Config]("otherlib"), // untagged, OTHERLIB_*
	confx.ProvideNoFile[otherlib.Config]("analytics"),       // tagged, ANALYTICS_*
	otherlib.Module(),
)
```

These helpers perform no `koanf` decode and require no `Source`. Fields without
an `env` tag remain zero-valued; choosing the tags and validating the completed
config are the config author's responsibility. The result is validated and
provided tagged or untagged just like `Provide` / `ProvideDefault`. No-file and
file-backed providers can be used in the same application.

### Load open configuration (strict TOML)

Without Fx, load a whole struct directly:

```go
type AppConfig struct {
	Service   ServiceConfig              `koanf:"service"`
	Instances map[string]otherlib.Config `koanf:"otherlib"`
}

var cfg AppConfig
if err := confmaker.Load(&cfg, "config/base.toml", "config/prod.toml"); err != nil {
	return err
}
```

- With no paths, `Load` reads `config.toml` from the current directory.
- Later files override earlier ones (base + environment overlay).
- Decoding is **strict**: an unknown key (a typo, or a field that does not exist
  on the struct) fails instead of being silently ignored.
- Values targeting exported `secret.Value` fields are ignored even if a `koanf` tag
  accidentally maps them; files can never populate secrets.
- Durations are parsed from strings such as `"30m"`; weak type coercion is off.

### Secrets

The `secret.Secret` type comes from the separate zero-dependency module
[`github.com/uchaloop/secret/v2`](https://github.com/uchaloop/secret). It masks
itself when formatted, logged, or serialized; retrieving the real value requires
an explicit `Reveal()` call.

Secrets never come from the config file. Before file decoding, confmaker clears
supported exported configuration values implementing `secret.Value`, regardless
of their tags; mapped file values for those types are discarded. Unexported
fields and map keys are outside the configuration schema and remain untouched.
With `confx`, a secret is then filled from the environment only when it has an
`env` tag. Without Fx, read a single secret directly:

```go
password, err := confmaker.ResolveSecret("OTHERLIB_PASSWORD")
// error names only the variable, never the value
```

### Named instances without Fx

For a non-Fx service that loads a map of instances, pick them by name:

```go
main, err := confmaker.Required(cfg.Instances, "otherlib", "main") // mandatory
reg := confmaker.MakeRegistry(cfg.Instances)                        // dynamic set
inst, err := reg.Get("otherlib", name)
```

### Validate (accumulate all errors)

```go
func (c AppConfig) Validate() error {
	var e validate.Errors
	e.Require(c.Service.Name != "", "service.name is required")
	// ...
	return e.Err()
}
```

## Testing

```bash
go test ./...
```

## Acknowledgements

confmaker builds on [knadh/koanf](https://github.com/knadh/koanf),
[go-viper/mapstructure](https://github.com/go-viper/mapstructure),
[caarlos0/env](https://github.com/caarlos0/env) and
[uber-go/fx](https://github.com/uber-go/fx). Thanks to their authors and
maintainers.

## License

[MIT](LICENSE).
