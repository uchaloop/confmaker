package confcli

import (
	"bytes"
	"errors"
	"flag"
	. "github.com/uchaloop/confmaker/v2"
	"github.com/uchaloop/confmaker/v2/confexport"
	"io"
	"testing"
)

func TestDescribeFlag(t *testing.T) {
	for _, format := range []string{"env", "markdown", "json"} {
		t.Run(format, func(t *testing.T) {
			flags := flag.NewFlagSet("test", flag.ContinueOnError)
			describe := MakeDescribeFlag(flags)
			if describe.Requested() {
				t.Fatal("requested before parsing")
			}

			if err := flags.Parse([]string{"-describe=" + format}); err != nil {
				t.Fatal(err)
			}

			if !describe.Requested() {
				t.Fatal("request lost")
			}

			loader := MakeLoader()
			handle := loader.Register[struct {
				Host string `env:"HOST"`
			}]("app")
			manifest, err := loader.Manifest()
			if err != nil {
				t.Fatal(err)
			}
			var got, want bytes.Buffer
			if err := describe.Write(&got, manifest); err != nil {
				t.Fatal(err)
			}

			err = nil
			switch format {
			case "env":
				err = confexport.WriteEnvExample(&want, manifest)
			case "markdown":
				err = confexport.WriteMarkdown(&want, manifest)
			case "json":
				err = confexport.WriteJSON(&want, manifest)
			}

			if err != nil || got.String() != want.String() {
				t.Fatalf("export differs: %v", err)
			}

			if _, err := handle.Value(); !errors.Is(err, ErrNotLoaded) {
				t.Fatalf("loaded config: %v", err)
			}

			cause := errors.New("writer failure")
			if err := describe.Write(exampleErrorWriter{cause}, manifest); !errors.Is(err, cause) {
				t.Fatalf("writer error: %v", err)
			}
		})
	}
}

func TestDescribeFlagInvalidOrAbsent(t *testing.T) {
	for _, args := range [][]string{nil, {"-describe=xml"}, {"-describe="}, {"-describe"}} {
		flags := flag.NewFlagSet("test", flag.ContinueOnError)
		flags.SetOutput(io.Discard)
		describe := MakeDescribeFlag(flags)
		err := flags.Parse(args)
		if (err != nil) != (len(args) != 0) {
			t.Fatalf("parse %v: %v", args, err)
		}

		if describe.Requested() {
			t.Fatal("unexpected request")
		}

		var output bytes.Buffer
		if err := describe.Write(&output, ManifestResult{}); err == nil || output.Len() != 0 {
			t.Fatalf("missing format: %v", err)
		}
	}
	// Each instance registers on its own FlagSet, not flag.CommandLine.
	first, second := flag.NewFlagSet("first", flag.ContinueOnError), flag.NewFlagSet("second", flag.ContinueOnError)
	a, b := MakeDescribeFlag(first), MakeDescribeFlag(second)
	if err := first.Parse([]string{"-describe=json"}); err != nil {
		t.Fatal(err)
	}

	if !a.Requested() || b.Requested() {
		t.Fatal("flag sets share state")
	}
}

type exampleErrorWriter struct{ err error }

func (w exampleErrorWriter) Write([]byte) (int, error) { return 0, w.err }
