// Package confexport renders prepared confmaker manifests as JSON, Markdown or
// ENV examples. It does not load configuration or evaluate defaults.
//
// # Preparing a manifest
//
// Use confmaker.Manifest or confmaker.Loader.Manifest. IncludeDefaults explicitly
// evaluates fresh defaults; otherwise only declarations are described. Inspect
// ManifestResult.Problems when all default values must be rendered.
//
// # Writing output
//
// Choose [WriteJSON], [WriteMarkdown] or [WriteEnvExample]. Reuse the same prepared
// manifest across formats. Sensitive defaults are suppressed. Writers invoke no
// user methods and own no files; the caller chooses and closes the destination.
// Writer errors may leave partial output. See each writer for format guarantees.
package confexport
