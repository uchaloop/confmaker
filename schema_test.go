package confmaker

import (
	"bytes"
	"strings"
	"sync/atomic"
	"testing"
)

type schemaConfig struct {
	Value countedByte `env:"VALUE"`
}

var schemaDefaultCalls atomic.Int32

func (c *schemaConfig) SetDefaults() { schemaDefaultCalls.Add(1) }

func TestLoaderLoadsWithoutEncoder(t *testing.T) {
	schemaDefaultCalls.Store(0)
	loader := MakeLoader(WithEnv(map[string]string{"CONFXSCHEMA_VALUE": "abcd"}))
	handle := loader.Add[schemaConfig](WithName("confxschema"))
	if schemaDefaultCalls.Load() != 0 {
		t.Fatal("registration evaluated defaults")
	}

	if err := loader.Load(); err != nil {
		t.Fatal(err)
	}

	cfg, err := handle.Value()
	if err != nil || cfg.Value != 4 || schemaDefaultCalls.Load() != 1 {
		t.Fatalf("value %v, %v, defaults calls %d", cfg.Value, err, schemaDefaultCalls.Load())
	}

	typo := WithEnv(map[string]string{"CONFXSCHEMA_VALUE": "abcd", "CONFXSCHEMA_VLAUE": "typo"})
	if _, err := Load[schemaConfig](typo, WithName("confxschema")); err == nil || !strings.Contains(err.Error(), "unknown configuration variable") {
		t.Fatalf("schema did not check typo: %v", err)
	}
}

func TestDumpAndManifestStillRequireEncoder(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	_, err := Load[schemaConfig](WithDump(&out), WithEnv(nil), WithName("confxschema"))
	if err == nil || !strings.Contains(err.Error(), "TextMarshaler") || !strings.Contains(err.Error(), `config "confxschema"`) {
		t.Fatalf("dump error: %v", err)
	}

	if _, err := Manifest[schemaConfig](WithName("confxschema")); err == nil || !strings.Contains(err.Error(), `config "confxschema"`) {
		t.Fatalf("manifest error: %v", err)
	}
}

type schemaNested struct {
	Left struct {
		Value string `env:"VALUE"`
	} `envPrefix:"LEFT_"`
	Right struct {
		Value string `env:"VALUE"`
	} `envPrefix:"RIGHT_"`
	Items []string `env:"ITEMS"`
}

func (c *schemaNested) SetDefaults() { c.Items = []string{"default"} }

func TestLoadersDoNotShareValues(t *testing.T) {
	t.Parallel()

	env := map[string]string{"CONFXSCHEMA_LEFT_VALUE": "left", "CONFXSCHEMA_RIGHT_VALUE": "right"}
	first, err := Load[schemaNested](WithEnv(env), WithName("confxschema"))
	if err != nil {
		t.Fatal(err)
	}

	first.Items[0] = "mutated"
	env["CONFXSCHEMA_LEFT_VALUE"] = "changed"
	second, err := Load[schemaNested](WithEnv(env), WithName("confxschema"))
	if err != nil {
		t.Fatal(err)
	}

	if first.Left.Value != "left" || second.Left.Value != "changed" || second.Right.Value != "right" || second.Items[0] != "default" {
		t.Fatalf("shared config or invalid paths: %+v, %+v", first, second)
	}
}

type invalidDefaults struct {
	Unsupported chan int `env:"UNSUPPORTED"`
}

func (*invalidDefaults) SetDefaults() { panic("invalid declaration must fail before defaults") }

func TestInvalidDeclarationFailsBeforeDefaults(t *testing.T) {
	t.Parallel()

	if _, err := Load[invalidDefaults](WithEnv(nil), WithName("confxschema")); err == nil {
		t.Fatal("invalid config accepted")
	}

	if _, err := Manifest[invalidDefaults](WithName("confxschema")); err == nil {
		t.Fatal("invalid manifest accepted")
	}
}
