# Changelog

All notable changes to this module are documented in this file.

Entries are grouped by version and change type, with the newest version first.

## [2.0.0]

### Added

- Added `Engine`, `EngineFunc` and `WithEngine` for application-owned backends without
  adding external parser dependencies. The built-in ENV behavior remains the default.
- Added cooperative `LoadContext` methods and generic helpers; cancellation of a waiter
  does not cancel the load, and cancellation of a started load is final.
- Added report `DetailLevel`: custom engines provide configuration outcomes; detailed
  field reports remain built-in. Custom-engine Manifest calls return
  `ErrManifestUnsupported` without running user code.
- Added pure `confexport` writers for JSON format version 2, Markdown and ENV examples.
- Added the optional `confcli` package for explicit describe-flag handling.

### Changed

- Updated the secret/v2 dependency to v2.2.0 for the structural sensitivity marker.
- **Breaking:** Renamed `EnvOption` to `LoaderOption`. ENV-specific options require the
  built-in engine and cannot be mixed with a custom engine.
- **Breaking:** Moved the module to `github.com/uchaloop/confmaker/v2`.
- **Breaking:** Replaced separate diagnostic receivers with `Loader.Report()` and
  parameterless `WithDiagnostics()`. Reports distinguish disabled, pending, running,
  succeeded, failed and panicked loads without exposing values or causes.
- Published the load result before invoking the diagnostic handler. Loading panics skip
  the handler and preserve completed work in the final report.
- **Breaking:** Changed manifest generation to describe schema by default.
  `IncludeDefaults()` evaluates fresh instances explicitly; default states replace
  `HasDefault`, and rendering problems retain the schema instead of failing the entire
  operation.
- **Breaking:** Restricted pointers to one level over scalar or text types; rejected
  pointer chains, collection pointers and pointers inside collections.
- Recognized sensitive fields through `env:",secret"` or the structural `IsSensitive()`
  marker without a production dependency on secret. Older secret versions require an
  explicit tag.

### Removed

- **Breaking:** Removed core writer methods and describe-flag helpers; use `confexport`
  and `confcli` with a prepared manifest.
- **Breaking:** Removed `Diagnostics`, `MakeDiagnostics` and the diagnostic receiver
  option.

## [1.0.1] - 2026-10-03

### Added

- Added integration tests for secret loading, manifest redaction and safe parse errors.
- Added a security policy; only the latest stable release is supported.

### Changed

- Set the minimum Go version to 1.27.0; CI uses the latest Go 1.27 patch release.
- Updated `secret/v2` to v2.1.0 to fix disclosure through nested formatting and logging.
  **Compatibility:** `Secret` is no longer comparable; JSON `null` now clears it.

## [1.0.0] - 2026-10-03

### Changed

- Published the first stable release of the explicit ENV configuration API, manifest and
  diagnostics.
- **Breaking:** Changed JSON fields to accept collections, including nested collections
  and scalar text types; ordinary structs are rejected at every depth.
- Changed JSON text values to consistently use text methods instead of JSON methods;
  manifest rejects text defaults without `MarshalText` with `ErrorDefaultRender`.
- Reduced diagnostic allocations and simplified tests around public behavior.
- Updated README and GoDoc, documented defaults outside the loader, and added
  contribution guidelines.
- Required passing CI for GitHub Releases; added module consistency checks, version tag
  validation and correct prerelease handling.

### Removed

- **Breaking:** Removed `WithDump`; use manifest for declarations and diagnostics for
  load sources and results without loaded values.

## [0.9.0] - 2026-10-01

### Added

- Added value-free load reports and completion handlers with `WithDiagnostics` and
  `WithDiagnosticHandler`; preserved completed-stage problems after panics and excluded
  dump writer error chains from report collection.
- Added `MakeDescribeFlag` for explicit manifest export format selection.
- Added `Handle.Name` and `Handle.BelongsTo` for registration metadata.

### Changed

- Improved Markdown rendering of ENV names, prefixes and Go types.

## [0.8.0] - 2026-10-01

### Added

- Added `Loader.Manifest()`, `ConfigManifest` and `envDescription` for complete
  configuration metadata without reading ENV or changing loader state.
- Added `.env.example`, versioned JSON and Markdown exports through `io.Writer`, with
  secret defaults omitted and strict manifest validation before writing.
- Added structured diagnostics (`ConfigError`, `ErrorKind`, `ConfigErrors`) and
  preserved standard scalar parser causes without changing messages or exposing secret
  parse causes.

### Changed

