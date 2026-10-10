package confcli

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"

	c "github.com/uchaloop/confmaker/v2"
	"github.com/uchaloop/confmaker/v2/confexport"
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

// Write exports the selected format from a prepared manifest. It never loads
// configurations or evaluates defaults. Calling it without selecting a format
// returns an error. Writer errors are passed through unchanged.
func (d *DescribeFlag) Write(writer io.Writer, manifest c.ManifestResult) error {
	switch d.format {
	case "env":
		return confexport.WriteEnvExample(writer, manifest)
	case "markdown":
		return confexport.WriteMarkdown(writer, manifest)
	case "json":
		return confexport.WriteJSON(writer, manifest)
	default:
		return fmt.Errorf("no description format selected; parse -describe before Write")
	}
}

// ManifestSource describes configuration without loading it. Both a confmaker
// Loader and application compositions can implement this interface.
type ManifestSource interface {
	Manifest(...c.ManifestOption) (c.ManifestResult, error)
}

// Describe handles -describe=env|markdown|json or -help with a private FlagSet.
// It returns false, nil when no action was requested and startup may continue.
// A successful description or help request returns true, nil. On any error,
// the caller must stop startup and choose how to report it.
//
// Defaults are evaluated only when explicitly requested with c.IncludeDefaults.
// Unknown flags and positional arguments are rejected. Applications with their
// own flags should use MakeDescribeFlag instead. Help and descriptions go to
// writer; parse errors are returned without printing. Describe never exits the
// process or changes flag.CommandLine. source is only used for descriptions;
// it and writer must be non-nil when used.
func Describe(source ManifestSource, args []string, writer io.Writer, opts ...c.ManifestOption) (bool, error) {
	flags := flag.NewFlagSet("configuration", flag.ContinueOnError)
	flags.SetOutput(io.Discard)

	describe := MakeDescribeFlag(flags)
	if err := flags.Parse(args); err != nil {
		if !errors.Is(err, flag.ErrHelp) {
			return false, err
		}

		var help bytes.Buffer
		flags.SetOutput(&help)
		flags.PrintDefaults()

		if _, err := writer.Write(help.Bytes()); err != nil {
			return false, err
		}

		return true, nil
	}

	if flags.NArg() > 0 {
		return false, fmt.Errorf("unexpected argument %q", flags.Arg(0))
	}

	if !describe.Requested() {
		return false, nil
	}

	manifest, err := source.Manifest(opts...)
	if err != nil {
		return false, err
	}

	if err := describe.Write(writer, manifest); err != nil {
		return false, err
	}

	return true, nil
}
