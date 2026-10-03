# Contributing

Bug fixes, documentation improvements and focused feature proposals are welcome.
Discuss new features and incompatible changes in an issue before implementing them.
For bug reports, include a minimal reproduction, the Go and confmaker versions,
and the expected and actual behavior. Use synthetic configuration values.

## Project scope

confmaker centralizes explicit application configuration through ENV. It owns
configuration registration, declaration checks, defaults, parsing, validation,
manifest generation and load diagnostics. Applications receive a prepared
environment at startup; deployment infrastructure can populate it from Vault
or other systems without confmaker knowing where the values originated.

Reading configuration files, fetching secrets from external systems and managing
application lifecycle are outside this library's responsibility. `WithEnv`
accepts an explicit set of variables, particularly for isolated tests; it does
not introduce source discovery or merging. Uber Fx integration belongs in
[confx](https://github.com/uchaloop/confx), while configuration rules remain here.

## Branches

Start each change from an up-to-date `main` and open its pull request against
`main`. Use a short English description with hyphens in the branch name:

- `fix/json-text-defaults` for bug fixes.
- `feat/manifest-option` for new features.
- `docs/default-config-example` for documentation.
- `refactor/field-parser` for internal restructuring.
- `ci/release-checks` for workflow changes.

For example, in a clone of this repository:

```sh
git switch main
git pull --ff-only
git switch -c fix/json-text-defaults
```

If contributing from a fork, update your branch from this repository's `main`
and target this repository's `main` in the pull request. Maintainers use
`release/<version>` branches for release preparation.

These names are recommendations, not CI requirements. Issue numbers are optional.

## Development

Use the Go version required by [go.mod](go.mod) or a compatible newer version.
Format changed Go files with `gofmt`, then run:

```sh
go test -race ./...
go vet ./...
```

Tests should verify behavior useful to callers. Add a regression test for a bug
fix and cover changed public behavior. Keep internal tests when they protect
meaningful properties such as concurrency, secret handling or codec correctness;
avoid tests that merely mirror implementation details. Support performance claims
with relevant benchmarks and before/after results.

## Code style

- Choose names that explain purpose. Use `Make…` / `make…` for constructors.
- Prefer early returns or loop continuation when they reduce nesting; do not
  replace straightforward control flow with `goto` for speculative optimization.
- Handle errors explicitly before returning successful results. Preserve partial
  results only where the function's contract requires them.
- Separate logical steps with blank lines, keeping a call and its error check
  together. Use `len(value)` for string emptiness checks.
- Write GoDoc for callers: describe behavior, guarantees and relevant limits.
  Internal comments should explain non-obvious reasons rather than restate code.

## Compatibility and pull requests

Keep each pull request focused. Explain the problem, resulting behavior and
validation performed. Update examples and documentation when behavior changes,
and add a concise entry under `Unreleased` in [CHANGELOG.md](CHANGELOG.md).

From v1 onward, compatibility includes more than Go signatures: ENV names and
tags, parsing and default semantics, error categories, and the exported manifest
schema are also contracts. Discuss changes to these contracts explicitly; an
unchanged function signature does not make a behavior change compatible.

Never include real credentials in code, examples, fixtures or issue reports.
Preserve secret handling and the absence of values from diagnostic reports.
Manifest exports may contain non-secret defaults, so those must be safe to share.
