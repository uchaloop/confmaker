package confmaker

import (
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/uchaloop/secret/v2"
)

type jsonEndpoint struct {
	URL     string        `json:"url"`
	Timeout time.Duration `json:"timeout"`
	Tags    []string      `json:"tags"`
}

type jsonConfig struct {
	Endpoints []jsonEndpoint           `env:"ENDPOINTS" envFormat:"json"`
	Limits    map[string]int           `env:"LIMITS" envFormat:"json"`
	Retry     jsonRetry                `env:"RETRY" envFormat:"json"`
	Fallback  *jsonEndpoint            `env:"FALLBACK" envFormat:"json"`
	Windows   map[string]time.Duration `env:"WINDOWS" envFormat:"json"`
	Plain     []string                 `env:"PLAIN"`
}

type jsonRetry struct {
	Attempts int           `json:"attempts"`
	Delay    time.Duration `json:"delay"`
	Started  time.Time     `json:"started"`
}

func (c *jsonConfig) SetDefaults() {
	c.Endpoints = []jsonEndpoint{{URL: "http://default", Timeout: time.Second}}
	c.Limits = map[string]int{"b": 2, "a": 1}
	c.Retry = jsonRetry{Attempts: 3, Delay: 500 * time.Millisecond}
	c.Windows = map[string]time.Duration{"slow": time.Minute}
	c.Plain = []string{"x", "y"}
}

// jsonParse compiles the envFormat json parser for T and runs it on raw.
func jsonParse[T any](raw string) (T, error) {
	var target T
	parse, err := jsonParser(reflect.TypeFor[T]())
	if err != nil {
		return target, err
	}

	err = parse(reflect.ValueOf(&target).Elem(), raw)

	return target, err
}

func TestJSONLoadsStructsListsAndMaps(t *testing.T) {
	t.Parallel()

	env := map[string]string{
		"CONFXJSON_ENDPOINTS": `[{"url":"http://a","timeout":"30s","tags":["x"]},{"url":"http://b","timeout":"1m30s"}]`,
		"CONFXJSON_LIMITS":    `{"rps":100}`,
		"CONFXJSON_RETRY":     `{"attempts":5,"delay":"2s","started":"2026-01-02T03:04:05Z"}`,
		"CONFXJSON_FALLBACK":  `{"url":"http://fallback","timeout":"5s"}`,
		"CONFXJSON_WINDOWS":   `{"fast":"100ms"}`,
	}

	cfg, err := Load[jsonConfig](WithName("confxjson"), WithEnv(env))
	if err != nil {
		t.Fatal(err)
	}

	want := jsonConfig{
		Endpoints: []jsonEndpoint{
			{URL: "http://a", Timeout: 30 * time.Second, Tags: []string{"x"}},
			{URL: "http://b", Timeout: 90 * time.Second},
		},
		Limits:   map[string]int{"rps": 100},
		Retry:    jsonRetry{Attempts: 5, Delay: 2 * time.Second, Started: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)},
		Fallback: &jsonEndpoint{URL: "http://fallback", Timeout: 5 * time.Second},
		Windows:  map[string]time.Duration{"fast": 100 * time.Millisecond},
		Plain:    []string{"x", "y"},
	}
	if !reflect.DeepEqual(cfg, want) {
		t.Fatalf("got  %+v\nwant %+v", cfg, want)
	}
}

func TestJSONReplacesTheWholeDefault(t *testing.T) {
	t.Parallel()

	env := map[string]string{
		"CONFXJSON_RETRY":  `{"attempts":1}`,
		"CONFXJSON_LIMITS": `{"c":3}`,
	}

	// Only attempts is written, so delay does not survive from the default, and
	// the map holds exactly what the variable says.

	cfg, err := Load[jsonConfig](WithName("confxjson"), WithEnv(env))
	if err != nil {
		t.Fatal(err)
	}

	if cfg.Retry != (jsonRetry{Attempts: 1}) {
		t.Fatalf("object merged into its default: %+v", cfg.Retry)
	}

	if !reflect.DeepEqual(cfg.Limits, map[string]int{"c": 3}) {
		t.Fatalf("map merged into its default: %v", cfg.Limits)
	}

	if len(cfg.Endpoints) != 1 || cfg.Endpoints[0].URL != "http://default" {
		t.Fatalf("an unset variable lost its default: %+v", cfg.Endpoints)
	}
}

