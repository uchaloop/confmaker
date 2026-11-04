package confexport_test

import (
	"bytes"
	"io"
	"math"
	"strconv"
	"strings"
	"testing"

	c "github.com/uchaloop/confmaker/v2"
	"github.com/uchaloop/confmaker/v2/confexport"
)

type nanDefaultConfig struct {
	Values map[float64]string `env:"VALUES"`
}

func (v *nanDefaultConfig) SetDefaults() {
	v.Values = map[float64]string{math.NaN(): "unexportable-default"}
}

type nanKey struct{ Number float64 }

func (k *nanKey) UnmarshalText(text []byte) error {
	n, err := strconv.ParseFloat(string(text), 64)
	if err != nil {
		return err
	}

	k.Number = n

	return nil
}

func (k *nanKey) MarshalText() ([]byte, error) {
	return []byte(strconv.FormatFloat(
		k.Number,
		'g',
		-1,
		64,
	)), nil
}

type customNaNDefaultConfig struct {
	Values map[nanKey]string `env:"VALUES"`
}

func (v *customNaNDefaultConfig) SetDefaults() {
	v.Values = map[nanKey]string{{Number: math.NaN()}: "unexportable-default"}
}

func TestNonReflexiveMapDefaultExports(t *testing.T) {
	for _, test := range []struct {
		name     string
		manifest func(string, ...c.DescribeOption) (c.ManifestResult, error)
	}{
		{name: "float", manifest: c.Manifest[nanDefaultConfig]},
		{name: "custom", manifest: c.Manifest[customNaNDefaultConfig]},
	} {
		t.Run(test.name, func(t *testing.T) {
			manifest, err := test.manifest("app", c.IncludeDefaults())
			if err != nil {
				t.Fatal(err)
			}

			variable := manifest.Configs[0].Variables[0]
			if variable.Default.State != c.DefaultUnrenderable || len(variable.Default.Text) != 0 {
				t.Fatalf("unexpected default: %+v", variable.Default)
			}

			if len(manifest.Problems) != 1 {
				t.Fatalf("problems: %+v", manifest.Problems)
			}

			problem := manifest.Problems[0]
			if problem.Kind != c.ErrorDefaultRender || problem.InstanceName != "app" || problem.VariableName != "APP_VALUES" || problem.FieldPath != "Values" {
				t.Fatalf("unexpected problem: %+v", problem)
			}

			for _, export := range []struct {
				name  string
				write func(io.Writer, c.ManifestResult) error
			}{
				{name: "env", write: confexport.WriteEnvExample},
				{name: "markdown", write: confexport.WriteMarkdown},
				{name: "json", write: confexport.WriteJSON},
			} {
				t.Run(export.name, func(t *testing.T) {
					var output bytes.Buffer
					if err := export.write(&output, manifest); err != nil {
						t.Fatal(err)
					}

					if strings.Contains(output.String(), "NaN") || strings.Contains(output.String(), "unexportable-default") {
						t.Fatalf("invalid default exported: %s", output.String())
					}
				})
			}
		})
	}
}
