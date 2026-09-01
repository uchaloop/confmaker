package confx

import (
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/uchaloop/secret/v2"
)

// parseInto assigns raw to a fresh T and returns it, so a case reads as
// "this text becomes this value". A type that cannot be read at all is reported
// by the same call, because the parser is chosen from the type before any value
// is seen.
func parseInto[T any](t *testing.T, raw string) (T, error) {
	t.Helper()

	var target T

	parse, err := fieldParser(reflect.TypeFor[T](), defaultSeparator, defaultKeyValSeparator)
	if err != nil {
		return target, err
	}

	err = parse(reflect.ValueOf(&target).Elem(), raw)

	return target, err
}

func TestParseScalars(t *testing.T) {
	t.Run("string", func(t *testing.T) {
		got, err := parseInto[string](t, " keeps spaces ")
		if err != nil || got != " keeps spaces " {
			t.Fatalf("got %q, err %v", got, err)
		}
	})

	t.Run("bool", func(t *testing.T) {
		for _, raw := range []string{"true", "1", "T", "FALSE"} {
			if _, err := parseInto[bool](t, raw); err != nil {
				t.Errorf("%q rejected: %v", raw, err)
			}
		}
		if _, err := parseInto[bool](t, "yes"); err == nil {
			t.Error(`"yes" accepted as a boolean`)
		}
	})

	t.Run("int", func(t *testing.T) {
		got, err := parseInto[int](t, "-42")
		if err != nil || got != -42 {
			t.Fatalf("got %d, err %v", got, err)
		}
	})

	t.Run("float", func(t *testing.T) {
		got, err := parseInto[float64](t, "2.5")
		if err != nil || got != 2.5 {
			t.Fatalf("got %v, err %v", got, err)
		}
	})

	t.Run("duration", func(t *testing.T) {
		// Duration is an int64 underneath: parsed by kind, "30s" would fail.
		got, err := parseInto[time.Duration](t, "30s")
		if err != nil || got != 30*time.Second {
			t.Fatalf("got %v, err %v", got, err)
		}
		if _, err := parseInto[time.Duration](t, "30"); err == nil {
			t.Error(`"30" accepted as a duration`)
		}
	})

	t.Run("pointer", func(t *testing.T) {
		got, err := parseInto[*bool](t, "true")
		if err != nil || got == nil || !*got {
			t.Fatalf("got %v, err %v", got, err)
		}
	})
}

