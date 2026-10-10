// Package confmaker centralizes typed application configuration with a built-in
// ENV engine and an optional application-provided backend.
//
// # Getting started
//
// Use [Load] for one configuration. For several, create a loader with [MakeLoader],
// register configurations with [Loader.Register], call [Loader.Load], then read
// [Handle.Value]. Configuration types are ordinary structs; they need no dependency
// on this package. See the runnable Load and Loader examples below.
//
// # Loading lifecycle
//
// The loader checks declarations and conflicts, creates fresh defaults, fills
// configurations and validates them. Only root SetDefaults and Validate methods run
// automatically. All handles withhold their values unless the entire load succeeds.
// Loading happens once; later calls return the published result without reloading.
// A registration made after loading starts is rejected.
//
// [Loader.LoadContext] supports cooperative cancellation. The initiating context
// controls loading; other callers can cancel only their waiting. Cancellation after
// starting is final. User methods run synchronously and cannot be forcibly stopped.
// See the method documentation for result precedence and cancellation before startup.
//
// # Built-in ENV engine
//
// Without [WithEngine], names derive ENV prefixes and tags select fields. For example,
// registration "store" and env:"HOST" select STORE_HOST. [WithPrefix] overrides the
// prefix. [WithEnv] copies a complete replacement environment; nil means empty ENV.
// Otherwise the loader reads one process ENV snapshot.
//
// Present variables replace whole fields; missing variables retain defaults.
// The required and notEmpty tag options constrain ENV presence and text, independently
// of defaults. Use Validate to constrain final Go values. Unknown variables under
// registered prefixes are errors unless [AllowUnknown] exempts them.
//
// Supported fields include scalars, durations, custom encoding.TextUnmarshaler types,
// plain scalar/text collections, and JSON collections. Nested configuration structs
// use envPrefix and are held by value. Pointers allow one level over scalar/text
// values, not collections or collection elements. JSON rejects ordinary structs and
// uses text methods for custom text types. The envDefault tag is not supported.
// No files, external secret stores or application lifecycles are managed by this engine.
//
// # Custom engines
//
// [WithEngine] selects an [Engine] or [EngineFunc]. Each [LoadRequest] supplies a
// registration name and a non-nil pointer to a fresh configuration struct. Names must
// be nonempty and unique. The backend owns its sources, tags, source consistency and
// error sanitization. Calls are sequential within one loader; resources stay caller-owned.
// The engine must not retain the target or modify it after returning.
//
// [WithEnv], [AllowUnknown] and [WithPrefix] cannot be combined with a custom engine.
// Custom engines have configuration-level diagnostics and do not support
// [Loader.Manifest], which returns [ErrManifestUnsupported].
//
// # Errors and diagnostics
//
// Load aggregates problems. [ConfigErrors] extracts library error categories and
// structural context. Handle the original error as well: lifecycle and engine errors
// need not be [ConfigError]. Ordinary causes remain available to errors.Is/errors.As;
// sensitive parse causes are discarded.
//
// [WithDiagnostics] enables independent [Loader.Report] snapshots. They contain no
// values, defaults, error messages or panic payloads. [WithDiagnosticHandler] also
// enables collection and invokes its callback after publication on success or error.
// Loading panic skips that callback, propagates and leaves subsequent load calls
// returning [ErrLoadPanicked]. See [Loader.Report] for states and partial results.
//
// # Manifest and exports
//
// [Manifest] and [Loader.Manifest] describe built-in schemas without reading ENV or
// running user methods. [IncludeDefaults] explicitly evaluates fresh defaults;
// render failures are recorded in [ManifestResult.Problems] without losing the schema.
// Loaded values are never used. Sensitive defaults are never rendered.
//
// Package github.com/uchaloop/confmaker/v2/confexport writes a prepared manifest as
// JSON, Markdown or ENV examples. Package github.com/uchaloop/confmaker/v2/confcli
// adds an optional describe flag to an application; it is not a standalone command.
// Callers own flag parsing, writers, files and application termination.
//
// # Sensitive values and concurrency
//
// [SensitiveValue] or env:",secret" marks a whole field as sensitive. The marker is
// inspected by type, never called. User Validate methods and custom engines must
// avoid disclosing secrets in their errors. The core has no external module dependencies.
//
// Loader supports concurrent calls. Returned maps, slices and pointers are shared:
// treat loaded configurations as read-only. User methods must not call Load recursively
// on their own loader. Sharing an engine across loaders, or concurrently evaluating
// manifest defaults, requires the corresponding user code to be concurrency-safe.
package confmaker