- **Breaking:** Required an explicit instance name in `Load`, `Loader.Register` and
  `Manifest`. Removed `WithName`, `ConfigNamer` and the `ConfigName()` fallback;
  `WithPrefix` still overrides only the ENV prefix. Renamed `Loader.Add` to
  `Loader.Register`.
- Simplified internal codecs, typed handle storage and shared helpers; refreshed naming,
  examples and GoDoc for the explicit-name API.

### Fixed

- Rejected non-reflexive plain ENV map keys, including NaN and custom keys containing
  NaN, as parse errors while preserving NaN values.
- Rejected cyclic pointer chains, nested ENV configs hidden behind multiple pointer or
  collection layers, and pointer keys in plain ENV maps at registration.
- Fixed default rendering to reject nil pointers at any depth in plain ENV collection
  elements and map keys or values.

## [0.7.0] - 2026-09-20

### Changed

- **Breaking:** Moved the Fx adapter to `github.com/uchaloop/confx`. Use that
  independent module; this repository contains only the core library.
- Updated documentation and CI for independent core and adapter releases.

## [0.6.2] - 2026-09-17

v0.6.0 and v0.6.1 were tagged on the v0.5.0 commit by mistake and hold the
v0.5.0 code; both are retracted. This is the release they were meant to be,
compared with v0.5.0.

### Added

- Added `Load[T]` to load one config in a call.
- Added `Loader` to load a set of configs: `MakeLoader`, the generic method
  `Loader.Add[T]` returning a `Handle[T]`, `Loader.Load` and `Handle.Value`. Values are
  handed out only when the whole set loaded; `ErrNotLoaded` and `ErrRegisteredAfterLoad`
  report a value read too early or registered too late. `Load` runs once, and a
  concurrent `Load` waits for its result. A `Loader` is safe for concurrent use and
  holds no lock while user code runs.
- Added `WithEnv` to load from a map instead of the process environment. The map is the
  complete environment (nil is empty) and is copied when passed.
- Added `ConfigNamer`: a config's `ConfigName` method gives its default instance name.
- Added `envFormat:"json"` to read structs, slices, arrays and maps, nested to any
  depth, from one variable with `encoding/json/v2`. A set variable replaces the whole
  field; unknown members are errors unless a JSON tag option says otherwise; `null` is
  accepted only for pointers, slices and maps; durations are strings. Secret types,
  `[]byte` and structs json/v2 rejects as objects are refused at registration. Errors
  name the place inside the value and keep the original `*json.SemanticError` or
  `*jsontext.SyntacticError` for `errors.As`.
- Added `Variable.NotEmpty` to separate `notEmpty` from `required`: `Required` means the
  variable must be set, `NotEmpty` that it must also be non-empty.
- Added fuzz tests to check that every plain field shape and JSON values never panic,
  and that a loaded value renders to text that loads back to the same value.

### Changed

- **Breaking:** Moved the Fx adapter `confx` out of this module into its own module,
  `github.com/uchaloop/confmaker/confx`, in the same repository and under the same
  import path. This module no longer depends on Fx; Fx applications add the confx
  module, whose changes are in `confx/CHANGELOG.md`.
- **Breaking:** Moved loading, options, the manifest and the dump to package
  `confmaker`. `confx.WithName`, `confx.WithPrefix`, `confx.WithDump`,
  `confx.AllowUnknown` and `confx.Manifest` become `confmaker.WithName`,
  `confmaker.WithPrefix`, `confmaker.WithDump`, `confmaker.AllowUnknown` and
  `confmaker.Manifest`.
- **Breaking:** Changed the instance name from a positional argument to an option:
  `confmaker.WithName("app")`, or a `ConfigName() string` method on the config.
- **Breaking:** Typed options by what they configure. A `ConfigOption` (`WithName`,
  `WithPrefix`) configures one config; an `EnvOption` (`WithEnv`, `WithDump`,
  `AllowUnknown`) configures a whole load. Passing the wrong kind does not compile.
- **Breaking:** Renamed the env tag option `require` to `required`, as in caarlos0/env;
  `require` is refused as an unknown option.
- **Breaking:** Rejected `envSeparator` on a field that is not a slice or map read in
  the plain syntax, and `envKeyValSeparator` on a field that is not such a map; both are
  refused without a variable name. They used to be ignored.
- **Breaking:** Rejected variable names containing `=` or NUL.
- Joined every load problem into one error: invalid registrations, conflicts, unknown
  variables, variables that did not parse, and the `Validate` errors of every config
  whose variables parsed. The report is ordered by instance name and prefix, not by
  registration order.
