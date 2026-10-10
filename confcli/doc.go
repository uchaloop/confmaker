// Package confcli adds optional configuration description to an application.
// It is a library helper, not a standalone executable installed with go install.
//
// [Describe] parses arguments with a private flag set and writes env, markdown
// or json descriptions from a [ManifestSource]. It returns whether it handled
// a description or help request. The caller handles errors and process exit.
// Pass confmaker.IncludeDefaults explicitly to evaluate and render defaults.
// Without a request, Describe does not call Manifest.
//
// Applications with their own flags can register [MakeDescribeFlag] on a
// flag.FlagSet, parse arguments, then use [DescribeFlag.Requested] and
// [DescribeFlag.Write]. This lower-level API does not parse arguments or
// evaluate defaults. Its flag state is not intended for concurrent mutation.
// Neither API loads configuration or starts application services.
package confcli
