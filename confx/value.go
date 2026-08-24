package confx

import (
	"encoding"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"
)

// defaultSeparator splits the elements of a slice and the entries of a map. The
// envSeparator tag overrides it.
const defaultSeparator = ","

// defaultKeyValSeparator splits one map entry into its key and its value. The
// envKeyValSeparator tag overrides it.
const defaultKeyValSeparator = ":"

var (
	durationType        = reflect.TypeFor[time.Duration]()
	textUnmarshalerType = reflect.TypeFor[encoding.TextUnmarshaler]()
)

// parser assigns the text of one variable to one value.
type parser func(target reflect.Value, raw string) error

// fieldParser returns the parser for a whole config field, or an error saying
// why the field cannot be read from a variable.
//
// It is chosen once, when the config is bound, and the binding keeps it. That is
// what makes a field of an unreadable type fail at startup rather than on the
// first deployment that happens to set its variable, and it leaves one place
// that decides what a type means.
func fieldParser(t reflect.Type, separator, keyValSeparator string) (parser, error) {
	// A named slice or map that decodes itself owns its whole syntax, so this
	// comes before any splitting.
	if declaresTextForm(t) {
		return parseText, nil
	}

	switch t.Kind() {
	case reflect.Slice:
		return sliceParser(t, separator)
	case reflect.Map:
		return mapParser(t, separator, keyValSeparator)
	default:
		return scalarParser(t)
	}
}

// scalarParser returns the parser for a single value.
func scalarParser(t reflect.Type) (parser, error) {
	if declaresTextForm(t) {
		return parseText, nil
	}

	if t.Kind() == reflect.Pointer {
		return pointerParser(t)
	}

	// Duration is an int64 underneath, so it has to be recognised by type before
	// the integer case turns "30s" into a parse error.
	if t == durationType {
		return parseDuration, nil
	}

	switch t.Kind() {
	case reflect.String:
		return parseString, nil
	case reflect.Bool:
		return parseBool, nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return parseInt, nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return parseUint, nil
	case reflect.Float32, reflect.Float64:
		return parseFloat, nil
	default:
		return nil, fmt.Errorf("%s cannot be read from a variable", t)
	}
}

// pointerParser allocates the value behind a pointer and fills it.
func pointerParser(t reflect.Type) (parser, error) {
	parseElement, err := scalarParser(t.Elem())
	if err != nil {
		return nil, err
	}

	return func(target reflect.Value, raw string) error {
		pointer := reflect.New(t.Elem())
		if err := parseElement(pointer.Elem(), raw); err != nil {
			return err
		}

		target.Set(pointer)

		return nil
	}, nil
}

// sliceParser splits a value on separator and parses each element. An empty
// value yields an empty slice rather than a nil one: the variable was set, so
// the field is assigned.
func sliceParser(t reflect.Type, separator string) (parser, error) {
	if t.Elem().Kind() == reflect.Uint8 {
		return nil, errors.New("a byte slice has no unambiguous text form; use a string")
	}
	// Splitting on nothing yields one element per character.
	if len(separator) == 0 {
		return nil, errors.New("envSeparator is empty; a slice needs something to split on")
	}

	parseElement, err := scalarParser(t.Elem())
	if err != nil {
		return nil, err
	}

	return func(target reflect.Value, raw string) error {
		parts := splitNonEmpty(raw, separator)

		slice := reflect.MakeSlice(t, len(parts), len(parts))
		for i, part := range parts {
			if err := parseElement(slice.Index(i), part); err != nil {
				return fmt.Errorf("element %d: %w", i, err)
			}
		}

		target.Set(slice)

		return nil
	}, nil
}

// mapParser reads entries of the form "key<keyValSeparator>value", themselves
// split on separator. A duplicate key is an error rather than a silent
// overwrite, and surrounding whitespace is rejected rather than trimmed, so
// "a: 1" reports itself instead of yielding the value " 1".
func mapParser(t reflect.Type, separator, keyValSeparator string) (parser, error) {
	if len(separator) == 0 {
		return nil, errors.New("envSeparator is empty; a map needs something to split entries on")
	}
	if len(keyValSeparator) == 0 {
		return nil, errors.New("envKeyValSeparator is empty; a map needs something to split a key from its value")
	}

	parseKey, err := scalarParser(t.Key())
	if err != nil {
		return nil, fmt.Errorf("map key: %w", err)
	}

	parseValue, err := scalarParser(t.Elem())
	if err != nil {
		return nil, fmt.Errorf("map value: %w", err)
	}

	return func(target reflect.Value, raw string) error {
		result := reflect.MakeMap(t)

		for _, entry := range splitNonEmpty(raw, separator) {
			rawKey, rawValue, found := strings.Cut(entry, keyValSeparator)
			if !found {
				return fmt.Errorf("entry %q has no %q separating key from value", entry, keyValSeparator)
			}
			if err := checkUntrimmed(rawKey, "key"); err != nil {
				return err
			}
			if err := checkUntrimmed(rawValue, "value"); err != nil {
				return err
			}

			key := reflect.New(t.Key()).Elem()
			if err := parseKey(key, rawKey); err != nil {
				return fmt.Errorf("key %q: %w", rawKey, err)
			}

			if result.MapIndex(key).IsValid() {
				return fmt.Errorf("key %q appears more than once", rawKey)
			}

			value := reflect.New(t.Elem()).Elem()
			if err := parseValue(value, rawValue); err != nil {
				return fmt.Errorf("key %q: %w", rawKey, err)
			}

			result.SetMapIndex(key, value)
		}

		target.Set(result)

		return nil
	}, nil
}

