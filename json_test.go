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

type jsonCollectionConfig struct {
	Headers map[string]string        `env:"HEADERS" envFormat:"json"`
	Ports   []int                    `env:"PORTS" envFormat:"json"`
	Delays  map[string]time.Duration `env:"DELAYS" envFormat:"json"`
	Groups  map[string][]string      `env:"GROUPS" envFormat:"json"`
	Times   []time.Time              `env:"TIMES" envFormat:"json"`
}

func (c *jsonCollectionConfig) SetDefaults() {
	c.Headers = map[string]string{"old": "value"}
	c.Ports = []int{80, 443}
	c.Delays = map[string]time.Duration{"retry": time.Second}
}

func loadJSONValue[T any](raw string) (T, error) {
	cfg, err := Load[struct {
		Value T `env:"VALUE" envFormat:"json"`
	}]("app", WithEnv(map[string]string{"APP_VALUE": raw}))
	if err != nil {
		var zero T

		return zero, err
	}

	return cfg.Value, nil
}

func TestJSONCollectionsAndTextValues(t *testing.T) {
	cfg, err := Load[jsonCollectionConfig]("app", WithEnv(map[string]string{
		"APP_HEADERS": `{"X-Service":"catalog"}`, "APP_PORTS": `[8080,8081]`,
		"APP_DELAYS": `{"retry":"3s"}`, "APP_GROUPS": `{"read":["a","b"]}`,
		"APP_TIMES": `["2026-01-02T03:04:05Z"]`,
	}))
	if err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(cfg.Headers, map[string]string{"X-Service": "catalog"}) || !reflect.DeepEqual(cfg.Ports, []int{8080, 8081}) || cfg.Delays["retry"] != 3*time.Second || len(cfg.Groups["read"]) != 2 || cfg.Times[0].Year() != 2026 {
		t.Fatal(cfg)
	}

	array, err := loadJSONValue[[2]int](`[1,2]`)
	if err != nil || array != [2]int{1, 2} {
		t.Fatal(array, err)
	}

	keys, err := loadJSONValue[map[int]string](`{"1":"one"}`)
	if err != nil || keys[1] != "one" {
		t.Fatal(keys, err)
	}
}

func TestJSONNullAndEmpty(t *testing.T) {
	for _, raw := range []string{"null", "[]"} {
		list, err := loadJSONValue[[]int](raw)
		if err != nil || len(list) != 0 || (list == nil) != (raw == "null") {
			t.Fatal(raw, list, err)
		}
	}

	pointer, err := loadJSONValue[*[]int]("null")
	if err == nil || pointer != nil {
		t.Fatal(pointer, err)
	}

	empty, err := loadJSONValue[map[string]int](`{}`)
	if err != nil || empty == nil || len(empty) != 0 {
		t.Fatal(empty, err)
	}
}

func TestJSONInvalidValuesAndCauses(t *testing.T) {
	for _, raw := range []string{"", `[null]`, `["bad"]`, `{}`, `[1,]`, `[NaN]`, `[1] trailing`} {
		_, err := loadJSONValue[[]int](raw)
		if err == nil || ConfigErrors(err)[0].Kind != ErrorParse || !strings.Contains(err.Error(), "APP_VALUE") {
			t.Fatal(raw, err)
		}
	}

	_, err := loadJSONValue[map[string]int](`{"a":1,"a":2}`)
	if err == nil {
		t.Fatal("duplicate key accepted")
	}

	_, err = loadJSONValue[[]int](`[1,]`)
	var syntax *jsontext.SyntacticError
	if !errors.As(err, &syntax) {
		t.Fatal(err)
	}

	_, err = loadJSONValue[[]time.Duration](`["soon"]`)
	var semantic *json.SemanticError
	if !errors.As(err, &semantic) || !strings.Contains(err.Error(), "/0") {
		t.Fatal(err)
	}

	_, err = loadJSONValue[[]time.Duration](`[3000]`)
	if err == nil {
		t.Fatal("duration without unit accepted")
	}
}

func rejectJSONType[T any](t *testing.T) {
	t.Helper()

	loader := MakeLoader(WithEnv(nil))
	loader.Register[struct {
		Value T `env:"VALUE" envFormat:"json"`
	}]("app")
	_, manifestErr := loader.Manifest()

	for _, err := range []error{loader.Load(), manifestErr} {
		problems := ConfigErrors(err)
		if len(problems) == 0 || problems[0].Kind != ErrorDeclaration {
			t.Fatal("declaration accepted", reflect.TypeFor[T](), err)
		}
	}
}

type jsonOnlyObject struct{ Name string }

func (*jsonOnlyObject) UnmarshalJSON([]byte) error { return nil }

type jsonTextValue struct{ value string }

func (v *jsonTextValue) UnmarshalText(raw []byte) error {
	v.value = string(raw)

	return nil
}
func (v jsonTextValue) MarshalText() ([]byte, error) { return []byte(v.value), nil }

type jsonTextWithSecret struct{ hidden secret.Secret }

func (*jsonTextWithSecret) UnmarshalText([]byte) error { return nil }

func TestJSONRejectsStructsAndUnsupportedTypes(t *testing.T) {
	rejectJSONType[struct{ Host string }](t)
	rejectJSONType[*struct{ Host string }](t)
	rejectJSONType[[]struct{ Host string }](t)
	rejectJSONType[map[string]struct{ Host string }](t)
	rejectJSONType[map[string][]*struct{ Host string }](t)
	rejectJSONType[[]jsonOnlyObject](t)
	rejectJSONType[map[string]any](t)
	rejectJSONType[[]byte](t)
	rejectJSONType[[2]byte](t)
	rejectJSONType[[]chan int](t)
	rejectJSONType[map[float64]string](t)
	rejectJSONType[string](t)
}

