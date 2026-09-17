# Changelog

All notable changes to this module are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this module adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.6.0] - 2026-09-17

### Changed

- **Breaking:** the Fx adapter `confx` moved out of this module into its own,
  `github.com/uchaloop/confmaker/confx`, in the same repository and under the same
  import path. This module no longer depends on Fx; Fx applications add the confx
  module, whose changes are in `confx/CHANGELOG.md`.
- **Breaking:** loading, options, the manifest and the dump live in package
  `confmaker`. `confx.WithName`, `confx.WithPrefix`, `confx.WithDump`,
  `confx.AllowUnknown` and `confx.Manifest` become `confmaker.WithName`,
  `confmaker.WithPrefix`, `confmaker.WithDump`, `confmaker.AllowUnknown` and
  `confmaker.Manifest`.
- **Breaking:** the instance name is an option, not a positional argument:
  `confmaker.WithName("app")`, or a `ConfigName() string` method on the config.
- **Breaking:** options are typed by what they configure. A `ConfigOption`
  (`WithName`, `WithPrefix`) configures one config; an `EnvOption` (`WithEnv`,
  `WithDump`, `AllowUnknown`) configures a whole load. Passing the wrong kind does
  not compile.
- **Breaking:** the env tag option `require` is now `required`, as in
  caarlos0/env; `require` is refused as an unknown option.
- **Breaking:** `envSeparator` is refused on a field that is not a slice or map
  read in the plain syntax, and `envKeyValSeparator` on a field that is not such a
  map; both are refused without a variable name. They used to be ignored.
- **Breaking:** a variable name containing `=` or NUL is refused.
- A load joins every problem into one error: invalid registrations, conflicts,
  unknown variables, variables that did not parse, and the `Validate` errors of
  every config whose variables parsed. The report is ordered by instance name and
  prefix, not by registration order.
- A load takes one snapshot of the environment, shared by loading, the check for
  unknown variables and the dump.
- `SetDefaults` runs once per loaded config, and the dump describes that same
  instance. The dump is written even when loading fails; a dump that cannot be
  rendered or written is reported with the other problems. Conflicting
  registrations stop loading before `SetDefaults` and the dump run.
- Declarations are checked when a config is registered, before any default or
  variable is read. Each registration compiles one immutable schema. A loaded
  value is kept by its loader and returned by copy, but maps, slices and pointers
  inside it are shared by every caller.
- Manifest defaults are rendered with `MarshalText` or the field's plain syntax,
  not `String`; a type read through `UnmarshalText` needs `MarshalText` only for
  the manifest and the dump. Marshal errors are returned, and a collection default
  its separators could not carry back is refused.
- Map variables parse about twice as fast with a sixth of the allocations, and
  typo hints compute only the diagonal band of the edit distance.
- Documentation is split by purpose: the README to get started, the package
  documentation for the exact rules.

### Added

- `Load[T]` loads one config in a call.
- `Loader` loads a set of configs: `MakeLoader`, the generic method
  `Loader.Add[T]` returning a `Handle[T]`, `Loader.Load` and `Handle.Value`.
  Values are handed out only when the whole set loaded; `ErrNotLoaded` and
  `ErrRegisteredAfterLoad` report a value read too early or registered too late.
  `Load` runs once, and a concurrent `Load` waits for its result. A `Loader` is
  safe for concurrent use and holds no lock while user code runs.
- `WithEnv` loads from a map instead of the process environment. The map is the
  complete environment (nil is empty) and is copied when passed.
- `ConfigNamer`: a config's `ConfigName` method gives its default instance name.
- `envFormat:"json"` reads structs, slices, arrays and maps, nested to any depth,
  from one variable with `encoding/json/v2`. A set variable replaces the whole
  field; unknown members are errors unless a JSON tag option says otherwise; `null`
  is accepted only for pointers, slices and maps; durations are strings. Secret
  types, `[]byte` and structs json/v2 rejects as objects are refused at
  registration. Errors name the place inside the value and keep the original
  `*json.SemanticError` or `*jsontext.SyntacticError` for `errors.As`.
