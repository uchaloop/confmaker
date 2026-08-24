# Changelog

All notable changes to this module are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this module adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Fixed

- The suggestion in an unknown-variable error is now stable: candidates at the
  same edit distance are broken by name instead of by map iteration order, which
  reported a different variable on every run.
- `confx.AllowUnknown("")` no longer disables the strict check. An empty prefix
  matches every variable, so it is ignored rather than honoured.
- A `T` the env parser cannot read - a pointer, a map, anything but a struct -
  no longer makes the strict check report every variable under its prefix as
  unknown. The prefix is skipped so the constructor reports the actual problem.

## [0.4.0] - 2026-08-24

Configuration is now read from the environment only. Files are gone, and with
them the environment-named configuration groups that
[12factor III](https://12factor.net/config) argues against.

### Removed

- The `confmaker` root package: `Load`, `LoadDir`, `Required`, `Registry`,
  `MakeRegistry`, and `ResolveSecret`. The module is now `confmaker/confx` and
  `confmaker/validate`.
- `confx.LoadModule`, `confx.LoadDir`, and `confx.Source`. Configuration files,
  the `ENVIRONMENT` variable, and the `common.toml` / `dev.toml` / `stage.toml` /
  `prod.toml` convention are no longer read.
- The `koanf` struct tag is no longer used. It stays inert where libraries still
  declare it.
- Nine dependencies: `koanf/v2`, its TOML parser and file provider, `koanf/maps`,
  `go-viper/mapstructure/v2`, `pelletier/go-toml/v2`, `fsnotify`,
  `mitchellh/copystructure`, and `mitchellh/reflectwalk`.

### Changed

- `confx.ProvideNoFileDefault[T](name)` is now `confx.Provide[T](name)`, and
  `confx.ProvideNoFile[T](name)` is now `confx.ProvideNamed[T](name)`. Both take
  an instance name rather than a file section; the environment prefix is derived
  from it as before.
- `confx.WithEnvPrefix` is now `confx.WithPrefix`.

### Added

- `confx.Module` checks the environment against the manifest the `Provide` calls
  register: a variable that starts with a prefix the application owns but matches
  no field fails the start, with a suggestion of the name it likely misspells.
  This replaces the strict file decoding that caught typos before.
- `confx.AllowUnknown` exempts prefixes from that check, for a deployment that
  shares one environment between several binaries.
- `confx.WithDump` writes every variable the application reads, its type, its
  current value, and where that value comes from. Secrets are reported as set or
  unset and never printed.
- `confx.Manifest[T](name)` returns that same list without building an
  application - name, type, required, secret, and declared default, in
  declaration order - so a `.env.example`, a ConfigMap, or a documentation table
  can be generated from the config type. It takes the options `Provide` takes.

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

[Unreleased]: https://github.com/uchaloop/confmaker/compare/v0.4.0...HEAD
[0.4.0]: https://github.com/uchaloop/confmaker/compare/v0.3.1...v0.4.0
[0.3.1]: https://github.com/uchaloop/confmaker/compare/v0.3.0...v0.3.1
[0.3.0]: https://github.com/uchaloop/confmaker/compare/v0.2.0...v0.3.0
[0.2.0]: https://github.com/uchaloop/confmaker/releases/tag/v0.2.0
[0.1.0]: https://github.com/uchaloop/confmaker/releases/tag/v0.1.0
