# confmaker

[![CI](https://github.com/uchaloop/confmaker/actions/workflows/ci.yml/badge.svg)](https://github.com/uchaloop/confmaker/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/uchaloop/confmaker.svg)](https://pkg.go.dev/github.com/uchaloop/confmaker)
[![License: MIT](https://img.shields.io/github/license/uchaloop/confmaker)](LICENSE)

Typed configuration for Go services, read from the environment and nowhere else
([12factor III](https://12factor.net/config)). Each library declares its own
config; confmaker fills it, validates it, and hands it to Uber Fx. Its only
dependencies are Fx and a secret type.

```bash
go get github.com/uchaloop/confmaker
```

## Declaring a config

A library declares its config as a plain struct with `env` tags:

```go
type Config struct {
	Addr     string        `env:"ADDR"`
	Timeout  time.Duration `env:"TIMEOUT"`
	Password secret.Secret `env:"PASSWORD,notEmpty"`
}
```

The tags are inert strings, so the library depends only on
[`secret/v2`](https://github.com/uchaloop/secret), not on this package. A field
without an `env` tag is not configuration and is never touched.

Field types: `string`, `bool`, every sized integer and float, `time.Duration`,
anything implementing `encoding.TextUnmarshaler`, a pointer to any of those, and
a slice or map of them. `TextUnmarshaler` is the extension point - a decimal, a
nullable, a UUID decodes itself, with no parser to register.

### Defaults

Defaults live in code, where the library's tests and its callers see the same
values the environment starts from:

```go
func (c *Config) SetDefaults() {
	c.Addr = "localhost:9000"
	c.Timeout = 30 * time.Second
}
```

The method is optional; a config without one starts from its zero value. Then:

- a variable that is **not set** leaves its field as `SetDefaults` left it;
- a variable that is **set** assigns its field;
- a variable set to the **empty string** assigns the empty value.

There is no `envDefault` tag - a default in a tag is invisible to code.

### Required values

`required` fails the start when the variable is unset, `notEmpty` when it is
unset or empty. Both are about the *variable*, not the value: a default in
`SetDefaults` satisfies neither, because the deployment is still expected to
supply it.

### Validation

`Validate() error` runs after the environment is applied:

```go
func (c Config) Validate() error {
	var errors validate.Errors
	errors.Require(len(c.Addr) != 0, "addr is required")
	errors.Require(c.Timeout > 0, "timeout must be positive")

	return errors.Err()
}
```

Every problem is reported at once - across binding, parsing and validation
alike - one per line, each naming the config it belongs to:

```text
config "postgres": required variable "POSTGRES_HOST" is not set
config "postgres": timeout must be positive
```

### Secrets

Use [`secret/v2`](https://github.com/uchaloop/secret) for anything that must not
be logged. A secret is never printed: not in the dump, not in the manifest, not
in the error for a value that would not parse, and it publishes no default.

## Wiring it up

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

The name is both the prefix and the Fx tag, so it may hold only lowercase
letters, digits and `_ - .`, and may not start or end with a separator.
Whichever separator it uses, the variable is written with underscores:
`read-replica` and `read_replica` both read `READ_REPLICA_HOST`, which is why
two instances may not be named that way at once.

`WithPrefix` overrides the prefix when it should not follow the name:

```go
confx.Provide[otherlib.Config]("analytics", confx.WithPrefix("REPORTING_"))
```

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

Nest by value: how many variables a config reads has to be known from its type,
which a pointer or a collection cannot promise.

### Slices and maps

A slice splits on `envSeparator` (default `,`). A map reads from one variable,
splitting entries the same way and a key from its value on `envKeyValSeparator`
(default `:`).

```go
type Config struct {
	Brokers []string          `env:"BROKERS"`
	Labels  map[string]string `env:"LABELS"`
}
```

```text
OZON_BROKERS=a:9092,b:9092
OZON_LABELS=env:prod,team:core
```

A duplicate key is an error, and a key or value padded with whitespace is
reported rather than trimmed - `env: prod` says so instead of quietly yielding
`" prod"`.

## Catching mistakes

What a declaration can get wrong is refused when the config is bound - on the
first start, whether or not the environment happens to set the variable:

| Refused | Because |
|---|---|
| `envDefault` on a field | a default belongs in `SetDefaults` |
| an option other than `required` / `notEmpty` | `requred` would leave the field optional in silence |
| two fields claiming one variable | one value would fill both |
| a config nested through a pointer, slice, map or array | its variables cannot be known from the type |
| a field of a type that cannot be read from text | it could never be filled |
| an empty `envSeparator` | a value would split into single characters |
| an instance name or prefix that is not one | it would read nothing and say nothing |

`confx.Module()` adds the check the environment itself needs. A variable that
starts with a prefix the application owns but matches no field fails the start:

```text
unknown configuration variable "POSTGRES_HSOT" (did you mean "POSTGRES_HOST"?)
```

Variables outside the application's own prefixes are never examined. Register
`Module` before the `Provide` calls it covers, so the check runs ahead of the
constructors and reports the typo instead of the missing value it causes. When
one environment is shared between binaries, exempt a sibling's prefix with
`confx.AllowUnknown("EXPORTER_")`.

### Seeing what was read

`WithDump` writes every variable the application reads, with its value and where
that value came from:

```go
confx.Module(confx.WithDump(os.Stdout))
```

```text
INSTANCE  VARIABLE                 TYPE           VALUE     SOURCE
postgres  POSTGRES_HOST            string         db:5432   env
postgres  POSTGRES_PASSWORD        secret.Secret  (set)     env
postgres  POSTGRES_POOL_MAX_CONNS  int32          2         default
```

## Generating from the manifest

`Manifest` resolves the same list without building an application, so a
`.env.example`, a ConfigMap or a documentation table comes from the config type
itself:

```go
variables, err := confx.Manifest[pgfx.Config]("postgres")
if err != nil {
	return err
}

for _, v := range variables {
	fmt.Printf("%s=%s\n", v.Name, v.Default)
}
```

```text
POSTGRES_HOST=
POSTGRES_POOL_MAX_CONNS=2
POSTGRES_PASSWORD=
```

Variables come in declaration order, each with its Go type, whether it is
required, whether it holds a secret, and its default rendered as text the
variable could carry back. It is the same traversal that fills the config, so a
variable it lists is exactly a variable the config reads.

## Acknowledgements

I am grateful to the authors of [Uber Fx](https://github.com/uber-go/fx), and to
the authors of [env](https://github.com/caarlos0/env), whose tag vocabulary this
library follows and whose implementation it learned from.

## License

[MIT](LICENSE)