- `Variable.NotEmpty` separates `notEmpty` from `required`: `Required` means the
  variable must be set, `NotEmpty` that it must also be non-empty.
- Fuzz tests check that every plain field shape and JSON values never panic, and
  that a loaded value renders to text that loads back to the same value.

### Fixed

- A secret behind two or more pointers (`***secret.Secret`) was printed by the
  dump, and a slice or map of such secrets was accepted. Secrets are recognised
  behind any number of pointers.
- Repeated instance names are refused even across different config types or
  prefixes, so matching names no longer hide variable conflicts.
- Dump values escape control characters without exposing secrets.

## [0.5.0] - 2026-09-01

Two declarations that used to pass now fail when the config is bound: a secret in
a slice or a map, and `envPrefix` on anything but a struct nested by value.

### Fixed

- A nil pointer to a type with its own text form no longer panics when its
  default is rendered. A `*time.Time` field brought the application down on the
  first bind, whether or not its variable was set.
- A secret in a slice or a map is refused rather than printed: `[]secret.Secret`
  was not recognised as a secret, so `WithDump` wrote its variable out in full.
- A slice of a named byte-sized type is read instead of refused. The element was
  matched by kind, so `[]Status` with `type Status uint8` looked like `[]byte`.
- A type whose `MarshalText` takes a pointer receiver renders through it. The
  parser was chosen from the method set of `*T` and the rendering from `T`.
- A joined error a stage wrapped in context of its own is labelled on every line
  rather than only the first.

### Added

- A variable two instances both claim fails the start, naming both - one
  instance's prefix running into another's arrives at a single variable name.
- `envPrefix` on anything but a struct nested by value is refused. It extended
  nothing, and on a non-struct field it dropped the field from the config.

### Removed

- The `validate` package moved to `github.com/uchaloop/validate`. Importing it
  dragged this module - the loader - into every library that declares a config.

### Changed

- The suggestion for an unknown variable is bounded rather than computed in full:
  a band around the diagonal, an early exit, and rows reused between candidates.
  Against 120 declared variables, 148 to 52 microseconds and 242 allocations to
  4. A test checks every result against the full matrix over a quarter of a
  million random pairs.
- An instance name is folded into its prefix a byte at a time. `NewReplacer`
  compiled a trie per call - an eighth of what a manifest allocated - to turn
  "store" into "STORE_".
- The traversals walk a struct through `reflect.Value.Fields` and
  `reflect.Type.Fields`. The iterators cost eight allocations per instance at
  startup; reading the traversal without index arithmetic is worth more.
- Bindings are allocated once against the field count, and a value renders
  through one interface conversion rather than two. Describing a twenty-variable
  config went from 8.1 to 5.2 microseconds.
- `parseText` asserts through `reflect.TypeAssert`, which takes a linter
  exemption off the line. Sorting goes through `slices`, and `sort` is gone.
- The module is built with Go 1.27. Nothing in 1.27 is used here, so this is a
  decision about the toolchain rather than a requirement; a module that depends
  on this one has to declare 1.27 as well.
- Benchmarks cover the three traversals a startup runs.
- The README lists what a declaration is refused for and the checks Module runs
  across instances, and points at the `validate` module.
- `WithDump` is described as writing the value a variable carries: it prints the
  text of the variable, so a duration set to 3600s appears as 3600s.
- `envPrefix` is documented as deliberately unchecked, unlike an instance prefix.
- The note on registering `Module` early names what the order is about: Fx runs
  invocations in the order they are registered.

## [0.4.2] - 2026-08-25

### Fixed

- The tests no longer depend on the environment of the machine that runs them.
  Module scans the real environment, so a test instance named "api" owned API_
  and reported whatever was exported under it - API_TIMEOUT_MS, on the machine
  where this surfaced. Test instances are named so that no environment can hold
  their prefix. The one example that reads the process environment no longer
  declares an output, because what it prints depends on the machine.

## [0.4.1] - 2026-08-25

### Added

- A logo, and runnable examples in the package documentation: wiring one
  instance, a second instance of the same type, and generating from the
  manifest. They are compiled by `go test`, so an example cannot drift from the
  API it demonstrates.

