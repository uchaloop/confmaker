package confmaker

import (
	"bytes"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"
)

var errMarshalDefault = errors.New("default cannot be marshaled")

type brokenText int

func (*brokenText) UnmarshalText([]byte) error   { return nil }
func (*brokenText) MarshalText() ([]byte, error) { return nil, errMarshalDefault }

type brokenTextConfig struct {
	Value brokenText `env:"VALUE"`
}

type displayInt int

func (displayInt) String() string { return "display only" }

func TestLoadDoesNotRenderDefaults(t *testing.T) {
	t.Parallel()

	_, err := Load[brokenTextConfig](WithName("confxrender"), WithEnv(nil))
	if err != nil {
		t.Fatalf("load invoked marshaler: %v", err)
	}

	_, err = Manifest[brokenTextConfig](WithName("confxrender"))
	if !errors.Is(err, errMarshalDefault) || !strings.Contains(err.Error(), "CONFXRENDER_VALUE") {
		t.Fatalf("lost marshal error or field context: %v", err)
	}
}

func TestPointerMarshalerInMapRoundTrips(t *testing.T) {
	original := map[pointerMarshaled]pointerMarshaled{{n: 7}: {n: 9}}
	raw, err := renderValue(reflect.ValueOf(original), ",", ":")
	if err != nil || raw != "7:9" {
		t.Fatalf("render: %q, %v", raw, err)
	}

	parsed, err := parseInto[map[pointerMarshaled]pointerMarshaled](t, raw)
	if err != nil || !reflect.DeepEqual(parsed, original) {
		t.Fatalf("round trip: %v, %v", parsed, err)
	}
}

func TestRenderingUsesScalarSyntaxNotStringer(t *testing.T) {
	original := displayInt(42)
	raw, err := renderValue(reflect.ValueOf(original), ",", ":")
	if err != nil || raw != "42" {
		t.Fatalf("render: %q, %v", raw, err)
	}

	parsed, err := parseInto[displayInt](t, raw)
	if err != nil || parsed != original {
		t.Fatalf("round trip: %v, %v", parsed, err)
	}
}

func TestAmbiguousCollectionDefaultsAreRejected(t *testing.T) {
	cases := []struct {
		name          string
		value         any
		separator, kv string
	}{
		{"slice separator", []string{"a,b", "c"}, ",", ":"},
		{"single empty", []string{""}, ",", ":"},
		{"overlap", []string{"a", "b"}, "aa", ":"},
		{"map entry separator", map[string]string{"a": "b,c"}, ",", ":"},
		{"map key separator", map[string]string{"a:b": "c"}, ",", ":"},
		{"map whitespace", map[string]string{"a": " b"}, ",", ":"},
		{"overlapping separators", map[string]string{"a": "b"}, ":", ":"},
		{"nil slice pointer", []*int{nil}, ",", ":"},
		{"nil map pointer", map[string]*int{"a": nil}, ",", ":"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if raw, err := renderValue(reflect.ValueOf(tc.value), tc.separator, tc.kv); err == nil {
				t.Fatalf("ambiguous default accepted: %q", raw)
			}
		})
	}
}

func TestCollectionDefaultsRoundTrip(t *testing.T) {
	cases := []struct {
		value         any
		separator, kv string
	}{
		{[]string{}, ",", ":"},
		{[]string{"", ""}, ",", ":"},
		{[]string{"a,b", "c"}, ";", ":"},
		{map[string]string{"url": "https://example.test", "empty": ""}, ",", ":"},
		{map[string]int{"basic": 1, "pro": 9}, "|", "="},
	}

	for _, tc := range cases {
		value := reflect.ValueOf(tc.value)
		raw, err := renderValue(value, tc.separator, tc.kv)
		if err != nil {
			t.Fatal(err)
		}

		parse, err := fieldParser(value.Type(), tc.separator, tc.kv)
		if err != nil {
			t.Fatal(err)
		}

		parsed := reflect.New(value.Type()).Elem()
		if err := parse(parsed, raw); err != nil {
			t.Fatal(err)
		}

		if !reflect.DeepEqual(tc.value, parsed.Interface()) {
			t.Fatalf("round trip through %q: %v != %v", raw, tc.value, parsed.Interface())
		}
	}
}

type ambiguousDefaults struct {
	Items []string `env:"ITEMS"`
}

func (c *ambiguousDefaults) SetDefaults() { c.Items = []string{"a,b"} }
func TestManifestRejectsAmbiguousDefaultWithContext(t *testing.T) {
	_, err := Manifest[ambiguousDefaults](WithName("confxrender"))
	if err == nil || !strings.Contains(err.Error(), "field Items") || !strings.Contains(err.Error(), "CONFXRENDER_ITEMS") {
		t.Fatalf("missing context: %v", err)
	}
}