- Used one environment snapshot for loading, the unknown-variable check and the dump.
- Changed `SetDefaults` to run once per loaded config, with the dump describing that
  same instance. The dump is written even when loading fails; a dump that cannot be
  rendered or written is reported with the other problems. Conflicting registrations
  stop loading before `SetDefaults` and the dump run.
- Moved declaration checks to registration, before any default or variable is read. Each
  registration compiles one immutable schema. A loaded value is kept by its loader and
  returned by copy, but maps, slices and pointers inside it are shared by every caller.
- Changed manifest default rendering to use `MarshalText` or the field's plain syntax
  instead of `String`; a type read through `UnmarshalText` needs `MarshalText` only for
  the manifest and the dump. Marshal errors are returned, and a collection default its
  separators could not carry back is refused.
- Improved map parsing to about twice the speed with a sixth of the allocations, and
  limited typo-hint computation to the diagonal band of the edit distance.
- Split documentation by purpose: the README to get started, the package documentation
  for the exact rules.

### Fixed

- Fixed disclosure of secrets behind two or more pointers (`***secret.Secret`) by the
  dump, and acceptance of slices or maps of such secrets. Secrets are recognised behind
  any number of pointers.
- Rejected repeated instance names even across different config types or prefixes, so
  matching names no longer hide variable conflicts.
- Escaped control characters in dump values without exposing secrets.

## [0.5.0] - 2026-09-01

Two declarations that used to pass now fail when the config is bound: a secret in
a slice or a map, and `envPrefix` on anything but a struct nested by value.

### Added

- Added a startup check that rejects a variable claimed by two instances, naming both -
  one instance's prefix running into another's arrives at a single variable name.
- Added validation rejecting `envPrefix` on anything but a struct nested by value. It
  extended nothing, and on a non-struct field it dropped the field from the config.

### Changed

- Bounded the suggestion calculation for an unknown variable: a band around the
  diagonal, an early exit, and rows reused between candidates. Against 120 declared
  variables, 148 to 52 microseconds and 242 allocations to 4. A test checks every result
  against the full matrix over a quarter of a million random pairs.
- Changed prefix derivation to fold an instance name a byte at a time. `NewReplacer`
  compiled a trie per call - an eighth of what a manifest allocated - to turn "store"
  into "STORE_".
- Changed struct traversal to use `reflect.Value.Fields` and `reflect.Type.Fields`. The
  iterators cost eight allocations per instance at startup; reading the traversal
  without index arithmetic is worth more.
- Allocated bindings once against the field count and reduced value rendering to one
  interface conversion instead of two. Describing a twenty-variable config went from 8.1
  to 5.2 microseconds.
- Changed `parseText` to assert through `reflect.TypeAssert`, removing a linter
  exemption. Sorting goes through `slices`, and `sort` is gone.
- Changed the build toolchain to Go 1.27. Nothing in 1.27 is used here, so this is a
  decision about the toolchain rather than a requirement; a module that depends on this
  one has to declare 1.27 as well.
- Expanded benchmarks to cover the three traversals a startup runs.
- Updated the README with declaration rejection rules, cross-instance Module checks and
  a link to the `validate` module.
- Clarified that `WithDump` writes the value a variable carries: it prints the text of
  the variable, so a duration set to 3600s appears as 3600s.
- Documented that `envPrefix` was deliberately unchecked, unlike an instance prefix.
- Clarified the note on registering `Module` early: Fx runs invocations in the order
  they are registered.

### Removed

- Moved the `validate` package to `github.com/uchaloop/validate`. Importing it dragged
  this module - the loader - into every library that declares a config.

### Fixed

- Fixed a panic when rendering the default of a nil pointer to a type with its own text
  form. A `*time.Time` field brought the application down on the first bind, whether or
  not its variable was set.
- Rejected secrets in slices or maps instead of printing them: `[]secret.Secret` was not
  recognised as a secret, so `WithDump` wrote its variable out in full.
- Fixed parsing of slices of named byte-sized types. The element was matched by kind, so
  `[]Status` with `type Status uint8` looked like `[]byte`.
- Fixed rendering through `MarshalText` with a pointer receiver. The parser was chosen
  from the method set of `*T` and the rendering from `T`.
- Fixed contextual labels on joined errors to appear on every line rather than only the
  first.

## [0.4.2] - 2026-08-25

### Fixed

