package confmaker_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/uchaloop/confmaker"
	"github.com/uchaloop/secret/v2"
)

type secretConfig struct {
	Password secret.Secret   `env:"PASSWORD"`
	Empty    secret.Secret   `env:"EMPTY"`
	Pointer  **secret.Secret `env:"POINTER"`
}

func (c *secretConfig) SetDefaults() {
	c.Password = secret.New("private-default")
	c.Empty = secret.New("")
}

func TestSecretLoadingAndDescription(t *testing.T) {
	for _, override := range []bool{false, true} {
		env := map[string]string{"APP_POINTER": "private-pointer"}
		if override {
			env["APP_PASSWORD"] = "private-env"
		}

		diagnostics := confmaker.MakeDiagnostics()
		loader := confmaker.MakeLoader(confmaker.WithEnv(env), confmaker.WithDiagnostics(diagnostics))
		handle := loader.Register[secretConfig]("app")
		for _, write := range []func(io.Writer) error{loader.WriteEnvExample, loader.WriteManifestJSON, loader.WriteManifestMarkdown} {
			var output bytes.Buffer
			if err := write(&output); err != nil {
				t.Fatal(err)
			}

			if strings.Contains(output.String(), "private-") {
				t.Fatal("manifest disclosed a secret")
			}
		}

		if err := loader.Load(); err != nil {
			t.Fatal(err)
		}

		cfg, err := handle.Value()
		if err != nil {
			t.Fatal(err)
		}

		expected, source := "private-default", confmaker.SourceDefault
		if override {
			expected, source = "private-env", confmaker.SourceEnv
		}

		if cfg.Password.Reveal() != expected || !cfg.Empty.IsZero() || (**cfg.Pointer).Reveal() != "private-pointer" {
			t.Fatal("incorrect secret loading")
		}

		report := diagnostics.Report()
		variables := report.Configs[0].Variables
		if variables[0].Source != source || variables[1].Source != confmaker.SourceZero || variables[2].Source != confmaker.SourceEnv {
			t.Fatal("incorrect diagnostic sources")
		}

		encoded, err := json.Marshal(report)
		if err != nil {
			t.Fatal(err)
		}

		if strings.Contains(string(encoded), "private-") {
			t.Fatal("diagnostics disclosed a secret")
		}

		cfg.Password.Clear()
		again, err := handle.Value()
		if err != nil {
			t.Fatal(err)
		}

		if again.Password.Reveal() != expected {
			t.Fatal("clearing a returned value changed the loader's secret")
		}
	}
}

type rejectedSecret struct{ secret.Secret }

type secretDecodeError struct{ input string }

func (e *secretDecodeError) Error() string { return "decoder rejected " + e.input }

func (*rejectedSecret) UnmarshalText(raw []byte) error {
	return &secretDecodeError{input: string(raw)}
}

func TestEmbeddedSecretParseErrorIsSanitized(t *testing.T) {
	_, err := confmaker.Load[struct {
		Password rejectedSecret `env:"PASSWORD"`
	}]("app", confmaker.WithEnv(map[string]string{"APP_PASSWORD": "private-input"}))
	problems := confmaker.ConfigErrors(err)
	if err == nil || len(problems) != 1 || problems[0].Kind != confmaker.ErrorParse {
		t.Fatal("expected a secret parse error")
	}

	var cause *secretDecodeError
	if strings.Contains(err.Error(), "private-input") || errors.As(err, &cause) {
		t.Fatal("secret parser input or cause escaped")
	}
}
