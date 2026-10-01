package confmaker

import (
	"flag"
	"fmt"
	"io"
)

// DescribeFlag selects an optional manifest export. Create it with
// MakeDescribeFlag, parse its FlagSet, then check Requested before Write.
// It does not parse arguments, load ENV or terminate the process.
// Like flag.FlagSet, it is not intended for concurrent mutation.
type DescribeFlag struct {
	format string
}

// MakeDescribeFlag registers -describe in flags. Accepted formats are env,
// markdown and json; omission means normal application startup. Invalid formats
// are rejected by FlagSet.Parse. As with other flag registration functions,
// duplicate flag names panic. flags must be non-nil.
func MakeDescribeFlag(flags *flag.FlagSet) *DescribeFlag {
	describe := &DescribeFlag{}
	flags.Func("describe", "Describe configuration: env, markdown or json", func(format string) error {
		switch format {
		case "env", "markdown", "json":
			describe.format = format
			return nil
		default:
			return fmt.Errorf("unknown description format %q; use env, markdown or json", format)
		}
	})
	return describe
}

// Requested reports whether a valid description format was selected.
// Check FlagSet.Parse's error before using the result.
func (d *DescribeFlag) Requested() bool {
	return len(d.format) != 0
}

// Write exports the selected format through loader to writer. It does not load
// configurations. Calling Write without selecting a format returns an error.
// Export and writer errors are passed through unchanged. loader must be non-nil.
func (d *DescribeFlag) Write(loader *Loader, writer io.Writer) error {
	switch d.format {
	case "env":
		return loader.WriteEnvExample(writer)
	case "markdown":
		return loader.WriteManifestMarkdown(writer)
	case "json":
		return loader.WriteManifestJSON(writer)
	default:
		return fmt.Errorf("no description format selected; parse -describe before Write")
	}
}