func TestJSONNullAndEmptyCollections(t *testing.T) {
	t.Parallel()

	env := map[string]string{
		"CONFXJSON_ENDPOINTS": `null`,
		"CONFXJSON_LIMITS":    `{}`,
		"CONFXJSON_WINDOWS":   `null`,
		"CONFXJSON_FALLBACK":  `null`,
	}

	cfg, err := Load[jsonConfig](WithName("confxjson"), WithEnv(env))
	if err != nil {
		t.Fatal(err)
	}

	if cfg.Endpoints != nil || cfg.Windows != nil || cfg.Fallback != nil {
		t.Fatalf("null did not clear: %+v", cfg)
	}

	if cfg.Limits == nil || len(cfg.Limits) != 0 {
		t.Fatalf("{} is not an empty map: %#v", cfg.Limits)
	}

	list, err := jsonParse[[]string](" [ ] ")
	if err != nil || list == nil || len(list) != 0 {
		t.Fatalf("[] is not an empty list: %#v, %v", list, err)
	}
}

func TestJSONRejectsBadValues(t *testing.T) {
	cases := []struct {
		name, raw, want string
		parse           func(string) error
	}{
		{"empty", "", "JSON value is empty; write [] for an empty list or null for none", endpoints},
		{"blank", " \n\t", "JSON value is empty", endpoints},
		{"empty struct", "", "write {} for an object with zero fields", retry},
		{"null struct", "null", "JSON at the top level: null is not allowed for confmaker.jsonRetry", retry},
		{"null string", `[{"url":null}]`, `JSON at "/0/url": null is not allowed for string`, endpoints},
		{"null duration", `{"delay":null}`, "null is not allowed for time.Duration", retry},
		{"duration number", `{"delay":30000}`, `JSON at "/delay": a duration must be a JSON string such as "30s", got 30000`, retry},
		{"duration in list", `[{"timeout":5}]`, `JSON at "/0/timeout": a duration must be a JSON string`, endpoints},
		{"bad duration", `{"delay":"soon"}`, `JSON at "/delay": time: invalid duration "soon"`, retry},
		{"unknown member", `{"attempts":1,"extra":true}`, `JSON at "/extra": unknown object member name`, retry},
		{"member case", `[{"URL":"x"}]`, `JSON at "/0/URL": unknown object member name`, endpoints},
		{"duplicate member", `{"attempts":1,"attempts":2}`, `invalid JSON at "/attempts", offset 14: duplicate object member name`, retry},
		{"wrong kind", `[{"url":1}]`, `JSON at "/0/url": a number cannot be read into string`, endpoints},
		{"wrong root", `{}`, `JSON at the top level: an object cannot be read into []confmaker.jsonEndpoint`, endpoints},
		{"truncated", `[{"url":"x"`, `invalid JSON at "/0", offset 11: unexpected EOF`, endpoints},
		{"trailing", `[] []`, `invalid JSON at the top level, offset 3: invalid character '[' after top-level value`, endpoints},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.parse(tc.raw)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("got %v, want %q", err, tc.want)
			}
		})
	}
}

func endpoints(raw string) error {
	_, err := jsonParse[[]jsonEndpoint](raw)
	return err
}

func retry(raw string) error {
	_, err := jsonParse[jsonRetry](raw)
	return err
}

func TestJSONErrorKeepsTheOriginal(t *testing.T) {
	t.Parallel()

	env := map[string]string{
		"CONFXJSON_RETRY":     `{"attempts":"five"}`,
		"CONFXJSON_ENDPOINTS": `[{"url":"x"`,
	}

	_, err := Load[jsonConfig](WithName("confxjson"), WithEnv(env))

	var semantic *json.SemanticError
	if !errors.As(err, &semantic) || semantic.JSONPointer != "/attempts" || semantic.GoType != reflect.TypeFor[int]() {
		t.Fatalf("semantic error not reachable: %#v from %v", semantic, err)
	}

	var syntactic *jsontext.SyntacticError
	if !errors.As(err, &syntactic) || syntactic.ByteOffset != 11 {
		t.Fatalf("syntactic error not reachable: %#v from %v", syntactic, err)
	}

	_, err = jsonParse[jsonRetry](`{"delay":"soon"}`)
	if !errors.As(err, &semantic) || semantic.Err == nil || !strings.Contains(semantic.Err.Error(), "invalid duration") {
		t.Fatalf("the cause behind a semantic error is lost: %v", err)
	}
}

