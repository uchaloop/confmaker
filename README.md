# confmaker

[![CI](https://github.com/uchaloop/confmaker/actions/workflows/ci.yml/badge.svg)](https://github.com/uchaloop/confmaker/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/uchaloop/confmaker.svg)](https://pkg.go.dev/github.com/uchaloop/confmaker)
[![License: MIT](https://img.shields.io/badge/github/license/uchaloop/confmaker)](LICENSE)

Typed configuration for Go services: environment variables only, wired into Uber
Fx, with validation and a strict check that catches misspelled variables at
startup.

Configuration lives in the environment and nowhere else - no files, no
environment-named config groups
([12factor III](https://12factor.net/config)).

## Installation

```bash
go get github.com/uchaloop/confmaker
```

## Configuration type

A library declares its config as a plain struct with `env` tags:

```go
type Config struct {
	Addr     string        `env:"ADDR"`
	Timeout  time.Duration `env:"TIMEOUT"`
	Password secret.Secret `env:"PASSWORD,notEmpty"`
}
```

The tags are inert strings, so the library depends only on
[`github.com/uchaloop/secret/v2`](https://github.com/uchaloop/secret), not on
this package. A field without an `env` tag keeps its zero value - that is where a
library puts its defaults.

## Fx

### Default and named instances

The instance name gives the environment prefix:

```go
fx.New(
	confx.Module(),
	confx.Provide[otherlib.Config]("otherlib"),
	confx.ProvideNamed[otherlib.Config]("analytics"),
	otherlib.Module(),
)
```

```text
OTHERLIB_ADDR
ANALYTICS_ADDR
```

`Provide` gives the container an untagged value - the single default instance.
`ProvideNamed` gives it a value tagged `name:"analytics"`, for replicas and
additional instances of the same type.

Override the prefix when it should not follow the name:

```go
confx.Provide[otherlib.Config]("analytics", confx.WithPrefix("REPORTING_"))
```

`WithPrefix("")` reads the `env` tags with no prefix at all. Such an instance
owns no prefix, so the strict check below skips it - it would otherwise claim
every variable in the environment.

### Nested configuration

`envPrefix` extends the prefix for a nested struct:

```go
type Config struct {
	Host string     `env:"HOST"`
	Pool PoolConfig `envPrefix:"POOL_"`
}
```

```text
POSTGRES_HOST
POSTGRES_POOL_MAX_CONNS
```

### The strict check

`confx.Module()` compares the environment against the manifest the `Provide`
calls register. A variable that starts with a prefix the application owns but
matches no field fails the start:

```text
unknown configuration variable "POSTGRES_HSOT" (did you mean "POSTGRES_HOST"?)
```

Only the application's own prefixes are examined; everything else in the
environment is left alone. Register `Module` before the `Provide` calls it
covers, so the check runs ahead of the constructors and reports the typo instead
of the missing value it causes.

When one environment is shared between several binaries, exempt a sibling's
prefix:

```go
confx.Module(confx.AllowUnknown("EXPORTER_"))
```

A config that reads per-element variables (a slice of structs, which the env
parser numbers as `PREFIX_0_FIELD`) cannot be enumerated from its type, so its
prefix is skipped by the check.

### Dumping the configuration

`WithDump` writes every variable the application reads, with its current value
and where that value comes from:

```go
confx.Module(confx.WithDump(os.Stdout))
```

```text
INSTANCE  VARIABLE                 TYPE           VALUE     SOURCE
postgres  POSTGRES_HOST            string         db:5432   env
postgres  POSTGRES_PASSWORD        secret.Secret  (set)     env
postgres  POSTGRES_POOL_MAX_CONNS  int32          (unset)   zero value
```

Secrets are reported as set or unset; their values are never written.

## Generating from the manifest

`Manifest` resolves the same list without building an application, so a
`.env.example`, a ConfigMap, or a documentation table can be generated from the
config type itself:

```go
for _, v := range confx.Manifest[pgfx.Config]("postgres") {
	fmt.Printf("%s=%s\n", v.Name, v.Default)
}
```

```text
POSTGRES_HOST=
POSTGRES_POOL_MAX_CONNS=2
POSTGRES_PASSWORD=
```

Variables come in declaration order, and each carries its Go type, whether it is
required, whether it holds a secret, and its declared `envDefault`. It takes the
same options as `Provide`, so a manifest resolved with `WithPrefix` matches the
instance provided with it.

## Secrets

Use [`github.com/uchaloop/secret/v2`](https://github.com/uchaloop/secret) for
secret values:

```go
type Config struct {
	Password secret.Secret `env:"PASSWORD,notEmpty"`
}
```

`notEmpty` fails the start when the variable is unset or empty, naming only the
variable and never the value.

## Validation

`confx` calls `Validate() error` after applying the environment:

```go
func (c Config) Validate() error {
	var errors validate.Errors
	errors.Require(len(c.Addr) != 0, "addr is required")
	errors.Require(c.Timeout > 0, "timeout must be positive")

	return errors.Err()
}
```

## Acknowledgements

I am grateful to the authors of [env](https://github.com/caarlos0/env) and
[Uber Fx](https://github.com/uber-go/fx). Their work made this library possible.

## License

[MIT](LICENSE)