func TestJSONCustomTextAndManifestRoundTrip(t *testing.T) {
	text, err := loadJSONValue[[]jsonTextValue](`["a","b"]`)
	if err != nil || len(text) != 2 || text[0].value != "a" {
		t.Fatal(text, err)
	}

	variables, err := Manifest[jsonCollectionConfig]("app")
	if err != nil {
		t.Fatal(err)
	}

	env := make(map[string]string)
	for _, v := range variables {
		env[v.Name] = v.Default
	}

	cfg, err := Load[jsonCollectionConfig]("app", WithEnv(env))
	if err != nil {
		t.Fatal(err)
	}

	var expected jsonCollectionConfig
	expected.SetDefaults()
	if !reflect.DeepEqual(cfg, expected) {
		t.Fatal(cfg)
	}
}

func TestJSONFormatTagRestrictions(t *testing.T) {
	problems := []error{
		bindError[struct {
			Value []int `env:"VALUE" envFormat:"json" envSeparator:";"`
		}](t),
		bindError[struct {
			Value map[string]int `env:"VALUE" envFormat:"json" envKeyValSeparator:"="`
		}](t),
		bindError[struct {
			Value []int `env:"VALUE" envFormat:"yaml"`
		}](t),
		bindError[struct {
			Value []int `envFormat:"json"`
		}](t),
	}

	for _, err := range problems {
		if ConfigErrors(err)[0].Kind != ErrorDeclaration {
			t.Fatal(err)
		}
	}
}

func FuzzJSONField(f *testing.F) {
	for _, raw := range []string{`{}`, `null`, `{"a":["b"]}`, `{"a":null}`, `{"a":[]}`, `{"a":[null]}`} {
		f.Add(raw)
	}

	f.Fuzz(func(t *testing.T, raw string) {
		first, err := loadJSONValue[map[string][]string](raw)
		if err != nil {
			return
		}

		encoded, err := renderJSON(reflect.ValueOf(first))
		if err != nil {
			t.Fatal(err)
		}

		second, err := loadJSONValue[map[string][]string](encoded)
		if err != nil || !reflect.DeepEqual(first, second) {
			t.Fatal(encoded, err)
		}
	})
}

type jsonDualText struct{ Value string }

func (v *jsonDualText) UnmarshalText(raw []byte) error {
	v.Value = string(raw)

	return nil
}

func (v *jsonDualText) MarshalText() ([]byte, error) { return []byte(v.Value), nil }
func (*jsonDualText) UnmarshalJSON([]byte) error     { panic("JSON method must not run") }
func (*jsonDualText) MarshalJSON() ([]byte, error)   { panic("JSON method must not run") }

type jsonTextDefaults struct {
	Values []jsonDualText                `env:"VALUES" envFormat:"json"`
	Keys   map[jsonDualText]jsonDualText `env:"KEYS" envFormat:"json"`
	Times  []time.Time                   `env:"TIMES" envFormat:"json"`
}

func (c *jsonTextDefaults) SetDefaults() {
	c.Values = []jsonDualText{{Value: "hello"}}
	c.Keys = map[jsonDualText]jsonDualText{{Value: "key"}: {Value: "value"}}
	c.Times = []time.Time{time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)}
}

func TestJSONTextMethodsTakePrecedence(t *testing.T) {
	variables, err := Manifest[jsonTextDefaults]("app")
	if err != nil {
		t.Fatal(err)
	}

	if variables[0].Default != `["hello"]` || variables[1].Default != `{"key":"value"}` {
		t.Fatal(variables)
	}

	env := make(map[string]string)
	for _, variable := range variables {
		env[variable.Name] = variable.Default
	}

	loaded, err := Load[jsonTextDefaults]("app", WithEnv(env))
	if err != nil {
		t.Fatal(err)
	}

	var expected jsonTextDefaults
	expected.SetDefaults()
	if !reflect.DeepEqual(loaded, expected) {
		t.Fatalf("got %+v, want %+v", loaded, expected)
	}

	for _, raw := range []string{`[{}]`, `[1]`, `[true]`} {
		if _, err := loadJSONValue[[]jsonDualText](raw); err == nil {
			t.Fatalf("accepted non-text element: %s", raw)
		}
	}
}

type jsonReadOnlyText struct{ Value string }

func (v *jsonReadOnlyText) UnmarshalText(raw []byte) error {
	v.Value = string(raw)

	return nil
}

type jsonReadOnlyDefaults struct {
	Values []jsonReadOnlyText `env:"VALUES" envFormat:"json"`
}

func (c *jsonReadOnlyDefaults) SetDefaults() {
	c.Values = []jsonReadOnlyText{{Value: "default"}}
}

func TestJSONTextDefaultRequiresMarshaler(t *testing.T) {
	loaded, err := Load[jsonReadOnlyDefaults]("app", WithEnv(map[string]string{"APP_VALUES": `["input"]`}))
	if err != nil || len(loaded.Values) != 1 || loaded.Values[0].Value != "input" {
		t.Fatal(loaded, err)
	}

	variables, err := Manifest[jsonReadOnlyDefaults]("app")
	problems := ConfigErrors(err)
	if variables != nil || len(problems) != 1 || problems[0].Kind != ErrorDefaultRender || problems[0].VariableName != "APP_VALUES" {
		t.Fatal(variables, err)
	}
}
