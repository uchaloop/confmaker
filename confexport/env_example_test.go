package confexport

import (
	"bytes"
	"errors"
	"fmt"
	. "github.com/uchaloop/confmaker/v2"
	"io"
	"strings"
	"testing"

	secret "github.com/uchaloop/confmaker/v2/internal/testsecret"
)

type envExampleConfig struct {
	Host      string        `env:"HOST,notEmpty" envDescription:"API address\nUse the staging endpoint"`
	Count     int           `env:"COUNT"`
	Enabled   bool          `env:"ENABLED"`
	Token     secret.Secret `env:"TOKEN,required"`
	Labels    []string      `env:"LABELS"`
	Command   string        `env:"COMMAND"`
	Multiline string        `env:"MULTILINE"`
	Pool      struct {
		Size int `env:"SIZE" envDescription:"Pool size"`
	} `envPrefix:"POOL_"`
}

func (c *envExampleConfig) SetDefaults() {
	c.Host = "https://api.example.test"
	c.Labels = []string{"single"}
	c.Command = "$(touch /tmp/not-executed)"
	c.Multiline = "first\nINJECTED=value"
	c.Pool.Size = 5
	_ = c.Token.UnmarshalText([]byte("hidden-default"))
}

func (c envExampleConfig) Validate() error {
	panic("example generation must not validate")
}

func TestWriteEnvExample(t *testing.T) {
	t.Setenv("APP_HOST", "environment-must-not-appear")
	loader := makeTestLoader()
	config := loader.Register[envExampleConfig]("app")
	var output bytes.Buffer
	if err := loader.WriteEnvExample(&output); err != nil {
		t.Fatal(err)
	}

	want := "# Configuration: \"app\"\n" +
		"# API address\n# Use the staging endpoint\n# Required; must not be empty.\nAPP_HOST=https://api.example.test\n\n" +
		"# APP_COUNT=\n\n# APP_ENABLED=\n\n" +
		"# Required.\n# Secret; supply your own value.\n# APP_TOKEN=\n\n" +
		"APP_LABELS=single\n\n# APP_COMMAND=\n\n# APP_MULTILINE=\n\n" +
		"# Pool size\nAPP_POOL_SIZE=5\n\n"
	if output.String() != want {
		t.Fatalf("got:\n%s\nwant:\n%s", &output, want)
	}

	if _, err := config.Value(); !errors.Is(err, ErrNotLoaded) {
		t.Fatalf("generation changed handle state: %v", err)
	}

	variables, err := singleManifestForTest[envExampleConfig]("app")
	if err != nil || variables[0].Description != "API address\nUse the staging endpoint" || variables[7].Description != "Pool size" {
		t.Fatalf("descriptions missing: %v, %v", variables, err)
	}
}

func TestWriteEnvExampleEscapesCommentContent(t *testing.T) {
	loader := makeTestLoader()
	loader.Register[struct {
		Value string `env:"BAD\nINJECTED" envDescription:"first\rINJECTED=value\x00"`
	}]("app")
	var output bytes.Buffer
	if err := loader.WriteEnvExample(&output); err != nil {
		t.Fatal(err)
	}

	for _, line := range strings.Split(output.String(), "\n") {
		if len(line) != 0 && !strings.HasPrefix(line, "# ") {
			t.Fatalf("uncommented content: %q", line)
		}
	}

	if strings.ContainsRune(output.String(), 0) {
		t.Fatal("raw control character in output")
	}
}

type exampleErrorWriter struct{ err error }

func (w exampleErrorWriter) Write(p []byte) (int, error) { return 0, w.err }

func TestWriteEnvExampleErrors(t *testing.T) {
	loader := makeTestLoader()
	loader.Register[envExampleConfig]("app")
	writeErr := errors.New("writer failed")
	if err := loader.WriteEnvExample(exampleErrorWriter{writeErr}); !errors.Is(err, writeErr) {
		t.Fatalf("writer error lost: %v", err)
	}

	if err := loader.WriteEnvExample(exampleErrorWriter{}); !errors.Is(err, io.ErrShortWrite) {
		t.Fatalf("short write lost: %v", err)
	}

	loader.Register[envExampleConfig]("app")
	var output bytes.Buffer
	if err := loader.WriteEnvExample(&output); err == nil || output.Len() != 0 {
		t.Fatalf("invalid manifest wrote output: %v, %q", err, &output)
	}
}

func TestEnvExampleAssignmentSyntax(t *testing.T) {
	for _, value := range []string{"localhost", "https://api.example.test:8080/path", "-3.5", "30s", "true"} {
		if !isEnvExampleDefaultSafe(value) {
			t.Errorf("safe default rejected: %q", value)
		}
	}

	for _, value := range []string{"", "two words", "$TOKEN", "${TOKEN}", "`command`", "#comment", "a\nb", "a\rb", "a\x00b", "'quoted'", "\"quoted\"", "a\\b", "a;b", "a&b"} {
		if isEnvExampleDefaultSafe(value) {
			t.Errorf("unsafe default accepted: %q", value)
		}
	}

	for _, name := range []string{"APP_URL", "_VALUE", "value2"} {
		if !isEnvExampleVariableName(name) {
			t.Errorf("safe name rejected: %q", name)
		}
	}

	for _, name := range []string{"", "2VALUE", "APP.HOST", "A B", "A\nB", "A=B"} {
		if isEnvExampleVariableName(name) {
			t.Errorf("unsafe name accepted: %q", name)
		}
	}
}

func ExampleWriteEnvExample() {
	loader := makeTestLoader()
	loader.Register[struct {
		URL string `env:"URL,notEmpty" envDescription:"Service endpoint"`
	}]("api")
	var output bytes.Buffer
	if err := loader.WriteEnvExample(&output); err != nil {
		panic(err)
	}

	fmt.Print(output.String())
	// Output:
	// # Configuration: "api"
	// # Service endpoint
	// # Required; must not be empty.
	// # API_URL=
}