func TestParseRejectsOutOfRange(t *testing.T) {
	cases := map[string]func() error{
		"int8 overflow":  func() error { _, err := parseInto[int8](t, "200"); return err },
		"uint negative":  func() error { _, err := parseInto[uint](t, "-1"); return err },
		"int32 overflow": func() error { _, err := parseInto[int32](t, "3000000000"); return err },
		"not a number":   func() error { _, err := parseInto[int](t, "12a"); return err },
		"empty int":      func() error { _, err := parseInto[int](t, ""); return err },
	}

	for name, run := range cases {
		if err := run(); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestParseUsesTextUnmarshaler(t *testing.T) {
	got, err := parseInto[secret.Secret](t, "s3cr3t")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got.Reveal() != "s3cr3t" {
		t.Fatal("the secret was not decoded through its own text form")
	}

	// time.Time reaches the same path from the standard library.
	moment, err := parseInto[time.Time](t, "2026-08-24T10:00:00Z")
	if err != nil || moment.Year() != 2026 {
		t.Fatalf("got %v, err %v", moment, err)
	}
}

func TestParseSlices(t *testing.T) {
	got, err := parseInto[[]string](t, "a,b,c")
	if err != nil || len(got) != 3 || got[2] != "c" {
		t.Fatalf("got %v, err %v", got, err)
	}

	durations, err := parseInto[[]time.Duration](t, "1s,2m")
	if err != nil || len(durations) != 2 || durations[1] != 2*time.Minute {
		t.Fatalf("got %v, err %v", durations, err)
	}

	// A set variable assigns the field, so an empty value is an empty slice - not
	// a slice holding one empty string.
	empty, err := parseInto[[]string](t, "")
	if err != nil || empty == nil || len(empty) != 0 {
		t.Fatalf("got %#v, err %v", empty, err)
	}

	if _, err := parseInto[[]int](t, "1,x"); err == nil {
		t.Error("a bad element was accepted")
	}
}

func TestParseRejectsByteSlice(t *testing.T) {
	_, err := parseInto[[]byte](t, "abc")
	if err == nil {
		t.Fatal("a byte slice was accepted; its text form is ambiguous")
	}
	if !strings.Contains(err.Error(), "string") {
		t.Fatalf("the error does not point at the alternative: %v", err)
	}
}

func TestParseMaps(t *testing.T) {
	got, err := parseInto[map[string]string](t, "env:prod,team:core")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got["env"] != "prod" || got["team"] != "core" {
		t.Fatalf("got %v", got)
	}

	typed, err := parseInto[map[string]time.Duration](t, "fast:1s,slow:1m")
	if err != nil || typed["slow"] != time.Minute {
		t.Fatalf("got %v, err %v", typed, err)
	}

	empty, err := parseInto[map[string]string](t, "")
	if err != nil || empty == nil || len(empty) != 0 {
		t.Fatalf("got %#v, err %v", empty, err)
	}
}

func TestParseMapRejections(t *testing.T) {
	cases := map[string]string{
		"no separator":     "env",
		"duplicate key":    "env:prod,env:dev",
		"padded key":       "env :prod",
		"padded value":     "env: prod",
		"bad value type":   "count:x",
		"padded both ends": " env:prod ",
	}

	for name, raw := range cases {
		t.Run(name, func(t *testing.T) {
			var err error
			if name == "bad value type" {
				_, err = parseInto[map[string]int](t, raw)
			} else {
				_, err = parseInto[map[string]string](t, raw)
			}
			if err == nil {
				t.Fatalf("%q was accepted", raw)
			}
		})
	}
}

func TestParseRejectsUnsupportedKind(t *testing.T) {
	if _, err := parseInto[complex128](t, "1+2i"); err == nil {
		t.Error("a complex number was accepted")
	}
	if _, err := parseInto[uintptr](t, "1"); err == nil {
		t.Error("a uintptr was accepted")
	}
	if _, err := parseInto[struct{ A int }](t, "x"); err == nil {
		t.Error("a bare struct was accepted as a value")
	}
}

// pointerRendered has its text form on the value, the way time.Time does, so
// the promoted pointer method dereferences its receiver.
type pointerRendered struct{ n int }

func (p pointerRendered) MarshalText() ([]byte, error) { return []byte(strconv.Itoa(p.n)), nil }

func (p *pointerRendered) UnmarshalText(text []byte) error {
	n, err := strconv.Atoi(string(text))
	p.n = n

	return err
}

// pointerMarshaled declares both halves of its text form on the pointer, so
// rendering has to look in the same method set parsing does.
type pointerMarshaled struct{ n int }

func (p *pointerMarshaled) MarshalText() ([]byte, error) { return []byte(strconv.Itoa(p.n)), nil }

func (p *pointerMarshaled) UnmarshalText(text []byte) error {
	n, err := strconv.Atoi(string(text))
	p.n = n

	return err
}

func TestRenderNilPointer(t *testing.T) {
	// A nil pointer renders as nothing rather than panicking: the text form is
	// looked for only once the pointer is known to point at something.
	for name, value := range map[string]any{
		"time":            (*time.Time)(nil),
		"declared text":   (*pointerRendered)(nil),
		"plain":           (*int)(nil),
		"nested pointers": (**int)(nil),
	} {
		t.Run(name, func(t *testing.T) {
			got := renderValue(reflect.ValueOf(value), defaultSeparator, defaultKeyValSeparator)
			if len(got) != 0 {
				t.Fatalf("nil %T rendered as %q", value, got)
			}
		})
	}
}

func TestRenderPointerReceiverText(t *testing.T) {
	// The marshaller is declared on the pointer, the same method set the parser
	// is chosen from, so the value renders through it and not through fmt.
	value := pointerMarshaled{n: 7}

	got := renderValue(reflect.ValueOf(&value).Elem(), defaultSeparator, defaultKeyValSeparator)
	if got != "7" {
		t.Fatalf("got %q, want %q", got, "7")
	}
}

// countedByte is byte-sized but a type of its own, and reads itself from text.
type countedByte uint8

func (c *countedByte) UnmarshalText(text []byte) error {
	*c = countedByte(len(text))

	return nil
}

func TestSliceOfNamedByteType(t *testing.T) {
	t.Run("declares its own text form", func(t *testing.T) {
		got, err := parseInto[[]countedByte](t, "ab,cde")
		if err != nil {
			t.Fatalf("rejected: %v", err)
		}
		if len(got) != 2 || got[0] != 2 || got[1] != 3 {
			t.Fatalf("got %v", got)
		}
	})

	t.Run("reads as a number", func(t *testing.T) {
		type level uint8

		got, err := parseInto[[]level](t, "1,2")
		if err != nil {
			t.Fatalf("rejected: %v", err)
		}
		if len(got) != 2 || got[0] != 1 || got[1] != 2 {
			t.Fatalf("got %v", got)
		}
	})

	t.Run("a byte slice is still refused", func(t *testing.T) {
		if _, err := parseInto[[]byte](t, "ab"); err == nil {
			t.Fatal("[]byte accepted")
		}
	})
}
