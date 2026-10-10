# Configuration engine examples

Five runnable adapters for confmaker's `Engine` interface. These are application
examples, not supported adapter packages or additions to confmaker's public API.
They form a separate Go module: none of these dependencies enter the core module.
The relative `replace` tests the confmaker checkout next to these examples.

## Run

Use Go 1.27 or later, from this directory:

```sh
GOWORK=off go run ./env11
GOWORK=off go run ./viper
GOWORK=off go run ./koanf
GOWORK=off go run ./yaml
GOWORK=off go run ./toml
```

Each program loads primary and replica configurations, then prints their ports,
labels and the `config` diagnostic detail level. Most print `8080 local` and
`9090 local`; koanf prints `override` labels from its second source.

YAML/TOML fixtures are embedded so the examples do not depend on the current
working directory or watch mutable files. In an application, read files or fetch
other sources before calling `makeEngine`, and handle those I/O errors there.
For these examples registration names explicitly select individual source snapshots.
They are not automatically ENV prefixes or document section names.

## Pick an example

| Example | Library / pinned version | Tags and source | What it demonstrates |
| --- | --- | --- | --- |
| [env11](env11) | [caarlos0/env](https://github.com/caarlos0/env) v11.4.1 | `env`, `envDefault`; explicit environment maps | Native defaults and no process ENV fallback |
| [viper](viper) | [Viper](https://github.com/spf13/viper) v1.21.0 | `mapstructure`; YAML | A configuration store decoded into a typed target |
| [koanf](koanf) | [koanf](https://github.com/knadh/koanf) v2.3.8 | `koanf`; TOML plus overrides | Source merging before decoding |
| [yaml](yaml) | [YAML](https://github.com/yaml/go-yaml) v3.0.5 | `yaml`; YAML | Direct strict decoding, one document only |
| [toml](toml) | [go-toml](https://github.com/pelletier/go-toml) v2.4.3 | `toml`; TOML | Direct strict decoding |

These represent different integration styles, not an exhaustive popularity ranking.
Viper and koanf are configuration managers; YAML and TOML packages are decoders.
The additional koanf provider/parser versions and all transitive versions are pinned
in this module's go.mod and go.sum.

## Responsibilities and differences

Confmaker still creates a fresh target, runs root `SetDefaults`, invokes the engine,
then runs root `Validate`. An error in any registration withholds every handle.
Reports contain configuration outcomes; Manifest is unsupported for custom engines.

The engine chooses tags and decoding semantics. Confmaker does not translate
`yaml`, `toml`, `mapstructure` or `koanf` tags, apply built-in ENV rules, or infer
source prefixes. For example, env11 accepts `envDefault`, which the built-in engine
rejects. A YAML file loaded by Viper uses `mapstructure` tags when filling structs.

These adapters intentionally differ:

- env11 uses a complete copied map per registration; native `envDefault` provides
  the primary port. Defaults and empty-value handling follow env11's rules.
- Viper uses a fresh instance per call and `UnmarshalExact`; it does not use global
  state, AutomaticEnv, file watching or remote providers. Viper's own type coercion
  rules still apply.
- koanf loads the base document, then an optional overlay. Later sources win per
  koanf's merge rules. Its ordinary Unmarshal does not enforce unknown-field errors.
- YAML enables `KnownFields(true)` and rejects additional documents.
- TOML enables `DisallowUnknownFields()`.

The tests demonstrate that an omitted label retains the root default in these
specific cases. They do not promise uniform behavior for zero values, nested maps,
all custom types, or every option the libraries expose. Any decoder may overwrite
values assigned by SetDefaults.

All adapters copy their inputs and create mutable parsers per call. Cancellation
is checked before decoding; these parsers are synchronous and cannot be interrupted
mid-decode. No target is retained, no live reload occurs, and no files are owned.

Backend errors are returned unchanged and can contain configuration values.
Confmaker's built-in secret tags and error sanitization do not apply to these
backends. Applications must sanitize errors before logging and configure their
chosen decoder for any dedicated secret types; these examples do not claim universal
Secret compatibility. Input-size budgets belong at the application's input boundary.

## Verify

```sh
GOWORK=off go mod tidy -diff
GOWORK=off go build ./...
GOWORK=off go vet ./...
GOWORK=off go test -race -count=1 ./...
```

Tests cover native tag mapping, two registrations, source snapshot isolation,
defaults, validation, malformed values, unknown registrations, cancellation before
decoding, configuration-level reports, unsupported manifests, and atomic publication.
Additional tests cover strict unknown-field handling, YAML document boundaries and
koanf overlays. Executable Go examples assert the programs' output.

The root `go test ./...` skips nested modules. CI checks this module separately
with `GOWORK=off`. No local workspace is required.