func TestJSONErrorNamesTheVariable(t *testing.T) {
	t.Parallel()

	env := map[string]string{
		"CONFXJSON_RETRY": `{"attempts":"five"}`,
	}

	_, err := Load[jsonConfig](WithName("confxjson"), WithEnv(env))
	want := `config "confxjson": variable "CONFXJSON_RETRY": JSON at "/attempts": a string cannot be read into int`
	if err == nil || err.Error() != want {
		t.Fatalf("got %v, want %q", err, want)
	}
}

type jsonOwnForm struct {
	value any
	Any   any
}

func (j *jsonOwnForm) UnmarshalText(text []byte) error {
	j.value = string(text)
	return nil
}

func (j jsonOwnForm) MarshalText() ([]byte, error) { return []byte(j.value.(string)), nil }

func TestJSONTypeWithItsOwnFormIsNotInspected(t *testing.T) {
	got, err := jsonParse[[]jsonOwnForm](`["a","b"]`)
	if err != nil || len(got) != 2 || got[1].value != "b" {
		t.Fatalf("got %+v, %v", got, err)
	}
}

type jsonNamedBytes []byte

func (b *jsonNamedBytes) UnmarshalText(text []byte) error {
	*b = append((*b)[:0], text...)
	return nil
}

func TestJSONDeclarationsAreChecked(t *testing.T) {
	type nestedSecret struct {
		URL      string        `json:"url"`
		Password secret.Secret `json:"password"`
	}

	type hiddenSecret struct {
		Password *secret.Secret `json:"-"`
	}

	cases := []struct {
		name  string
		field reflect.StructField
		want  string
	}{
		{"unknown format", jsonField[[]string](`envFormat:"yaml"`), `unknown envFormat "yaml"`},
		{"empty format", jsonField[[]string](`envFormat:""`), `unknown envFormat ""`},
		{"separator", jsonField[[]string](`envFormat:"json" envSeparator:";"`), "cannot be combined with envSeparator"},
		{"key separator", jsonField[map[string]string](`envFormat:"json" envKeyValSeparator:"="`), "cannot be combined with envKeyValSeparator"},
		{"scalar", jsonField[string](`envFormat:"json"`), "envFormat json reads a struct, slice, array or map, not string"},
		{"pointer to scalar", jsonField[*int](`envFormat:"json"`), "not *int"},
		{"nested secret", jsonField[[]nestedSecret](`envFormat:"json"`), "Password: JSON contains secret type secret.Secret"},
		{"ignored secret", jsonField[hiddenSecret](`envFormat:"json"`), "Password: JSON contains secret type *secret.Secret"},
		{"interface", jsonField[map[string]any](`envFormat:"json"`), "JSON requires concrete types"},
		{"bytes", jsonField[[][]byte](`envFormat:"json"`), "JSON would read []uint8 as base64 text"},
		{"byte array", jsonField[struct{ Key [4]byte }](`envFormat:"json"`), "Key: JSON would read [4]uint8 as base64 text"},
		{"float key", jsonField[map[float64]string](`envFormat:"json"`), "JSON map key float64"},
		{"channel", jsonField[[]chan int](`envFormat:"json"`), "chan int cannot be read from JSON"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := fieldCodec(tc.field)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("got %v, want %q", err, tc.want)
			}
		})
	}

	if _, _, err := fieldCodec(jsonField[[]jsonNamedBytes](`envFormat:"json"`)); err != nil {
		t.Fatalf("bytes with a text form refused: %v", err)
	}
}

func jsonField[T any](tag reflect.StructTag) reflect.StructField {
	return reflect.StructField{Name: "Field", Type: reflect.TypeFor[T](), Tag: `env:"FIELD" ` + tag}
}

func TestJSONFormatWithoutAVariableIsRefused(t *testing.T) {
	type config struct {
		Empty struct {
			Value string `env:"VALUE"`
		} `envFormat:""`
	}

	_, err := compileSchema(reflect.TypeFor[config](), "CONFXJSON_")
	if err == nil || !strings.Contains(err.Error(), "field Empty declares envFormat but names no variable") {
		t.Fatalf("got %v", err)
	}
}

