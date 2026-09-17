# confx

The [Uber Fx](https://github.com/uber-go/fx) adapter for
[confmaker](https://github.com/uchaloop/confmaker): it loads configs when the
application starts and provides them to the Fx container.

```bash
go get github.com/uchaloop/confmaker/confx
```

Declaring configs, tags, JSON values, the manifest and the dump are described in
the [confmaker README](../README.md) and its
[package documentation](https://pkg.go.dev/github.com/uchaloop/confmaker).

## Usage

```go
fx.New(
	confx.Module(), // required

	confx.Provide[postgres.Config](), // POSTGRES_*
	confx.ProvideNamed[postgres.Config](
		"replica",
		confmaker.WithPrefix("REPLICA_POSTGRES_"),
	), // REPLICA_POSTGRES_*, tagged name:"replica"

	fx.Invoke(func(p Params) { /* ... */ }),
)
```

- `Module` is required once per application. It loads every provided config at
  start, whether or not anything consumes it, and fails the start with one error
  that lists every problem.
- `Provide` provides an untagged value: the default instance of the type.
- `ProvideNamed` provides a value tagged `name:"<name>"`, for a second instance
  of the same type.

A consumer receives the named instance through its tag:

```go
type Params struct {
	fx.In

	Primary postgres.Config
	Replica postgres.Config `name:"replica"`
}
```

## Name, prefix and tag

These are three different things:

| | Comes from | Used for |
|---|---|---|
| instance name | `ConfigName`, `confmaker.WithName`, or the `ProvideNamed` name | the default prefix and errors |
| ENV prefix | the instance name (`replica` → `REPLICA_`), or `confmaker.WithPrefix` | variable names |
| Fx tag | `ProvideNamed` only | how consumers ask for the value |

`Provide` with `confmaker.WithName` changes the name and prefix but adds no Fx
tag. `ProvideNamed` refuses a further `WithName`.

`Module` takes the options of `confmaker.MakeLoader`, such as
`confmaker.WithDump(os.Stdout)` or `confmaker.WithEnv(vars)` in tests.

## Documentation

[pkg.go.dev/github.com/uchaloop/confmaker/confx](https://pkg.go.dev/github.com/uchaloop/confmaker/confx)
