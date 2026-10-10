# Secret integration tests

This separate module tests confmaker against the real `secret/v2` package.
Core unit tests use a minimal structural marker fixture and need no external modules.
The relative core replacement deliberately tests this checkout.

With published secret v2.2.0, run from this directory:

```sh
GOWORK=off go mod tidy
GOWORK=off go test -race ./...
GOWORK=off go vet ./...
```

Before publication, add this directory and the sibling secret checkout to a local
`go.work`. From GolandProjects, run:

```sh
go test -race ./confmaker/... ./confmaker/integration/secret/... ./confx/... ./secret/...
```

The root `go test ./...` does not enter this module. CI runs it separately.
The workspace is local only; published-version checksums must be generated after
secret v2.2.0 is published.