### Changed

- The rules and the reasons behind them moved from the README into the package
  documentation, where a Go developer reads them - in an editor and on
  pkg.go.dev. The README is the landing page: what the library is, what it
  catches, what it prints, what it generates.

## [0.4.0] - 2026-08-25

Configuration is read from the environment only. Files are gone, and with them
the environment-named configuration groups that
[12factor III](https://12factor.net/config) argues against. This module now
reads and parses the environment itself, which leaves Fx and the secret type as
its only dependencies.

### Added

- A config establishes its own defaults through a `SetDefaults()` method, called
  before the environment is applied, so a library declares them in code its tests
  and its callers can see. A variable that is not set leaves its field exactly as
  `SetDefaults` left it.
- `confx.Module` checks the environment against the manifest the `Provide` calls
  register: a variable that starts with a prefix the application owns but matches
  no field fails the start, with a suggestion of the name it likely misspells.
  This replaces the strict file decoding that caught typos before.
  `confx.AllowUnknown` exempts prefixes from that check, for a deployment that
  shares one environment between several binaries.
- `confx.WithDump` writes every variable the application reads, its type, its
  current value, and where that value came from. Secrets are reported as set or
  unset and never printed.
- `confx.Manifest[T](name) ([]Variable, error)` returns that same list without
  building an application - name, type, whether it is required, whether it holds
  a secret, and the default `SetDefaults` establishes, rendered as text the
  variable could carry back. A `.env.example`, a ConfigMap or a documentation
  table is generated from the config type itself. It takes the options `Provide`
  takes, and refuses a declaration the application would refuse rather than
  returning an empty list.
- Maps are read from a single variable, split with `envSeparator` (default `,`)
  and `envKeyValSeparator` (default `:`). A duplicate key is an error, and a key
  or value padded with whitespace is reported rather than trimmed.

### Changed

- `confx.ProvideNoFileDefault[T](name)` is now `confx.Provide[T](name)`, and
  `confx.ProvideNoFile[T](name)` is now `confx.ProvideNamed[T](name)`. Both take
  an instance name rather than a file section; the environment prefix is derived
  from it as before.
- `confx.WithEnvPrefix` is now `confx.WithPrefix`, and the prefix it is given is
  checked: upper-case letters, digits and underscores, ending with an underscore.
  `WithPrefix("")` is refused, where it used to read every `env` tag unprefixed
  and leave that instance outside the strict check.
- The tag option that marks a variable as mandatory is `require`, not
  `required`, and an option the tag does not define is an error - a misspelled
  one used to leave the field quietly optional.
- An instance name is checked: it gives both the variable prefix and the Fx tag,
  so it may hold only lowercase letters, digits and `_ - .`, and may not start or
  end with a separator. A name with a space in it read nothing and answered to a
  tag nobody asked for.
- Field types are `string`, `bool`, every sized integer and float,
  `time.Duration`, any `encoding.TextUnmarshaler`, pointers to those, and slices
  and maps of them. `complex`, `uintptr` and `[]byte` are refused rather than
  guessed at, and the parser for a field is chosen from its type when the config
  is bound, so a field that could never be read fails on the first start.

### Removed

- The `confmaker` root package: `Load`, `LoadDir`, `Required`, `Registry`,
  `MakeRegistry` and `ResolveSecret`. The module is now `confmaker/confx` and
  `confmaker/validate`.
- `confx.LoadModule`, `confx.LoadDir` and `confx.Source`. Configuration files,
  the `ENVIRONMENT` variable and the `common.toml` / `dev.toml` / `stage.toml` /
  `prod.toml` convention are no longer read.
- The `koanf` struct tag. It stays inert where libraries still declare it.
- The `envDefault` tag. Declaring one is an error naming `SetDefaults` as its
  replacement, so a default cannot silently disappear during the migration.
- Nesting a config through a pointer, a slice, a map or an array. How many
  variables such a field reads cannot be known from its type, which is what the
  strict check and the manifest rest on; nest by value instead.
- Tag options this module never used: `init`, `expand`, `file` and `unset`.
- Every dependency but Fx and the secret type: `caarlos0/env`, `koanf/v2` with
  its TOML parser, file provider and `koanf/maps`, `go-viper/mapstructure/v2`,
  `pelletier/go-toml/v2`, `fsnotify`, `mitchellh/copystructure`,
  `mitchellh/reflectwalk`, and `uchaloop/utilfx`.

### Fixed

- An env tag that carried options but named no variable - `env:",require"` -
  read as an untagged field, so a field its author meant to configure
  disappeared. It is now refused.
- Two fields resolving to one variable - nesting the same struct twice and
  leaving `envPrefix` off the second - were both filled from it, in silence. The
  declaration is now refused, naming both fields.

## [0.3.1] - 2026-08-06

### Changed

- Reworked the README as concise, user-focused documentation.

## [0.3.0] - 2026-08-06

### Added

- `confmaker.LoadDir` and `confmaker/confx.LoadDir` add an opt-in,
  convention-based configuration mode. They require `ENVIRONMENT` (`dev`,
  `stage`, `prod`, or `prd` as a `prod` alias), load optional `common.toml`
  first, then the required canonical environment file.

## [0.2.0] - 2026-08-04

### Added

- `confmaker/confx`: `ProvideNoFile[T](name)` and
  `ProvideNoFileDefault[T](name)` build a config from its zero value without a
  file or `LoadModule`, fill its `env`-tagged fields, validate it, and provide it
  tagged or untagged.
- File decoding clears existing `secret.Secret` values and ignores values
  accidentally mapped to that type, so configuration files can never populate
  secrets.
- Updated the secret integration to the opaque
  `github.com/uchaloop/secret/v2` API and its sealed `secret.Value` marker.

## [0.1.0] - 2026-08-04

### Added

- Application-level configuration for Go services, split so infrastructure
  libraries stay dependency-light.
- `confmaker/validate`: `Errors` accumulator (`Add`/`Addf`/`Require`/`Err`) so a
  `Validate` method reports every problem at once. No external dependencies.
- `confmaker.Load(dst, paths...)`: strict TOML decoding (unknown-key rejection,
  duration parsing, no weak typing, base + overlay merging). With no paths, reads
  `config.toml` from the current directory.
- `confmaker/confx`: Fx wiring. `LoadModule(paths...)` loads the file once (or
  `config.toml` from the current directory); `ProvideDefault[T](section)` and
  `Provide[T](section, name)` decode a section into a library's typed config, fill
  its `env`-tagged fields (prefix from the section/name, overridable with
  `WithEnvPrefix`), validate it, and provide it into the container - untagged or
  tagged `name:"<name>"`.
- `confmaker.Required` and `confmaker.MakeRegistry[T]`: helpers for mandatory and
  dynamic named instances.
- `confmaker.ResolveSecret(envName)`: read a single secret from the environment;
  returns the masked type from the separate zero-dependency
  `github.com/uchaloop/secret` module, and the error names only the variable,
  never the value.

[Unreleased]: https://github.com/uchaloop/confmaker/compare/v0.6.0...HEAD
[0.6.0]: https://github.com/uchaloop/confmaker/compare/v0.5.0...v0.6.0
[0.5.0]: https://github.com/uchaloop/confmaker/compare/v0.4.2...v0.5.0
[0.4.2]: https://github.com/uchaloop/confmaker/compare/v0.4.1...v0.4.2
[0.4.1]: https://github.com/uchaloop/confmaker/compare/v0.4.0...v0.4.1
[0.4.0]: https://github.com/uchaloop/confmaker/compare/v0.3.1...v0.4.0
[0.3.1]: https://github.com/uchaloop/confmaker/compare/v0.3.0...v0.3.1
[0.3.0]: https://github.com/uchaloop/confmaker/compare/v0.2.0...v0.3.0
[0.2.0]: https://github.com/uchaloop/confmaker/releases/tag/v0.2.0
[0.1.0]: https://github.com/uchaloop/confmaker/releases/tag/v0.1.0