func TestDumpEscapesControlsAndMasksSecrets(t *testing.T) {
	env := environment{
		"CONFXDUMP_VALUE":  "first\nsecond\t\x1b[31m",
		"CONFXDUMP_SECRET": "never print this\n",
	}

	var out bytes.Buffer
	err := writeDump(&out, []dumpSection{{label: "dump", variables: []Variable{
		{Name: "CONFXDUMP_VALUE", Type: "string"},
		{Name: "CONFXDUMP_SECRET", Type: "secret.Secret", Secret: true},
	}}}, env)
	if err != nil {
		t.Fatal(err)
	}

	text := out.String()
	if strings.Count(text, "\n") != 3 || strings.Contains(text, "never print") || strings.Contains(text, "\x1b") {
		t.Fatalf("unsafe dump: %q", text)
	}

	if !strings.Contains(text, `first\nsecond\t\x1b[31m`) {
		t.Fatalf("controls not visible: %q", text)
	}
}

type encodeOnlyInt int

func (encodeOnlyInt) MarshalText() ([]byte, error) { return []byte("not an integer"), nil }

func TestMarshalOnlyScalarUsesItsParserSyntax(t *testing.T) {
	raw, err := renderValue(reflect.ValueOf(encodeOnlyInt(7)), ",", ":")
	if err != nil || raw != "7" {
		t.Fatalf("render: %q, %v", raw, err)
	}
}

func TestManifestRequiresEncoderForCustomDecoder(t *testing.T) {
	type cfg struct {
		Value countedByte `env:"VALUE"`
	}

	if _, err := Manifest[cfg](WithName("confxrender")); err == nil || !strings.Contains(err.Error(), "TextMarshaler") {
		t.Fatalf("missing encoder: %v", err)
	}
}

type duplicateTextKey int

func (*duplicateTextKey) UnmarshalText([]byte) error  { return nil }
func (duplicateTextKey) MarshalText() ([]byte, error) { return []byte("same"), nil }

func TestDuplicateEncodedMapKeysAreRejected(t *testing.T) {
	value := map[duplicateTextKey]string{1: "a", 2: "b"}
	if _, err := renderValue(reflect.ValueOf(value), ",", ":"); err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("duplicate map text: %v", err)
	}
}

func TestManifestAggregatesMarshalErrors(t *testing.T) {
	type cfg struct {
		First  brokenText `env:"FIRST"`
		Second brokenText `env:"SECOND"`
	}

	_, err := Manifest[cfg](WithName("confxrender"))
	if err == nil || !errors.Is(err, errMarshalDefault) || !strings.Contains(err.Error(), "field First") || !strings.Contains(err.Error(), "field Second") {
		t.Fatalf("marshal errors not aggregated: %v", err)
	}
}

// TestDefaultsRenderForCollections covers what a generated .env.example carries
// for the composite types: the text has to be something the variable could
// carry back.
func TestDefaultsRenderForCollections(t *testing.T) {
	type config struct {
		Brokers []string          `env:"BROKERS" envSeparator:";"`
		Tiers   map[string]int    `env:"TIERS"`
		Timeout *time.Duration    `env:"TIMEOUT"`
		Absent  *int              `env:"ABSENT"`
		Labels  map[string]string `env:"LABELS"`
	}

	var cfg config
	cfg.Brokers = []string{"a:9092", "b:9092"}
	cfg.Tiers = map[string]int{"pro": 9, "basic": 1}
	timeout := 30 * time.Second
	cfg.Timeout = &timeout

	bindings, err := compileSchema(reflect.TypeFor[config](), "CONFXAPP_")
	if err != nil {
		t.Fatalf("bind: %v", err)
	}

	variables, err := describeFields(reflect.ValueOf(&cfg).Elem(), bindings)
	if err != nil {
		t.Fatal(err)
	}

	rendered := make(map[string]string, len(variables))
	for _, b := range variables {
		rendered[b.Name] = b.Default
	}

	want := map[string]string{
		"CONFXAPP_BROKERS": "a:9092;b:9092",
		"CONFXAPP_TIERS":   "basic:1,pro:9",
		"CONFXAPP_TIMEOUT": "30s",
		"CONFXAPP_ABSENT":  "",
		"CONFXAPP_LABELS":  "",
	}

	for name, expected := range want {
		if rendered[name] != expected {
			t.Errorf("%s rendered as %q, want %q", name, rendered[name], expected)
		}
	}
}

// TestDumpReportsWhereAValueComesFrom covers the branches of the source column
// that a set variable never reaches.
func TestDumpReportsWhereAValueComesFrom(t *testing.T) {
	t.Parallel()

	type config struct {
		Host    string `env:"HOST,required"`
		Timeout string `env:"TIMEOUT"`
		Spare   string `env:"SPARE"`
	}

	var out bytes.Buffer

	// Host is required and unset, Timeout has a default, Spare has neither.
	if _, err := Load[config](WithDump(&out), WithEnv(nil), WithName("confxapp")); err == nil {
		t.Fatal("expected the required variable to fail the load")
	}

	dump := out.String()
	for _, want := range []string{"required", "zero value"} {
		if !strings.Contains(dump, want) {
			t.Errorf("dump is missing the %q source:\n%s", want, dump)
		}
	}
}
