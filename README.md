# confmaker

<p align="center"><img src="logo.png" alt="confmaker" width="240"></p>

[![Go Reference](https://pkg.go.dev/badge/github.com/uchaloop/confmaker/v2.svg)](https://pkg.go.dev/github.com/uchaloop/confmaker/v2) [![CI](https://github.com/uchaloop/confmaker/actions/workflows/ci.yml/badge.svg)](https://github.com/uchaloop/confmaker/actions/workflows/ci.yml) [![Coverage](https://codecov.io/gh/uchaloop/confmaker/branch/main/graph/badge.svg)](https://app.codecov.io/gh/uchaloop/confmaker) [![Release](https://img.shields.io/github/v/tag/uchaloop/confmaker?label=release)](https://github.com/uchaloop/confmaker/tags) [![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

**Explicit, centralized configuration for Go applications.** Packages declare
configuration types; the application loads and validates them before use.
Use the built-in ENV engine or supply your own backend while keeping the same
Loader, handles and diagnostics. Fx wiring is provided separately by
[confx](https://github.com/uchaloop/confx).

The core module has no external dependencies, including its unit tests.
This README describes v2; use the documentation for your installed version.

[Install](#installation) · [Quick start](#quick-start) · [Lifecycle](#how-it-works) · [ENV](#built-in-env-engine) · [Engines](#custom-engines) · [Diagnostics](#errors-and-diagnostics) · [Manifest](#manifest-and-exports) · [Reference](#reference)

## Installation

Requires Go **1.27.0** or later.

```sh
go get github.com/uchaloop/confmaker/v2
```

## Quick start

```go
package main

import (
	"fmt"
	"log"
	"time"

	"github.com/uchaloop/confmaker/v2"
)

type StoreConfig struct {
	Host    string        `env:"HOST,notEmpty"`
	Timeout time.Duration `env:"TIMEOUT"`
}

func (c *StoreConfig) SetDefaults() {
	c.Timeout = 30 * time.Second
}

func (c StoreConfig) Validate() error {
	if c.Timeout <= 0 {
		return fmt.Errorf("timeout must be positive")
	}

	return nil
}

func main() {
	cfg, err := confmaker.Load[StoreConfig]("store")
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(cfg.Host, cfg.Timeout)
}
```

Run with `STORE_HOST=db:5432 go run .`. The result is `db:5432 30s`.
Configuration types need no confmaker import; defaults and validation are ordinary
Go methods. Only methods on the root configuration run automatically.

## How it works

For one configuration, use `Load[T]`. For several, create a `MakeLoader()`,
register every configuration, call `Load()`, then read each handle's `Value()`.

1. Check declarations and conflicts.
2. Create fresh configurations and call root `SetDefaults()` methods.
3. Fill configurations using the selected engine.
4. Run root `Validate()` after successful parsing of that configuration.
5. Publish values only if the whole set succeeds.

Loading is one-shot: subsequent calls return the same result. `Value()` never
starts loading. A registration made after loading starts is rejected.

The built-in engine takes one ENV snapshot for the load. Missing variables retain
defaults; present values replace entire fields, including collections.
`LoadContext` supports cooperative cancellation; see [Loader.LoadContext] for
ownership and waiting rules. Configuration maps, slices and pointers are shared:
treat loaded configurations as read-only.

## Names and instances

Register the same type under distinct names:

```go
loader := confmaker.MakeLoader()

primary := loader.Register[StoreConfig]("primary")
replica := loader.Register[StoreConfig]("replica", confmaker.WithPrefix("READ_DB_"))

if err := loader.Load(); err != nil {
	return err
}

primaryConfig, err := primary.Value()
if err != nil {
	return err
}

replicaConfig, err := replica.Value()
if err != nil {
	return err
}

fmt.Println(primaryConfig.Host, replicaConfig.Host)
```

This fragment reuses `StoreConfig` from the quick start and runs inside a function
returning an error. It reads `PRIMARY_HOST` and `READ_DB_HOST`.
With the built-in engine, names use lowercase letters, digits, `_`, `-` and `.`,
and start/end with a letter or digit. `read-replica` produces `READ_REPLICA_`.
`WithPrefix` changes the prefix, not registration identity. Names, prefixes and
full variable names must not conflict. `Handle.Name` and `Handle.BelongsTo`
inspect identity without loading.

## Built-in ENV engine

| Declaration | Meaning |
| --- | --- |
| `env:"HOST"` | Missing ENV retains defaults |
| `env:"HOST,required"` | ENV must contain the variable; defaults do not satisfy it |
| `env:"HOST,notEmpty"` | ENV must contain nonempty text |
| `env:"TOKEN,secret"` | Treat the whole field as sensitive |
| `env:"-"` | Ignore the field and its children |
| `envPrefix:"POOL_"` | Extend prefix for a nested config held by value |
| `envDescription:"..."` | Description in manifest |
| `envSeparator:";"` | Plain slice/map separator, default comma |
| `envKeyValSeparator:"="` | Plain map key/value separator, default colon |
| `envFormat:"json"` | One JSON value for a collection |

Only the root SetDefaults and Validate methods run. Apply defaults first, replace
fields from present ENV, then Validate if parsing succeeded. Compose nested
methods explicitly. A defaults factory can also be used outside confmaker.
Defaults must be deterministic and free of side effects. `envDefault` is refused.
Validate checks resulting values: JSON `[]` is nonempty text but an empty list.

Supported values include strings, bools, integers, floats, durations, custom
encoding.TextUnmarshaler types, and one pointer level to a scalar/text type.
Plain lists and maps support scalar/text values. JSON supports arrays, slices,
maps and nested collections; ordinary structs are rejected at every depth.
Text types use JSON strings and text methods, not their JSON methods.
Pointer chains, pointers to collections and pointers inside collections are rejected.
No automatic traversal of custom text types' internal state is performed.

Present ENV replaces the entire field, including collections. There is no merge.
Plain syntax has no quoting/escaping language; choose another separator or JSON.
Duplicate map keys and padded plain key/value text are errors. JSON null clears
maps/slices; null is invalid for nonnullable values. Durations use text such as
30s, including JSON strings. Interfaces, complex, uintptr and byte collections
without custom text forms are unsupported. Map keys must satisfy codec rules.

Unknown variables under registered prefixes are errors; unrelated ENV is ignored.
AllowUnknown exempts specified prefixes. Required/notEmpty concern the supplied
text, not the final Go value.

## Custom engines

Without options, confmaker uses the [built-in ENV engine](#built-in-env-engine).
`WithEngine(engine)` selects a backend for every registration. It implements:

```go
type Engine interface {
	Load(context.Context, LoadRequest) error
}

type LoadRequest struct {
	Name   string
	Target any
}
```

`Target` is a non-nil pointer to a fresh configuration struct. Root `SetDefaults`
runs before the backend and root `Validate` after successful filling. The backend
owns tags, supported field types, sources and overwrite rules; its defaults may
replace the initial values. It must not retain Target or modify it after returning.
Names may be any nonempty unique strings with a custom engine. The built-in engine
retains the stricter [name rules](#names-and-instances).

`EngineFunc` wraps a function. This complete application example uses env/v11;
install that dependency in the application, not in confmaker:

```go
package main

import (
	"context"
	"fmt"
	"log"
	"maps"

	env "github.com/caarlos0/env/v11"
	"github.com/uchaloop/confmaker/v2"
)

func makeEnvEngine(vars, prefixes map[string]string) confmaker.Engine {
	snapshot := maps.Clone(vars)
	names := maps.Clone(prefixes)

	return confmaker.EngineFunc(func(ctx context.Context, req confmaker.LoadRequest) error {
		if err := ctx.Err(); err != nil {
			return err
		}

		prefix, ok := names[req.Name]
		if !ok {
			return fmt.Errorf("unknown registration %q", req.Name)
		}

		return env.ParseWithOptions(req.Target, env.Options{
			Environment: maps.Clone(snapshot),
			Prefix:      prefix,
		})
	})
}

func main() {
	engine := makeEnvEngine(
		map[string]string{"APP_PORT": "8080"},
		map[string]string{"server": "APP_"},
	)

	cfg, err := confmaker.Load[struct {
		Port int `env:"PORT,required"`
	}]("server", confmaker.WithEngine(engine))
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(cfg.Port)
}
```

This example adopts env/v11 semantics; it does not interrupt a running parser or
sanitize its errors. Errors from any backend may contain sensitive input. Report
never includes error text, but returning or logging the original error is the
backend/application's responsibility. No external engine dependency is included
in confmaker.

The engine receives sequential calls within one Loader. Sharing it across loaders
requires safe concurrent use. The caller owns resources; Loader does not close
them. A consistent source snapshot is the backend's responsibility. The built-in
engine still takes one ENV snapshot per load.

`LoaderOption` configures a loader. `WithEngine`, `WithDiagnostics` and `WithDiagnosticHandler` are general
options. `WithEnv`, `AllowUnknown` and registration-level `WithPrefix` require the
built-in engine: mixing them with a custom engine is an error, regardless of order.
Nil (including typed nil) or repeated `WithEngine` is an error.

### More adapters

Runnable and tested adapters for **env/v11, Viper, koanf, YAML and TOML** are in
[examples/engines](examples/engines/README.md). Each uses the backend's own tags
and decoding rules. Viper demonstrates YAML with `mapstructure` tags; koanf shows
TOML with layered sources. Their dependencies live in a separate examples module,
not in confmaker's core. See the examples for differences and error-handling limits.

## Errors and diagnostics

`Load` aggregates problems. `ConfigErrors(err)` extracts structured kinds and
configuration/field names; also handle the original error, since not every error
is a `ConfigError`. Ordinary causes support `errors.Is` and `errors.As`.
Sensitive parse causes are discarded.

```go
loader := confmaker.MakeLoader(confmaker.WithDiagnostics())
loader.Register[StoreConfig]("store")

err := loader.Load()
report := loader.Report() // Available even when loading fails.
```

Reports contain no values, defaults, error messages or panic payloads. The built-in
engine provides field-level detail; custom engines provide configuration outcomes.
Report snapshots are independent. See [Loader.Report] for states and [LoadReport]
for fields. A successful field does not imply that the entire configuration passed.

`WithDiagnosticHandler` enables collection and invokes a callback after the result
is published, on success or ordinary failure. Loading panic skips the callback,
propagates to the caller and leaves later load calls returning `ErrLoadPanicked`.
See [WithDiagnosticHandler] for callback and concurrency guarantees.

## Manifest and exports

`Manifest` describes the built-in schema without loading ENV. Add
`IncludeDefaults()` to evaluate fresh defaults explicitly. It never uses loaded
values. Custom engines return `ErrManifestUnsupported`.

### Prepare once, export in several formats

```go
document, err := loader.Manifest(confmaker.IncludeDefaults())
if err != nil {
	return err
}

if err := confexport.WriteMarkdown(writer, document); err != nil {
	return err
}
```

Import `github.com/uchaloop/confmaker/v2/confexport`. Its `WriteJSON`,
`WriteMarkdown` and `WriteEnvExample` functions accept a prepared manifest and
never load configuration or run user methods. The caller owns the writer and files.
Writer errors may leave partial output.

Schema errors return no manifest. Default-render failures stay in
`document.Problems`; inspect them when every default must render. Sensitive fields
are always redacted and are never marshaled. See [DefaultInfo] for default states
and [ManifestResult] for the result contract.

### Optional application flag

Import `github.com/uchaloop/confmaker/v2/confcli` to add
`-describe=env|markdown|json` to your application:

```go
described, err := confcli.Describe(
	loader,
	args,
	writer,
	confmaker.IncludeDefaults(),
)
if err != nil {
	return err
}

if described {
	return nil
}

return loader.Load()
```

Register configurations before this fragment. `args` contains command-line
arguments without the executable name, and `writer` is an `io.Writer`.
`Describe` accepts a loader or another `ManifestSource`, such as `confx.Modules`.
Defaults are evaluated only with explicit `IncludeDefaults()`.

The helper uses a private flag set. `-help` prints usage and returns `true, nil`;
unknown flags and positional arguments return errors. Handle errors before deciding
whether to continue startup. For applications with their own flags, use
`MakeDescribeFlag` on your `*flag.FlagSet`, then `Requested` and `Write`.
`confcli` does not load ENV, manage files or exit the process.

## Sensitive fields

Use the secret tag or a type implementing `interface { IsSensitive() }`. The marker
is inspected, never called; value and pointer receivers are recognized. Sensitive
collection elements or keys make the whole field sensitive. No opt-out exists.
The core does not depend on secret. `secret.Secret` implements the marker from
`secret/v2 v2.2.0`; older versions require explicit secret tags.

Tags only protect confmaker operations, not a returned ordinary string. Dedicated
secret types protect later formatting separately. Validate errors remain the
application's responsibility and must not contain secrets.

## Testing

Use `WithEnv` for isolated tests. It copies a complete replacement environment;
`WithEnv(nil)` means an empty environment, not a fallback to process ENV.

```go
cfg, err := confmaker.Load[struct {
	Port int `env:"PORT,required"`
}]("server", confmaker.WithEnv(map[string]string{
	"SERVER_PORT": "8080",
}))
if err != nil {
	t.Fatal(err)
}

if cfg.Port != 8080 {
	t.Fatalf("port = %d, want 8080", cfg.Port)
}
```

Core checks run independently of a workspace:

```sh
GOWORK=off go test -race ./...
GOWORK=off go vet ./...
```

Compatibility with the real secret package is checked in a separate
[integration module](integration/secret/README.md). The root test pattern does not
include nested modules. See [CONTRIBUTING.md](CONTRIBUTING.md) for development rules.

## Reference

- [Core API and runnable examples](https://pkg.go.dev/github.com/uchaloop/confmaker/v2)
- [Manifest exporters](https://pkg.go.dev/github.com/uchaloop/confmaker/v2/confexport)
- [Application describe flag](https://pkg.go.dev/github.com/uchaloop/confmaker/v2/confcli)
- [Fx adapter](https://github.com/uchaloop/confx): use the latest version of confx
- [Changelog](CHANGELOG.md), [contributing](CONTRIBUTING.md), [security policy](SECURITY.md)
- [MIT license](LICENSE)

[Loader.LoadContext]: https://pkg.go.dev/github.com/uchaloop/confmaker/v2#Loader.LoadContext
[Loader.Report]: https://pkg.go.dev/github.com/uchaloop/confmaker/v2#Loader.Report
[LoadReport]: https://pkg.go.dev/github.com/uchaloop/confmaker/v2#LoadReport
[WithDiagnosticHandler]: https://pkg.go.dev/github.com/uchaloop/confmaker/v2#WithDiagnosticHandler
[DefaultInfo]: https://pkg.go.dev/github.com/uchaloop/confmaker/v2#DefaultInfo
[ManifestResult]: https://pkg.go.dev/github.com/uchaloop/confmaker/v2#ManifestResult
