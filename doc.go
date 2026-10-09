// Package confmaker centralizes explicit, typed configuration.
//
// The default engine loads ENV. WithEngine selects a custom Engine or EngineFunc,
// which receives a LoadRequest with a registration name and a non-nil struct pointer.
// Custom engines own their tags, source consistency and error sanitization. They
// must not retain or modify the target after returning. Sharing an engine between
// loaders requires safe concurrent use; its resources remain caller-owned.
// WithEnv, AllowUnknown and WithPrefix cannot be used with a custom engine.
// Custom registration names may be any nonempty unique strings.
//
// LoadContext supports cooperative cancellation. The initiating context controls
// the load; other callers cancel only their waiting. Cancellation after starting
// is final, while an already published result takes precedence. Load uses Background.
// LoaderOption is the common loader option type.
//
// Custom engines have config-level reports (DetailConfig) and do not support
// Manifest (ErrManifestUnsupported). The built-in engine has DetailField reports.
// The ENV-specific guarantees below apply to the built-in engine.
//
// Register configs on a Loader, call Load once, then read their Handle values.
// Load[T] handles a single config. Only the root SetDefaults and Validate methods
// run; ENV replaces defaults before validation. All values are withheld unless
// the entire set succeeds. WithEnv supplies a complete isolated environment.
//
// Required and notEmpty constrain ENV presence/text independently of defaults.
// Nested configs are held by value with envPrefix. Scalars, custom text types,
// simple collections and JSON collections are supported. Pointers are restricted
// to one level over scalars/text types; collection elements cannot be pointers.
// Unknown variables under registered prefixes are errors unless AllowUnknown
// exempts them. No files, external secret stores or service lifecycles are managed.
//
// WithDiagnostics enables Loader.Report snapshots. WithDiagnosticHandler also
// enables collection and runs after publication on success or error, never on a
// loading panic. A loading panic propagates and subsequent Load calls return
// ErrLoadPanicked. Reports never contain values, messages or error causes.
//
// Manifest describes only the schema by default and executes no user methods.
// IncludeDefaults evaluates fresh defaults and records per-field render problems
// in ManifestResult. Exporters in confexport accept that prepared result. The
// optional confcli package handles format selection for command-line programs.
//
// SensitiveValue or the secret ENV tag marks a whole field as sensitive. Sensitive
// defaults are never rendered and parse errors discard their original causes.
// User Validate errors must avoid disclosing secrets. Returned maps, slices and
// pointers are shared; configurations should be treated as read-only.
package confmaker
