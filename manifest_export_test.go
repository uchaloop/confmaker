package confmaker

import (
	"bytes"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
)

func TestWriteManifestJSON(t *testing.T) {
	t.Setenv("APP_HOST", "environment-must-not-appear")
	loader := MakeLoader()
	handle := loader.Register[envExampleConfig]("app")
	loader.Register[struct{}]("aaa")
	var output bytes.Buffer
	if err := loader.WriteManifestJSON(&output); err != nil {
		t.Fatal(err)
	}

	var document struct {
		Version int `json:"version"`
		Configs []struct {
			Name      string           `json:"instanceName"`
			Prefix    string           `json:"prefix"`
			Variables []map[string]any `json:"variables"`
		} `json:"configs"`
	}
	if err := json.Unmarshal(output.Bytes(), &document); err != nil {
		t.Fatal(err)
	}

	if document.Version != 1 || len(document.Configs) != 2 || document.Configs[0].Name != "aaa" || document.Configs[1].Prefix != "APP_" {
		t.Fatalf("document: %+v", document)
	}

	if document.Configs[0].Variables == nil {
		t.Fatal("empty variables must be an array")
	}

	fields := document.Configs[1].Variables
	if fields[0]["name"] != "APP_HOST" || fields[0]["description"] != "API address\nUse the staging endpoint" || fields[0]["notEmpty"] != true || fields[0]["default"] != "https://api.example.test" {
		t.Fatalf("host: %#v", fields[0])
	}

	if _, exists := fields[3]["default"]; exists {
		t.Fatal("secret default key present")
	}

	if fields[3]["secret"] != true || fields[3]["hasDefault"] != false {
		t.Fatalf("secret metadata: %#v", fields[3])
	}

	if fields[1]["hasDefault"] != false || fields[1]["default"] != "0" {
		t.Fatalf("zero metadata: %#v", fields[1])
	}

	if strings.Contains(output.String(), "hidden-default") || strings.Contains(output.String(), "environment-must-not-appear") {
		t.Fatal("runtime or secret value exposed")
	}

	if !strings.HasPrefix(output.String(), "{\n  \"version\": 1,") || !strings.HasSuffix(output.String(), "\n") {
		t.Fatal("formatting")
	}

	if _, err := handle.Value(); !errors.Is(err, ErrNotLoaded) {
		t.Fatalf("handle state: %v", err)
	}
}

func TestWriteManifestMarkdown(t *testing.T) {
	loader := MakeLoader()
	loader.Register[envExampleConfig]("app")
	var output bytes.Buffer
	if err := loader.WriteManifestMarkdown(&output); err != nil {
		t.Fatal(err)
	}

	for _, text := range []string{
		"## app\n", "Prefix: `APP_`", "Required, non-empty", "API address<br>Use the staging endpoint",
		"| `APP_TOKEN` | `secret.Secret` | Required | Yes | — |",
		"| `APP_COUNT` | `int` | Optional | No | — |",
	} {
		if !strings.Contains(output.String(), text) {
			t.Fatalf("missing %q:\n%s", text, &output)
		}
	}

	if strings.Contains(output.String(), "hidden-default") {
		t.Fatal("secret default exposed")
	}
}

func TestManifestMarkdownEscaping(t *testing.T) {
	got := escapeManifestMarkdownText("a|b`[link](x)<script>&_*\\\r\nnext\x00")
	want := "a&#124;b&#96;&#91;link&#93;(x)&#60;script&#62;&#38;&#95;&#42;&#92;<br>next&#92;u0000"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestManifestExportErrors(t *testing.T) {
	for _, format := range []string{"json", "markdown"} {
		t.Run(format, func(t *testing.T) {
			loader := MakeLoader()
			loader.Register[envExampleConfig]("app")
			write := loader.WriteManifestJSON
			if format == "markdown" {
				write = loader.WriteManifestMarkdown
			}

			cause := errors.New("writer failed")
			if err := write(exampleErrorWriter{cause}); !errors.Is(err, cause) {
				t.Fatalf("writer cause: %v", err)
			}

			if err := write(exampleErrorWriter{}); !errors.Is(err, io.ErrShortWrite) {
				t.Fatalf("short write: %v", err)
			}

			loader.Register[envExampleConfig]("app")
			var output bytes.Buffer
			if err := write(&output); err == nil || output.Len() != 0 {
				t.Fatalf("invalid manifest: %v, %q", err, &output)
			}
		})
	}
}

func TestEmptyManifestJSON(t *testing.T) {
	var output bytes.Buffer
	if err := MakeLoader().WriteManifestJSON(&output); err != nil {
		t.Fatal(err)
	}

	if output.String() != "{\n  \"version\": 1,\n  \"configs\": []\n}\n" {
		t.Fatalf("empty manifest: %s", &output)
	}
}

func ExampleLoader_WriteManifestMarkdown() {
	loader := MakeLoader()
	loader.Register[struct {
		URL string `env:"URL,notEmpty" envDescription:"Service endpoint"`
	}]("api")
	var output bytes.Buffer
	if err := loader.WriteManifestMarkdown(&output); err != nil {
		panic(err)
	}

	fmt.Print(output.String())
	// Output:
	// # Configuration manifest
	//
	// ## api
	//
	// Prefix: `API_`
	//
	// | ENV | Type | Requirement | Secret | Default | Description |
	// | --- | --- | --- | --- | --- | --- |
	// | `API_URL` | `string` | Required, non-empty | No | — | Service endpoint |
}

func TestManifestExportRenderErrorWritesNothing(t *testing.T) {
	loader := MakeLoader()
	loader.Register[struct {
		Value failingDefaultText `env:"VALUE"`
	}]("app")
	for _, write := range []func(io.Writer) error{loader.WriteManifestJSON, loader.WriteManifestMarkdown} {
		var output bytes.Buffer
		err := write(&output)
		if !errors.Is(err, defaultRenderCause) || output.Len() != 0 {
			t.Fatalf("render failure: %v, %q", err, &output)
		}
	}
}

func ExampleLoader_WriteManifestJSON() {
	loader := MakeLoader()
	var output bytes.Buffer
	if err := loader.WriteManifestJSON(&output); err != nil {
		panic(err)
	}

	fmt.Print(output.String())
	// Output:
	// {
	//   "version": 1,
	//   "configs": []
	// }
}

func TestManifestMarkdownCode(t *testing.T) {
	for _, value := range []string{"POSTGRES_HOST", "POSTGRES_", "[]string", "map[string]*int"} {
		if got := manifestMarkdownCode(value); got != "`"+value+"`" {
			t.Fatalf("identifier %q rendered as %q", value, got)
		}
	}

	for _, value := range []string{"A|B", "A`B", "A\nB", "A\x00B"} {
		if got := manifestMarkdownCode(value); got != escapeManifestMarkdownText(value) {
			t.Fatalf("unsafe identifier %q rendered as %q", value, got)
		}
	}
}
