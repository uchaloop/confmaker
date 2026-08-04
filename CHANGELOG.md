# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

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

[0.1.0]: https://github.com/uchaloop/confmaker/releases/tag/v0.1.0