- Removed test dependence on the host environment. Module scans the real environment, so
  a test instance named "api" owned API_ and reported whatever was exported under it -
  API_TIMEOUT_MS, on the machine where this surfaced. Test instances are named so that
  no environment can hold their prefix. The one example that reads the process
  environment no longer declares an output, because what it prints depends on the
  machine.

## [0.4.1] - 2026-08-25

### Added

- Added a logo and runnable examples in the package documentation: wiring one instance,
  a second instance of the same type, and generating from the manifest. They are
  compiled by `go test`, so an example cannot drift from the API it demonstrates.

### Changed

- Moved the rules and their rationale from the README into the package documentation,
  where a Go developer reads them - in an editor and on pkg.go.dev. The README is the
  landing page: what the library is, what it catches, what it prints, what it generates.

## [0.4.0] - 2026-08-25

Configuration is read from the environment only. Files are gone, and with them
the environment-named configuration groups that
[12factor III](https://12factor.net/config) argues against. This module now
reads and parses the environment itself, which leaves Fx and the secret type as
its only dependencies.

### Added

- Added configuration defaults through `SetDefaults()`, called before applying the
  environment, so a library declares them in code its tests and its callers can see. A
  variable that is not set leaves its field exactly as `SetDefaults` left it.
- Added an environment check in `confx.Module` against the manifest registered by
  `Provide` calls: a variable that starts with a prefix the application owns but matches
  no field fails the start, with a suggestion of the name it likely misspells. This
  replaces the strict file decoding that caught typos before. `confx.AllowUnknown`
  exempts prefixes from that check, for a deployment that shares one environment between
  several binaries.
- Added `confx.WithDump` to write every variable the application reads, its type, its
  current value, and where that value came from. Secrets are reported as set or unset
  and never printed.
- Added `confx.Manifest[T](name) ([]Variable, error)` to return that same list without
  building an application - name, type, whether it is required, whether it holds a
  secret, and the default `SetDefaults` establishes, rendered as text the variable could
  carry back. A `.env.example`, a ConfigMap or a documentation table is generated from
  the config type itself. It takes the options `Provide` takes, and refuses a
  declaration the application would refuse rather than returning an empty list.
- Added map parsing from a single variable, split with `envSeparator` (default `,`) and
  `envKeyValSeparator` (default `:`). A duplicate key is an error, and a key or value
  padded with whitespace is reported rather than trimmed.

### Changed

- Renamed `confx.ProvideNoFileDefault[T](name)` to `confx.Provide[T](name)` and
  `confx.ProvideNoFile[T](name)` to `confx.ProvideNamed[T](name)`. Both take an instance
  name rather than a file section; the environment prefix is derived from it as before.
- Renamed `confx.WithEnvPrefix` to `confx.WithPrefix` and validated its prefix:
  upper-case letters, digits and underscores, ending with an underscore.
  `WithPrefix("")` is refused, where it used to read every `env` tag unprefixed and
  leave that instance outside the strict check.
- Changed the mandatory tag option to `require` instead of `required` and rejected
  unknown options - a misspelled one used to leave the field quietly optional.
- Added instance-name validation: it gives both the variable prefix and the Fx tag, so
  it may hold only lowercase letters, digits and `_ - .`, and may not start or end with
  a separator. A name with a space in it read nothing and answered to a tag nobody asked
  for.
- Defined supported field types as `string`, `bool`, every sized integer and float,
  `time.Duration`, any `encoding.TextUnmarshaler`, pointers to those, and slices and
  maps of them. `complex`, `uintptr` and `[]byte` are refused rather than guessed at,
  and the parser for a field is chosen from its type when the config is bound, so a
  field that could never be read fails on the first start.

### Removed

- Removed the `confmaker` root package: `Load`, `LoadDir`, `Required`, `Registry`,
  `MakeRegistry` and `ResolveSecret`. The module is now `confmaker/confx` and
  `confmaker/validate`.
- Removed `confx.LoadModule`, `confx.LoadDir` and `confx.Source`. Configuration files,
  the `ENVIRONMENT` variable and the `common.toml` / `dev.toml` / `stage.toml` /
  `prod.toml` convention are no longer read.
- Removed the `koanf` struct tag. It stays inert where libraries still declare it.
- Removed the `envDefault` tag. Declaring one is an error naming `SetDefaults` as its
  replacement, so a default cannot silently disappear during the migration.
- Removed nesting a config through a pointer, a slice, a map or an array. How many
  variables such a field reads cannot be known from its type, which is what the strict
  check and the manifest rest on; nest by value instead.
- Removed tag options this module never used: `init`, `expand`, `file` and `unset`.
- Removed every dependency but Fx and the secret type: `caarlos0/env`, `koanf/v2` with
  its TOML parser, file provider and `koanf/maps`, `go-viper/mapstructure/v2`,
  `pelletier/go-toml/v2`, `fsnotify`, `mitchellh/copystructure`,
  `mitchellh/reflectwalk`, and `uchaloop/utilfx`.

### Fixed

- Fixed handling of env tags with options but no variable name, such as
  `env:",require"`, which previously disappeared as untagged fields. It is now refused.
- Fixed silent acceptance of two fields resolving to one variable, such as nesting the
  same struct twice without a distinct `envPrefix`. The declaration is now refused,
  naming both fields.

## [0.3.1] - 2026-08-06

### Changed

- Reworked the README as concise, user-focused documentation.

## [0.3.0] - 2026-08-06

### Added

- Added `confmaker.LoadDir` and `confmaker/confx.LoadDir` for an opt-in,
  convention-based configuration mode. They require `ENVIRONMENT` (`dev`, `stage`,
  `prod`, or `prd` as a `prod` alias), load optional `common.toml` first, then the
  required canonical environment file.

## [0.2.0] - 2026-08-04

### Added

- Added `confmaker/confx`: `ProvideNoFile[T](name)` and `ProvideNoFileDefault[T](name)`
  build a config from its zero value without a file or `LoadModule`, fill its
  `env`-tagged fields, validate it, and provide it tagged or untagged.
- Added file-decoding protection that clears existing `secret.Secret` values and ignores
  values accidentally mapped to that type, so configuration files can never populate
  secrets.
- Updated the secret integration to the opaque `github.com/uchaloop/secret/v2` API and
  its sealed `secret.Value` marker.

## [0.1.0] - 2026-08-04

### Added

- Added application-level configuration for Go services, split so infrastructure
  libraries stay dependency-light.
- Added `confmaker/validate`: `Errors` accumulator (`Add`/`Addf`/`Require`/`Err`) so a
  `Validate` method reports every problem at once. No external dependencies.
- Added `confmaker.Load(dst, paths...)`: strict TOML decoding (unknown-key rejection,
  duration parsing, no weak typing, base + overlay merging). With no paths, reads
  `config.toml` from the current directory.
- Added `confmaker/confx`: Fx wiring. `LoadModule(paths...)` loads the file once (or
  `config.toml` from the current directory); `ProvideDefault[T](section)` and
  `Provide[T](section, name)` decode a section into a library's typed config, fill its
  `env`-tagged fields (prefix from the section/name, overridable with `WithEnvPrefix`),
  validate it, and provide it into the container - untagged or tagged `name:"<name>"`.
- Added `confmaker.Required` and `confmaker.MakeRegistry[T]`: helpers for mandatory and
  dynamic named instances.
- Added `confmaker.ResolveSecret(envName)`: read a single secret from the environment;
  returns the masked type from the separate zero-dependency `github.com/uchaloop/secret`
  module, and the error names only the variable, never the value.

[2.0.0]: https://github.com/uchaloop/confmaker/compare/v1.0.1...v2.0.0
[1.0.1]: https://github.com/uchaloop/confmaker/compare/v1.0.0...v1.0.1
[1.0.0]: https://github.com/uchaloop/confmaker/compare/v0.9.0...v1.0.0
[0.9.0]: https://github.com/uchaloop/confmaker/compare/v0.8.0...v0.9.0
[0.8.0]: https://github.com/uchaloop/confmaker/compare/v0.7.0...v0.8.0
[0.7.0]: https://github.com/uchaloop/confmaker/compare/v0.6.2...v0.7.0
[0.6.2]: https://github.com/uchaloop/confmaker/compare/v0.5.0...v0.6.2
[0.5.0]: https://github.com/uchaloop/confmaker/compare/v0.4.2...v0.5.0
[0.4.2]: https://github.com/uchaloop/confmaker/compare/v0.4.1...v0.4.2
[0.4.1]: https://github.com/uchaloop/confmaker/compare/v0.4.0...v0.4.1
[0.4.0]: https://github.com/uchaloop/confmaker/compare/v0.3.1...v0.4.0
[0.3.1]: https://github.com/uchaloop/confmaker/compare/v0.3.0...v0.3.1
[0.3.0]: https://github.com/uchaloop/confmaker/compare/v0.2.0...v0.3.0
[0.2.0]: https://github.com/uchaloop/confmaker/releases/tag/v0.2.0
[0.1.0]: https://github.com/uchaloop/confmaker/releases/tag/v0.1.0
