# confmaker

Explicit, centralized ENV configuration for Go applications. Packages declare typed
configuration; the application registers instances, loads the whole set once and
passes checked values to consumers. Go 1.27.0 or later is required.

This branch prepares **v2**. It is not a published release. See
[Migration to v2](docs/migration-v2.md) for incompatible changes and release order.

## Loading

```go
import "github.com/uchaloop/confmaker/v2"

type StoreConfig struct {
    Host string `env:"HOST,notEmpty"`
    Timeout time.Duration `env:"TIMEOUT"`
}
func (c *StoreConfig) SetDefaults() { c.Timeout = 30 * time.Second }
func (c StoreConfig) Validate() error {
    if c.Timeout <= 0 { return fmt.Errorf("timeout must be positive") }
    return nil
}

loader := confmaker.MakeLoader()
store := loader.Register[StoreConfig]("postgres")
if err := loader.Load(); err != nil { return err }
cfg, err := store.Value()
```

The example uses standard `fmt` and `time`. For one config use
`confmaker.Load[StoreConfig]("postgres")`. Config types need no confmaker import.
`WithEnv(map[string]string{...})` replaces the entire process ENV and copies the
map. A nil map means empty ENV. No files or external stores are read.

Instance names contain lowercase letters, digits, `_`, `-`, `.` and start/end
with a letter or digit. `read-replica` produces `READ_REPLICA_`.
`WithPrefix("READ_DB_")` overrides the prefix, not identity. Names, prefixes and
full variable names must not collide. Prefix overrides contain uppercase letters,
digits or underscores and end in `_`.

Register everything before Load. Load checks every registration, including unused
ones, and takes one ENV snapshot. Conflicts stop processing before defaults;
other errors are aggregated. Values become available only if the whole set succeeds.
Repeat Load returns the same result; concurrent callers wait for its publication.
Value does not load. A late registration returns a rejected handle.
Name and BelongsTo expose identity and ownership without loading.

Returned structs share nested maps, slices and pointers: treat them as read-only.
Loader is safe for concurrent use. User defaults, parsers and validators must not
recursively call the same loader's Load.

## Field rules

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

## Errors and diagnostics

Load returns joined errors. ConfigErrors extracts structured ErrorKind,
InstanceName, VariableName and FieldPath. Handle the original error too:
lifecycle and writer errors need not be ConfigError. Ordinary causes are preserved
for errors.Is/As. Sensitive parse causes and inputs are discarded.

```go
loader := confmaker.MakeLoader(confmaker.WithDiagnostics())
// Register configurations, then:
err := loader.Load()
report := loader.Report() // Also available on error.
```

Report is an independent, value-free snapshot. It includes no defaults, error
messages, causes or panic values. States are disabled, not_started, in_progress,
succeeded, failed and panicked. Disabled/not_started/in_progress have no details.
Field success means parsing succeeded, not that validation or the whole set passed.
After panic, completed work stays visible, interrupted work is marked interrupted,
and untouched work remains not_processed. Default/zero sources distinguish a
nonzero value after defaults from a zero value, not explicit assignments.

WithDiagnosticHandler(func(LoadReport)) also enables collection. It runs once on
normal success or failure, after the result and report are published. It can read
handles, Report and call Load again. The executing Load waits for the handler;
other waiting callers may return earlier. Loading panic skips the handler,
propagates to the executing caller and makes later Load return ErrLoadPanicked.
Handler panic propagates but does not change the published load result.

## Manifest and exports

```go
schema, err := loader.Manifest() // No user methods or ENV reads.
document, err := loader.Manifest(confmaker.IncludeDefaults())
// Handle err before using either result.
err = confexport.WriteMarkdown(writer, document)
```

Import `github.com/uchaloop/confmaker/v2/confexport` for WriteJSON,
WriteMarkdown and WriteEnvExample. Exporters consume a prepared ManifestResult,
never a loader. Reuse the same snapshot for multiple formats. JSON format version 2
preserves default states and structured render problems. Writer errors may leave
partial output. File handling belongs to the caller.

ManifestResult contains Configs and Problems. Schema/declaration conflicts return
an error with no result. IncludeDefaults evaluates fresh instances once per config;
render failure marks that field unrenderable and appends a structural problem
without removing the schema. Inspect Problems if all defaults must render.
No error messages or causes are included in these problems.

Default states are not_evaluated, zero, rendered, redacted and unrenderable.
Only rendered has meaningful Text, which may be empty. Zero cannot distinguish an
explicit default from no assignment. Sensitive fields are always redacted and
never marshaled. A custom parser needs MarshalText only to render nonzero defaults.
Panic from defaults/marshalers propagates without changing Load or its diagnostics.

Manifest[T](name, options...) returns the same model with one configuration.
Manifest snapshots are independent and can run before or after loading. Concurrent
IncludeDefaults/Load calls require user methods to support concurrent invocation.

The optional `confcli.MakeDescribeFlag` handles `-describe=env|markdown|json`.
After parsing and checking Requested, build the manifest explicitly, then call
`describe.Write(writer, document)`. It never loads or terminates the application.

## Sensitive fields

Use the secret tag or a type implementing `interface { IsSensitive() }`. The marker
is inspected, never called; value and pointer receivers are recognized. Sensitive
collection elements or keys make the whole field sensitive. No opt-out exists.
The core does not import secret/v2. The companion secret.Secret needs the new
IsSensitive marker; older versions require explicit secret tags.

Tags only protect confmaker operations, not a returned ordinary string. Dedicated
secret types protect later formatting separately. Validate errors remain the
application's responsibility and must not contain secrets.

## Fx and development

[confx](https://github.com/uchaloop/confx) owns Fx wiring, not parsing or validation.
Use its matching v2-compatible development version; earlier releases use v1 types.

Run `go test -race ./...` and `go vet ./...`. Integration with the unpublished
marker and v2 adapter is verified through a local Go workspace; publication must
follow the migration guide. Production dependency checks use `go list -deps .`.
