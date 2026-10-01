package confmaker

import (
	"bytes"
	"errors"
	"flag"
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
			handle := loader.Register[envExampleConfig]("app")
			var got, want bytes.Buffer
			if err := describe.Write(loader, &got); err != nil {
				t.Fatal(err)
			}
			var err error
			switch format {
			case "env":
				err = loader.WriteEnvExample(&want)
			case "markdown":
				err = loader.WriteManifestMarkdown(&want)
			case "json":
				err = loader.WriteManifestJSON(&want)
			}
			if err != nil || got.String() != want.String() {
				t.Fatalf("export differs: %v", err)
			}
			if _, err := handle.Value(); !errors.Is(err, ErrNotLoaded) {
				t.Fatalf("loaded config: %v", err)
			}
			cause := errors.New("writer failure")
			if err := describe.Write(loader, exampleErrorWriter{cause}); !errors.Is(err, cause) {
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
		if err := describe.Write(MakeLoader(), &output); err == nil || output.Len() != 0 {
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
