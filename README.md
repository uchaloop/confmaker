# confmaker

[![CI](https://github.com/uchaloop/confmaker/actions/workflows/ci.yml/badge.svg)](https://github.com/uchaloop/confmaker/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/uchaloop/confmaker.svg)](https://pkg.go.dev/github.com/uchaloop/confmaker)
[![License: MIT](https://img.shields.io/github/license/uchaloop/confmaker)](LICENSE)

Typed configuration for Go services: environment variables only, wired into Uber
Fx, with defaults in code, validation, and a strict check that catches
misspelled variables at startup. It depends on Fx and a secret type, nothing
else.

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
this package. A field without an `env` tag is not configuration and is never
touched.

Field types: `string`, `bool`, every sized integer and float, `time.Duration`,
anything implementing `encoding.TextUnmarshaler`, a pointer to any of those, and
a slice or map of them. `complex`, `uintptr`, and `[]byte` are rejected rather
than guessed at.

## Defaults

A config establishes its own defaults in code, where its tests and its callers
see the same values the environment starts from:

```go
func (c *Config) SetDefaults() {
	c.Addr = "localhost:9000"
	c.Timeout = 30 * time.Second
}
```

The method is optional and its name is the whole contract - a config without one
starts from its zero value. Three rules govern what happens next:

- a variable that is **not set** leaves its field as `SetDefaults` left it;
- a variable that is **set** assigns its field;
- a variable set to the **empty string** assigns the empty value, and fails
  `notEmpty`.

There is no `envDefault` tag: a default in a tag is invisible to code and to
tests. Declaring one is an error that names `SetDefaults` as its replacement.

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

### Nesting

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

Nest by value. A **config** reached through a pointer, a slice, a map, or an
array is rejected: how many variables it would read cannot be known from the
type, and that is exactly what the strict check and the manifest rest on. Only a
struct that names variables of its own counts, so an untagged field holding a
decimal or a timestamp is skipped like any other untagged field.

### Maps

A map is read from one variable, so it stays enumerable:

```go
type Config struct {
	Labels map[string]string `env:"LABELS"`
}
```

```text
OZON_LABELS=env:prod,team:core
```

`envSeparator` (default `,`) splits entries, `envKeyValSeparator` (default `:`)
splits a key from its value. A duplicate key is an error, and a key or value
padded with whitespace is reported rather than trimmed - `env: prod` says so
instead of quietly yielding the value `" prod"`. The same `envSeparator`
splits a slice.

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
postgres  POSTGRES_POOL_MAX_CONNS  int32          2         default
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
required, whether it holds a secret, and the default `SetDefaults` establishes
for it - rendered as text the variable could carry back. It takes the same
options as `Provide`, so a manifest resolved with `WithPrefix` matches the
instance provided with it.

The manifest comes from the same traversal that fills the config, so a variable
it lists is exactly a variable the config reads.

## Secrets

Use [`github.com/uchaloop/secret/v2`](https://github.com/uchaloop/secret) for
secret values:

```go
type Config struct {
	Password secret.Secret `env:"PASSWORD,notEmpty"`
}
```

`notEmpty` fails the start when the variable is unset or empty, naming only the
variable and never the value. A secret is never printed: not in the dump, not in
the manifest, and not in the error for a value that would not parse.

`required` and `notEmpty` are about the variable, not the value - a default in
`SetDefaults` does not satisfy either, because the deployment is still expected
to supply it.

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

Every problem is reported at once - across defaults, parsing, and validation
alike - so a misconfigured deployment takes one run to diagnose, not one run per
mistake.

## Acknowledgements

I am grateful to the authors of [Uber Fx](https://github.com/uber-go/fx), and to
the authors of [env](https://github.com/caarlos0/env), whose tag vocabulary this
library follows and whose implementation it learned from.

## License

[MIT](LICENSE)
