// Package confcli adds an optional configuration description flag to an application.
// It is a library helper, not a standalone executable installed with go install.
//
// # Using the describe flag
//
// Register -describe with [MakeDescribeFlag] on your flag.FlagSet. Parse arguments
// and handle parsing errors, then inspect [DescribeFlag.Requested]. On a describe
// request, prepare a confmaker manifest and call [DescribeFlag.Write] with a writer.
// Supported formats are env, markdown and json.
//
// # Ownership
//
// The caller decides when to evaluate defaults, write output or continue startup.
// This package does not load configuration, evaluate defaults, parse arguments
// automatically, manage files or exit the application. Like flag.FlagSet, its
// flag state is not intended for concurrent mutation.
package confcli