func parseText(target reflect.Value, raw string) error {
	//nolint:forcetypeassert // declaresTextForm established the method set.
	return target.Addr().Interface().(encoding.TextUnmarshaler).UnmarshalText([]byte(raw))
}

func parseString(target reflect.Value, raw string) error {
	target.SetString(raw)

	return nil
}

func parseBool(target reflect.Value, raw string) error {
	parsed, err := strconv.ParseBool(raw)
	if err != nil {
		return fmt.Errorf("%q is not a boolean", raw)
	}

	target.SetBool(parsed)

	return nil
}

func parseInt(target reflect.Value, raw string) error {
	parsed, err := strconv.ParseInt(raw, 10, target.Type().Bits())
	if err != nil {
		return fmt.Errorf("%q is not %s", raw, target.Type())
	}

	target.SetInt(parsed)

	return nil
}

func parseUint(target reflect.Value, raw string) error {
	parsed, err := strconv.ParseUint(raw, 10, target.Type().Bits())
	if err != nil {
		return fmt.Errorf("%q is not %s", raw, target.Type())
	}

	target.SetUint(parsed)

	return nil
}

func parseFloat(target reflect.Value, raw string) error {
	parsed, err := strconv.ParseFloat(raw, target.Type().Bits())
	if err != nil {
		return fmt.Errorf("%q is not %s", raw, target.Type())
	}

	target.SetFloat(parsed)

	return nil
}

func parseDuration(target reflect.Value, raw string) error {
	parsed, err := time.ParseDuration(raw)
	if err != nil {
		return fmt.Errorf("%q is not a duration such as \"30s\" or \"5m\"", raw)
	}

	target.SetInt(int64(parsed))

	return nil
}

// declaresTextForm reports whether a value of type t decodes itself from text.
// The method set of *T is checked, not T's, because UnmarshalText has to take a
// pointer to have anything to assign to.
func declaresTextForm(t reflect.Type) bool {
	return reflect.PointerTo(t).Implements(textUnmarshalerType)
}

// splitNonEmpty splits raw on separator, treating an empty raw as no elements at
// all rather than as one empty element.
func splitNonEmpty(raw, separator string) []string {
	if len(raw) == 0 {
		return nil
	}

	return strings.Split(raw, separator)
}

// checkUntrimmed rejects a map key or value padded with whitespace. Trimming it
// away would silently accept "a: 1" as the value " 1"; reporting it lets the
// deployment fix what it meant to write.
func checkUntrimmed(part, kind string) error {
	if part != strings.TrimSpace(part) {
		return fmt.Errorf("%s %q is padded with whitespace", kind, part)
	}

	return nil
}

// renderValue turns a field's current value back into the text a variable would
// carry. A type that declares its own text form decides how it appears, and a
// slice or map is joined with the separators its field declares, so the result
// can be pasted back into the environment it came from.
func renderValue(value reflect.Value, separator, keyValSeparator string) string {
	if marshaler, ok := value.Interface().(encoding.TextMarshaler); ok {
		text, err := marshaler.MarshalText()
		if err != nil {
			return ""
		}

		return string(text)
	}
	if stringer, ok := value.Interface().(fmt.Stringer); ok {
		return stringer.String()
	}

	switch value.Kind() {
	case reflect.Pointer:
		if value.IsNil() {
			return ""
		}

		return renderValue(value.Elem(), separator, keyValSeparator)
	case reflect.Slice:
		parts := make([]string, value.Len())
		for i := range parts {
			parts[i] = renderValue(value.Index(i), separator, keyValSeparator)
		}

		return strings.Join(parts, separator)
	case reflect.Map:
		return renderMap(value, separator, keyValSeparator)
	default:
		return fmt.Sprint(value.Interface())
	}
}

// renderMap joins a map's entries in key order, so the same map always renders
// the same way.
func renderMap(value reflect.Value, separator, keyValSeparator string) string {
	parts := make([]string, 0, value.Len())

	iter := value.MapRange()
	for iter.Next() {
		parts = append(parts, renderValue(iter.Key(), separator, keyValSeparator)+
			keyValSeparator+
			renderValue(iter.Value(), separator, keyValSeparator))
	}

	sort.Strings(parts)

	return strings.Join(parts, separator)
}