func TestJSONManifestIsDeterministicAndLoadsBack(t *testing.T) {
	t.Parallel()

	variables := manifested[jsonConfig](t, "confxjson")

	defaults := make(map[string]string, len(variables))
	for _, variable := range variables {
		defaults[variable.Name] = variable.Default
	}

	for name, want := range map[string]string{
		"CONFXJSON_ENDPOINTS": `[{"url":"http://default","timeout":"1s","tags":null}]`,
		"CONFXJSON_LIMITS":    `{"a":1,"b":2}`,
		"CONFXJSON_RETRY":     `{"attempts":3,"delay":"500ms","started":"0001-01-01T00:00:00Z"}`,
		"CONFXJSON_FALLBACK":  `null`,
		"CONFXJSON_WINDOWS":   `{"slow":"1m0s"}`,
		"CONFXJSON_PLAIN":     `x,y`,
	} {
		if defaults[name] != want {
			t.Errorf("%s = %s, want %s", name, defaults[name], want)
		}
	}

	// Writing every default back into the environment loads the same config.
	var original jsonConfig
	original.SetDefaults()
	loaded, err := Load[jsonConfig](WithName("confxjson"), WithEnv(defaults))
	if err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(loaded, original) {
		t.Fatalf("round trip changed the config:\n%+v\n%+v", loaded, original)
	}
}

func TestJSONThroughLoader(t *testing.T) {
	t.Parallel()

	env := map[string]string{
		"CONFXJSON_ENDPOINTS": `[{"url":"http://a","timeout":"soon"}]`,
	}

	_, err := Load[jsonConfig](WithEnv(env), WithName("confxjson"))
	want := `config "confxjson": variable "CONFXJSON_ENDPOINTS": JSON at "/0/timeout": time: invalid duration "soon"`
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("got %v, want %q", err, want)
	}
}

// FuzzJSONField checks that no input panics, and that whatever loads renders to
// text that loads back to the same value.
func FuzzJSONField(f *testing.F) {
	for _, seed := range []string{
		`[{"url":"http://a","timeout":"30s","tags":["x"]}]`,
		`null`, `[]`, `[{}]`, ``, ` `, `[{"timeout":1}]`, "[{\"url\":\"\\u0000\"}]",
		`[{"timeout":"-1h2m3.5s"}]`, `[{"url":"a","url":"b"}]`,
	} {
		f.Add(seed)
	}

	parse, err := jsonParser(reflect.TypeFor[[]jsonEndpoint]())
	if err != nil {
		f.Fatal(err)
	}

	f.Fuzz(func(t *testing.T, raw string) {
		var first []jsonEndpoint
		if err := parse(reflect.ValueOf(&first).Elem(), raw); err != nil {
			return
		}

		rendered, err := renderJSON(reflect.ValueOf(first))
		if err != nil {
			t.Fatalf("render %q: %v", raw, err)
		}

		var second []jsonEndpoint
		if err := parse(reflect.ValueOf(&second).Elem(), rendered); err != nil {
			t.Fatalf("rendered %q does not load: %v", rendered, err)
		}

		if !reflect.DeepEqual(first, second) {
			t.Fatalf("round trip of %q: %#v != %#v", raw, first, second)
		}
	})
}

func BenchmarkJSONField(b *testing.B) {
	raw := `[{"url":"http://a:9000","timeout":"30s","tags":["primary","eu"]},{"url":"http://b:9000","timeout":"1m","tags":["replica"]},{"url":"http://c:9000","timeout":"500ms"}]`

	parse, err := jsonParser(reflect.TypeFor[[]jsonEndpoint]())
	if err != nil {
		b.Fatal(err)
	}

	var value []jsonEndpoint
	if err := parse(reflect.ValueOf(&value).Elem(), raw); err != nil {
		b.Fatal(err)
	}

	b.Run("parse", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			var target []jsonEndpoint
			if err := parse(reflect.ValueOf(&target).Elem(), raw); err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("render", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			if _, err := renderJSON(reflect.ValueOf(value)); err != nil {
				b.Fatal(err)
			}
		}
	})
}

