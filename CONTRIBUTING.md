# Contributing

Bug fixes, documentation improvements and focused feature proposals are welcome.
Discuss new features and incompatible changes in an issue before implementing them.
For bug reports, include a minimal reproduction, the Go and confmaker versions,
and the expected and actual behavior. Use synthetic configuration values.
For suspected vulnerabilities, follow [SECURITY.md](SECURITY.md) instead of
opening a public issue with disclosure details.

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

## Commit messages

Start each commit subject with the source branch name, followed by a colon,
a space and a short English description of the change:

```text
release/1.0.0: library release with fixes
feat/manifest-option: add manifest export option
fix/json-text-defaults: reject defaults without a text marshaler
```

Use the full branch name as written. For example, on a branch named `feat/1.0.0`,
the prefix is `feat/1.0.0: `. Prefer a description of the actual change over
generic subjects such as "updates". For a squash merge, keep the source branch
prefix in the resulting commit subject. This convention is not enforced by CI.

## Release versions

Release branches use `release/x.y.z`, where `x.y.z` is the target library version,
not a branch counter. From v1 onward:

- `x` (major) changes for incompatible public contract changes.
- `y` (minor) changes for backward-compatible functionality additions.
- `z` (patch) changes for backward-compatible bug fixes.

Reset the lower components to zero when increasing a major or minor version.
Feature and fix branches normally describe the task; they do not need a version
number. The branch prefix alone does not determine the release version.

Maintainers publish versions with Git tags: `release/1.0.0` prepares `v1.0.0`.
A release candidate uses a tag such as `v1.0.0-rc.1`. A branch name does not
publish a module version. Never move or replace a published version tag.
See [Semantic Versioning](https://semver.org/) and
[Go module version numbering](https://go.dev/doc/modules/version-numbers)
for the full rules, including module path requirements for v2 and later.

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

## Coverage

CI uploads Go coverage reports to Codecov for PR review. Project and patch
coverage checks are informational: there is no required percentage, and upload
failures do not block CI or releases. Test failures still fail CI. Use uncovered
code to identify useful missing scenarios, not to add tests solely for a score.

To inspect coverage locally:

```sh
go test -race -covermode=atomic -coverprofile=coverage.out ./...
go tool cover -html=coverage.out
```

Maintainers must enable `uchaloop/confmaker` in Codecov and configure its repository
upload token as the GitHub Actions secret `CODECOV_TOKEN`. Do not put the token in
source files. Fork PRs do not receive this secret; their upload availability
follows Codecov's public-repository tokenless upload settings. The badge becomes
available after a successful upload for `main`.

## Code style

- Choose names that explain purpose. Use `Make…` / `make…` for constructors.
- Prefer early returns or loop continuation when they reduce nesting; do not
  replace straightforward control flow with `goto` for speculative optimization.
- Handle errors explicitly before returning successful results. Preserve partial
  results only where the function's contract requires them.
- Separate logical steps with blank lines, keeping a call and its error check
  together. Use `len(value)` for string emptiness checks.
- Use anonymous structs for one-use internal data shapes; retain named types when
  they are reused, carry methods, or form a public contract. Apply the same code
  style to README and GoDoc examples.
- Write GoDoc for callers: describe behavior, guarantees and relevant limits.
  Internal comments should explain non-obvious reasons rather than restate code.

## Compatibility and pull requests

Keep each pull request focused. Explain the problem, resulting behavior and
validation performed. Update examples and documentation when behavior changes,
and add a concise entry under the current unreleased version in [CHANGELOG.md](CHANGELOG.md).

From v1 onward, compatibility includes more than Go signatures: ENV names and
tags, parsing and default semantics, error categories, and the exported manifest
schema are also contracts. Discuss changes to these contracts explicitly; an
unchanged function signature does not make a behavior change compatible.

Never include real credentials in code, examples, fixtures or issue reports.
Preserve secret handling and the absence of values from diagnostic reports.
Manifest exports may contain non-secret defaults, so those must be safe to share.

## Changelog style

Use the same format in confmaker, confx and secret:

- Keep newest versions first. Use `## [x.y.z] - Unreleased` once the target version
  is known, or `## [Unreleased]` before choosing it. Do not add a separate target
  release paragraph. Version headings omit `v`; Git tags include it.
- At publication, replace `Unreleased` with the actual release date in
  `YYYY-MM-DD` format. Do not infer publication dates from commit dates. Preserve
  undated historical entries when the release date cannot be verified.
- Group entries under `Added`, `Changed`, `Deprecated`, `Removed`, `Fixed`,
  `Security`, in that order. Omit empty categories, even for small releases.
- Write in English, using past-tense opening verbs such as `Added`, `Changed`,
  `Removed` and `Fixed`. Each bullet describes one user-visible change, normally
  in one or two sentences. Include internal work only when it explains a useful
  result. Preserve necessary historical compatibility and retraction notes.
- Prefix incompatible changes with `**Breaking:**` in their normal category and
  explain the replacement or required action when applicable. Do not duplicate
  them in a separate breaking-changes section.
- Define heading links at the bottom of the file. An unreleased target compares
  the last released tag with `HEAD`; a released version compares its predecessor
  with its tag. Link the first release to its release page. Preserve explicit
  historical exceptions for retracted or incorrectly tagged versions.
- Separate headings, paragraphs and lists with one blank line. Wrap continuation
  lines consistently and format API names as inline code.

Example before publication:

```markdown
## [2.0.0] - Unreleased

### Added

- Added support for application-provided configuration engines.

### Changed

- **Breaking:** Renamed `EnvOption` to `LoaderOption`; update option declarations.

[2.0.0]: https://github.com/uchaloop/confmaker/compare/v1.0.1...HEAD
```

## Integration module

Core tests must not import external secret packages. Use the internal test fixture
for the structural sensitivity contract. Real secret compatibility tests live in
`integration/secret`; see its README for commands. Its relative core replacement
is intentional and must not be added to the root module.
