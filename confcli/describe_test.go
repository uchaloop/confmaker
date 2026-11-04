package confcli

import (
	"bytes"
	"errors"
	"flag"
	"io"
	"testing"

	. "github.com/uchaloop/confmaker/v2"
	"github.com/uchaloop/confmaker/v2/confexport"
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

func TestDescribe(t *testing.T) {
	for _, format := range []string{"env", "markdown", "json"} {
		t.Run(format, func(t *testing.T) {
			loader := MakeLoader()
			handle := loader.Register[describeConfig]("app")

			var output bytes.Buffer
			isHandled, err := Describe(
				loader,
				[]string{"-describe=" + format},
				&output,
				IncludeDefaults(),
			)
			if err != nil || !isHandled || !bytes.Contains(output.Bytes(), []byte("localhost")) {
				t.Fatalf("description: handled=%v, err=%v, output=%s", isHandled, err, &output)
			}

			if _, err := handle.Value(); !errors.Is(err, ErrNotLoaded) {
				t.Fatalf("loaded configuration: %v", err)
			}

			output.Reset()
			if _, err := Describe(loader, []string{"-describe=" + format}, &output); err != nil {
				t.Fatal(err)
			}

			if bytes.Contains(output.Bytes(), []byte("localhost")) {
				t.Fatal("defaults included without explicit option")
			}
		})
	}
}

func TestDescribeArguments(t *testing.T) {
	for _, args := range [][]string{nil, {"-help"}, {"-describe=xml"}, {"-unknown"}, {"extra"}, {"-describe=json", "extra"}} {
		var output bytes.Buffer
		isHandled, err := Describe(nil, args, &output)
		isErrorExpected := len(args) > 0 && args[0] != "-help"
		if (err != nil) != isErrorExpected {
			t.Fatalf("args %v: %v", args, err)
		}

		if !isErrorExpected && isHandled != (len(args) > 0) {
			t.Fatalf("args %v: handled=%v", args, isHandled)
		}
	}
}

func TestDescribeErrors(t *testing.T) {
	cause := errors.New("output unavailable")
	if _, err := Describe(MakeLoader(), []string{"-describe=json"}, exampleErrorWriter{cause}); !errors.Is(err, cause) {
		t.Fatalf("writer error: %v", err)
	}

	if _, err := Describe(nil, []string{"-help"}, exampleErrorWriter{cause}); !errors.Is(err, cause) {
		t.Fatalf("help writer error: %v", err)
	}

	loader := MakeLoader()
	loader.Register[int]("invalid")
	if _, err := Describe(loader, []string{"-describe=json"}, io.Discard); err == nil {
		t.Fatal("manifest error lost")
	}
}

type describeConfig struct {
	Host string `env:"HOST"`
}

func (c *describeConfig) SetDefaults() {
	c.Host = "localhost"
}

type shortHelpWriter struct{}

func (shortHelpWriter) Write(data []byte) (int, error) {
	return len(data) / 2, nil
}

func TestDescribeHelpShortWrite(t *testing.T) {
	isHandled, err := Describe(nil, []string{"-help"}, shortHelpWriter{})
	if isHandled || !errors.Is(err, io.ErrShortWrite) {
		t.Fatalf("handled=%v, error=%v; want false and short write", isHandled, err)
	}
}
