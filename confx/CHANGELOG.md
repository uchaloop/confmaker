# Changelog

All notable changes to this module are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this module adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.1.0] - 2026-09-17

### Added

- `confx` is a Go module of its own, `github.com/uchaloop/confmaker/confx`, in the
  confmaker repository and under the import path the package had in confmaker
  v0.5.0 and earlier. It requires confmaker v0.6.1, which no longer contains it
  (v0.6.0 is retracted and still contains it).
- `Module`, `Provide` and `ProvideNamed` provide configs loaded by one
  `confmaker.Loader` per application. `Module` takes `confmaker.EnvOption`
  (`WithEnv`, `WithDump`, `AllowUnknown`); `Provide` and `ProvideNamed` take
  `confmaker.ConfigOption` (`WithName`, `WithPrefix`).
- `Module` is required: `Provide` or `ProvideNamed` without it fails the start
  with an error naming the config type.
- Every provided config is loaded and validated at start, whether or not anything
  consumes it, and every problem comes back in one error, whatever order the
  options are given in. Invalid registrations are part of that error.
- `ProvideNamed` uses its name as the instance name; a further `WithName` is
  reported as an error.

[Unreleased]: https://github.com/uchaloop/confmaker/compare/confx/v0.1.0...HEAD
[0.1.0]: https://github.com/uchaloop/confmaker/releases/tag/confx/v0.1.0
