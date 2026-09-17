package confmaker

import (
	"math"
	"reflect"
	"testing"
	"time"
)

// plainFuzzCodec is one field shape the plain (non-JSON) syntax reads.
type plainFuzzCodec struct {
	name  string
	field reflect.StructField
	parse parser
	emit  renderer
}

func plainFuzzCodecs(tb testing.TB) []plainFuzzCodec {
	tb.Helper()

	fields := []reflect.StructField{
		{Name: "Text", Type: reflect.TypeFor[string]()},
		{Name: "Flag", Type: reflect.TypeFor[bool]()},
		{Name: "Int8", Type: reflect.TypeFor[int8]()},
		{Name: "Int64", Type: reflect.TypeFor[int64]()},
		{Name: "Uint16", Type: reflect.TypeFor[uint16]()},
		{Name: "Float32", Type: reflect.TypeFor[float32]()},
		{Name: "Float64", Type: reflect.TypeFor[float64]()},
		{Name: "Duration", Type: reflect.TypeFor[time.Duration]()},
		{Name: "Pointer", Type: reflect.TypeFor[*int]()},
		{Name: "Strings", Type: reflect.TypeFor[[]string]()},
		{Name: "Durations", Type: reflect.TypeFor[[]time.Duration]()},
		{Name: "Semicolons", Type: reflect.TypeFor[[]string](), Tag: `envSeparator:";;"`},
		{Name: "Map", Type: reflect.TypeFor[map[string]int]()},
		{Name: "IntKeys", Type: reflect.TypeFor[map[int]string]()},
		{Name: "Pairs", Type: reflect.TypeFor[map[string]string](), Tag: `envSeparator:"&" envKeyValSeparator:"="`},
		{Name: "Custom", Type: reflect.TypeFor[money]()},
	}

	codecs := make([]plainFuzzCodec, 0, len(fields))
	for _, field := range fields {
		parse, emit, err := fieldCodec(field)
		if err != nil {
			tb.Fatalf("%s: %v", field.Name, err)
		}

		codecs = append(codecs, plainFuzzCodec{name: field.Name, field: field, parse: parse, emit: emit})
	}

	return codecs
}

// FuzzPlainField feeds one input to every plain field shape. Nothing may panic,
// and whatever loads must render to text that loads back to the same value - the
// promise a manifest makes about its defaults.
func FuzzPlainField(f *testing.F) {
	for _, seed := range []string{
		"", " ", "0", "-1", "255", "1e3", "NaN", "-Inf", "0x10", "true", "1",
		"30s", "-1h2m3.5s", "a,b", "a,,b", ",", "a;;b;;;c", "k:1,j:2", "k:1:2",
		"a=b&c=d", "k: 1", "12.50", "\x00", "é,ü",
	} {
		f.Add(seed)
	}

	codecs := plainFuzzCodecs(f)

	f.Fuzz(func(t *testing.T, raw string) {
		for _, codec := range codecs {
			first := reflect.New(codec.field.Type).Elem()
			if err := codec.parse(first, raw); err != nil {
				continue
			}

			rendered, err := codec.emit(first)
			if err != nil {
				t.Fatalf("%s: %q loaded but does not render: %v", codec.name, raw, err)
			}

			second := reflect.New(codec.field.Type).Elem()
			if err := codec.parse(second, rendered); err != nil {
				t.Fatalf("%s: %q rendered as %q, which does not load: %v", codec.name, raw, rendered, err)
			}

			if !sameValue(first, second) {
				t.Fatalf("%s: %q -> %q changed the value: %#v != %#v", codec.name, raw, rendered, first, second)
			}
		}
	})
}

// sameValue is DeepEqual with NaN equal to itself: "NaN" is a value a float
// field accepts, and it has to survive a round trip like any other.
func sameValue(a, b reflect.Value) bool {
	if a.Kind() == reflect.Float32 || a.Kind() == reflect.Float64 {
		x, y := a.Float(), b.Float()
		return x == y || (math.IsNaN(x) && math.IsNaN(y))
	}

	return reflect.DeepEqual(a.Interface(), b.Interface())
}