func TestJSONIgnoresOnlyAFieldTaggedDash(t *testing.T) {
	t.Parallel()

	type ignored struct {
		Skipped chan int `json:"-"`
		Name    string   `json:"name"`
	}

	if _, _, err := fieldCodec(jsonField[ignored](`envFormat:"json"`)); err != nil {
		t.Fatalf(`json:"-" was not ignored: %v`, err)
	}

	// json/v2 names this field "-": it is not ignored, and a chan cannot be read.
	type named struct {
		Dash chan int `json:"-,omitempty"`
	}

	if _, _, err := fieldCodec(jsonField[named](`envFormat:"json"`)); err == nil || !strings.Contains(err.Error(), "chan int cannot be read from JSON") {
		t.Fatalf(`json:"-,omitempty" was treated as ignored: %v`, err)
	}

	var decoded named
	if err := json.Unmarshal([]byte(`{"-":null}`), &decoded); err == nil {
		t.Fatal(`json/v2 accepted a member "-" for the chan field; the rule changed`)
	}
}

func TestJSONConflictingNamesAreRefusedAtRegistration(t *testing.T) {
	t.Parallel()

	// Built with reflect: vet refuses a repeated json tag written in source.
	conflict := reflect.StructOf([]reflect.StructField{
		{Name: "First", Type: reflect.TypeFor[string](), Tag: `json:"value"`},
		{Name: "Second", Type: reflect.TypeFor[string](), Tag: `json:"value"`},
	})

	outer := reflect.StructOf([]reflect.StructField{
		{Name: "Inner", Type: conflict, Tag: `json:"inner"`},
	})

	config := reflect.StructOf([]reflect.StructField{
		{Name: "Payload", Type: reflect.SliceOf(outer), Tag: `env:"PAYLOAD" envFormat:"json"`},
	})

	// Only the declaration is compiled: no variable is read, nothing is dumped.
	_, err := compileSchema(config, "CONFXJSON_")
	if err == nil || !strings.Contains(err.Error(), `conflict over JSON object name "value"`) {
		t.Fatalf("got %v", err)
	}
}

type jsonEmbeddedName struct {
	Name string `json:"name"`
}

type jsonShadowing struct {
	jsonEmbeddedName
	Name string `json:"name"`
}

func TestJSONEmbeddingFollowsJSONRules(t *testing.T) {
	t.Parallel()

	// A shallower field shadows an embedded one with the same name: valid JSON.
	got, err := jsonParse[jsonShadowing](`{"name":"outer"}`)
	if err != nil || got.Name != "outer" {
		t.Fatalf("got %+v, %v", got, err)
	}
}

type jsonCaseInsensitive struct {
	Name string `json:"name,case:ignore"`
}

type jsonEmbeddedMap struct {
	Name  string            `json:"name"`
	Extra map[string]string `json:",embed"`
}

// The JSON tag options of json/v2 are honoured: they are part of the contract,
// not exceptions to it.
func TestJSONTagOptionsAreHonoured(t *testing.T) {
	t.Parallel()

	insensitive, err := jsonParse[jsonCaseInsensitive](`{"NAME":"db"}`)
	if err != nil || insensitive.Name != "db" {
		t.Fatalf("case:ignore: %+v, %v", insensitive, err)
	}

	// An embedded map holds members no field names, so they are not unknown.
	embedded, err := jsonParse[jsonEmbeddedMap](`{"name":"db","nmae":"typo"}`)
	if err != nil || embedded.Extra["nmae"] != "typo" {
		t.Fatalf("embedded map: %+v, %v", embedded, err)
	}
}

type jsonOmit struct {
	Empty []string `json:"empty,omitempty"`
	Zero  []string `json:"zero,omitzero"`
}

func TestJSONOmitEmptyAndOmitZeroRoundTrip(t *testing.T) {
	t.Parallel()

	for name, value := range map[string]jsonOmit{
		"empty collections": {Empty: []string{}, Zero: []string{}},
		"nil collections":   {},
	} {
		rendered, err := renderJSON(reflect.ValueOf(value))
		if err != nil {
			t.Fatal(err)
		}

		back, err := jsonParse[jsonOmit](rendered)
		if err != nil {
			t.Fatal(err)
		}

		// omitempty drops an empty slice, which comes back nil; omitzero keeps it.
		if back.Empty != nil || (back.Zero == nil) != (value.Zero == nil) {
			t.Errorf("%s: %s loaded back as %#v", name, rendered, back)
		}
	}
}
