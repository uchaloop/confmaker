# confmaker

[![CI](https://github.com/uchaloop/confmaker/actions/workflows/ci.yml/badge.svg)](https://github.com/uchaloop/confmaker/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/uchaloop/confmaker.svg)](https://pkg.go.dev/github.com/uchaloop/confmaker)
[![License: MIT](https://img.shields.io/badge/github/license/uchaloop/confmaker)](LICENSE)

Typed configuration for Go services with strict TOML decoding, environment
overrides, validation, and Uber Fx integration.

## Installation

```bash
go get github.com/uchaloop/confmaker
```

## Configuration type

Use `koanf` tags for TOML fields and `env` tags for environment variables:

```go
type Config struct {
	Addr     string        `koanf:"addr" env:"ADDR"`
	Timeout  time.Duration `koanf:"timeout"`
	Password secret.Secret `koanf:"-" env:"PASSWORD,notEmpty"`
}
```

Environment variables override TOML values. A field marked with `koanf:"-"`
cannot be populated from TOML.

## Fx

### Environment directory

For applications with environment-specific files:

```text
config/
├── common.toml   # optional
├── dev.toml
├── stage.toml
└── prod.toml
```

Set the deployment environment:

```bash
ENVIRONMENT=prod
```

Then pass only the directory:

```go
fx.New(
	confx.LoadDir("config"),
	confx.ProvideDefault[otherlib.Config]("otherlib"),
	otherlib.Module(),
)
```

Supported values:

| `ENVIRONMENT` | File |
|---|---|
| `dev` | `dev.toml` |
| `stage` | `stage.toml` |
| `prod` | `prod.toml` |
| `prd` | `prod.toml` |

The environment file is required. When `common.toml` exists, precedence is:

```text
common.toml < environment file < field environment variables
```

Without Fx:

```go
var cfg AppConfig
if err := confmaker.LoadDir(&cfg, "config"); err != nil {
	return err
}
```

### Explicit files

Use `LoadModule` when file paths are selected by the application:

```go
fx.New(
	confx.LoadModule(
		"config/common.toml",
		"config/prod.toml",
	),
	confx.ProvideDefault[otherlib.Config]("otherlib"),
	otherlib.Module(),
)
```

Files are applied in order; later files override earlier ones.
`confx.LoadModule()` without arguments reads `config.toml` from the current
directory and does not require `ENVIRONMENT`.

### Default and named instances

Given:

```toml
[otherlib]
addr = "localhost:9000"

[analytics]
addr = "analytics:9000"
```

Provide the default instance untagged and the additional instance with an Fx
name:

```go
fx.New(
	confx.LoadModule("config.toml"),
	confx.ProvideDefault[otherlib.Config]("otherlib"),
	confx.Provide[otherlib.Config]("analytics", "analytics"),
)
```

Environment prefixes are derived from the section or instance name:

```text
OTHERLIB_ADDR
ANALYTICS_ADDR
```

Override a prefix when needed:

```go
confx.Provide[otherlib.Config](
	"analytics",
	"analytics",
	confx.WithEnvPrefix("REPORTING_"),
)
```

### Environment-only configuration

When a configuration does not need a file:

```go
fx.New(
	confx.ProvideNoFileDefault[otherlib.Config]("otherlib"),
	confx.ProvideNoFile[otherlib.Config]("analytics"),
)
```

Only fields with `env` tags are populated. `ProvideNoFileDefault` provides an
untagged value; `ProvideNoFile` provides a named value.

## Without Fx

Load one or more explicit TOML files:

```go
type AppConfig struct {
	Service   ServiceConfig              `koanf:"service"`
	Instances map[string]otherlib.Config `koanf:"otherlib"`
}

var cfg AppConfig
if err := confmaker.Load(
	&cfg,
	"config/common.toml",
	"config/prod.toml",
); err != nil {
	return err
}
```

`confmaker.Load(&cfg)` reads `config.toml` from the current directory.

Decoding is strict:

- unknown TOML keys return an error;
- durations are accepted as strings such as `"30s"`;
- weak type conversion is disabled.

## Secrets

Use [`github.com/uchaloop/secret/v2`](https://github.com/uchaloop/secret) for
secret values:

```go
type Config struct {
	Password secret.Secret `koanf:"-" env:"PASSWORD,notEmpty"`
}
```

Secrets are not loaded from TOML. With `confx`, fields with `env` tags are
populated from environment variables.

Without Fx, read a required secret directly:

```go
password, err := confmaker.ResolveSecret("OTHERLIB_PASSWORD")
```

## Validation

`confx` calls `Validate() error` after applying file and environment values:

```go
func (c Config) Validate() error {
	var errors validate.Errors
	errors.Require(c.Addr != "", "addr is required")
	errors.Require(c.Timeout > 0, "timeout must be positive")

	return errors.Err()
}
```

## Named instances without Fx

Get a required entry from a map:

```go
main, err := confmaker.Required(cfg.Instances, "otherlib", "main")
```

For names selected at runtime:

```go
registry := confmaker.MakeRegistry(cfg.Instances)
instance, err := registry.Get("otherlib", name)
names := registry.Names()
```

## Acknowledgements

I am grateful to the authors of [Koanf](https://github.com/knadh/koanf),
[env](https://github.com/caarlos0/env), and
[Uber Fx](https://github.com/uber-go/fx). Their work made this library possible.

## License

[MIT](LICENSE)
