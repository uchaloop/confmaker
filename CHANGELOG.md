# Changelog

All notable changes to this module are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this module adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

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

[Unreleased]: https://github.com/uchaloop/confmaker/compare/v0.3.1...HEAD
[0.3.1]: https://github.com/uchaloop/confmaker/compare/v0.3.0...v0.3.1
[0.3.0]: https://github.com/uchaloop/confmaker/compare/v0.2.0...v0.3.0
[0.2.0]: https://github.com/uchaloop/confmaker/releases/tag/v0.2.0
[0.1.0]: https://github.com/uchaloop/confmaker/releases/tag/v0.1.0
