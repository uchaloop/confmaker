# Migration to confmaker v2

This branch is unreleased. Changes are incompatible with v1.

1. Publish the compatible secret update with `(Secret) IsSensitive()` first.
   Keep its previous marker. Until that version is available, use explicit
   `env:"NAME,secret"` tags or test with the local updated module.
2. Update confmaker imports to `github.com/uchaloop/confmaker/v2`. Remove pointer
   chains, collection pointers and pointer elements; use scalar pointers, value
   collections or custom text types. Keep all registrations before Load.
3. Replace MakeDiagnostics and receiver options with WithDiagnostics() and
   loader.Report(). For single Load[T] or internal confx loaders use
   WithDiagnosticHandler. Disabled collection is now explicit.
4. Manifest() no longer evaluates defaults. Request IncludeDefaults explicitly.
   Both Manifest forms return ManifestResult; read Configs, Variables and Problems.
   Replace HasDefault with Default.State and read Text only for DefaultRendered.
   Render failures no longer return an error: inspect Problems for strict output.
5. Replace loader.WriteManifestJSON/Markdown/WriteEnvExample with confexport
   WriteJSON/WriteMarkdown/WriteEnvExample(writer, result). JSON wire version is 2;
   consumers must understand the default object and structured problems array.
6. Move MakeDescribeFlag to confcli and pass a prepared result to Write(writer,
   result). Exporting multiple formats must reuse a result rather than call
   IncludeDefaults repeatedly.
7. Update confx to the matching version using v2 types. Its registration and
   provisioning API remains the same, but v1 handles/options are incompatible.

For local integration use a go.work outside the repositories containing secret,
confmaker and confx. If dependency graph loading requires it, add a version-specific
workspace replace for confmaker/v2 v2.0.0 pointing to the local confmaker. Do not
commit local replacements into module files. The v2.0.0 requirement in the adapter
is a planned release target, not a claim that the tag exists.

Before publication, update test dependencies to the actual published secret marker
version, tidy each module with GOWORK=off, run race tests and vet in each module,
publish confmaker v2, then resolve/tidy and release the adapter against that tag.
No release was performed by this migration.

The core's production imports do not include secret. A test-only dependency remains
for integration checks. Older secret.Secret values without an explicit secret tag
or the new marker are not automatically recognized: audit these fields during
migration. Tags do not make ordinary strings safe to log. Validate errors remain
application-authored and are not sanitized.
