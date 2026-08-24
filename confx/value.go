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

// setField assigns raw to target, which is one field of a config. A type that
// declares its own text form is decoded through it, whatever its kind;
// otherwise a slice or a map is split with the given separators and a single
// value is parsed by its type.
func setField(target reflect.Value, raw, separator, keyValSeparator string) error {
	// A named slice or map that unmarshals itself owns its whole syntax, so this
	// comes before any splitting.
	if unmarshaler, ok := textUnmarshaler(target); ok {
		return unmarshaler.UnmarshalText([]byte(raw))
	}

	switch target.Kind() {
	case reflect.Slice:
		return setSlice(target, raw, separator)
	case reflect.Map:
		return setMap(target, raw, separator, keyValSeparator)
	default:
		return setScalar(target, raw)
	}
}

// setSlice fills target with the elements of raw, split on separator. An empty
// raw yields an empty slice rather than a nil one: the variable was set, so the
// field is assigned.
func setSlice(target reflect.Value, raw, separator string) error {
	if target.Type().Elem().Kind() == reflect.Uint8 {
		return errors.New("a byte slice has no unambiguous text form; use a string")
	}

	parts := splitNonEmpty(raw, separator)

	slice := reflect.MakeSlice(target.Type(), len(parts), len(parts))
	for i, part := range parts {
		if err := setScalar(slice.Index(i), part); err != nil {
			return fmt.Errorf("element %d: %w", i, err)
		}
	}

	target.Set(slice)

	return nil
}

// setMap fills target from entries of the form "key<keyValSeparator>value",
// themselves split on separator. A duplicate key is an error rather than a
// silent overwrite, and surrounding whitespace is rejected rather than trimmed,
// so "a: 1" reports itself instead of yielding the value " 1".
func setMap(target reflect.Value, raw, separator, keyValSeparator string) error {
	mapType := target.Type()
	result := reflect.MakeMap(mapType)

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

		key := reflect.New(mapType.Key()).Elem()
		if err := setScalar(key, rawKey); err != nil {
			return fmt.Errorf("key %q: %w", rawKey, err)
		}

		if result.MapIndex(key).IsValid() {
			return fmt.Errorf("key %q appears more than once", rawKey)
		}

		value := reflect.New(mapType.Elem()).Elem()
		if err := setScalar(value, rawValue); err != nil {
			return fmt.Errorf("key %q: %w", rawKey, err)
		}

		result.SetMapIndex(key, value)
	}

	target.Set(result)

	return nil
}

// setScalar assigns one parsed value to target: a pointer is allocated and
// filled, a type with its own text form decodes itself, and everything else is
// parsed by kind.
func setScalar(target reflect.Value, raw string) error {
	if unmarshaler, ok := textUnmarshaler(target); ok {
		return unmarshaler.UnmarshalText([]byte(raw))
	}

	if target.Kind() == reflect.Pointer {
		pointer := reflect.New(target.Type().Elem())
		if err := setScalar(pointer.Elem(), raw); err != nil {
			return err
		}

		target.Set(pointer)

		return nil
	}

	// Duration is an int64 underneath, so it has to be recognised by type before
	// the integer case turns "30s" into a parse error.
	if target.Type() == durationType {
		return setDuration(target, raw)
	}

	switch target.Kind() {
	case reflect.String:
		target.SetString(raw)
	case reflect.Bool:
		parsed, err := strconv.ParseBool(raw)
		if err != nil {
			return fmt.Errorf("%q is not a boolean", raw)
		}

		target.SetBool(parsed)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		parsed, err := strconv.ParseInt(raw, 10, target.Type().Bits())
		if err != nil {
			return fmt.Errorf("%q is not %s", raw, target.Type())
		}

		target.SetInt(parsed)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		parsed, err := strconv.ParseUint(raw, 10, target.Type().Bits())
		if err != nil {
			return fmt.Errorf("%q is not %s", raw, target.Type())
		}

		target.SetUint(parsed)
	case reflect.Float32, reflect.Float64:
		parsed, err := strconv.ParseFloat(raw, target.Type().Bits())
		if err != nil {
			return fmt.Errorf("%q is not %s", raw, target.Type())
		}

		target.SetFloat(parsed)
	default:
		return fmt.Errorf("%s is not a configurable type", target.Type())
	}

	return nil
}

func setDuration(target reflect.Value, raw string) error {
	parsed, err := time.ParseDuration(raw)
	if err != nil {
		return fmt.Errorf("%q is not a duration such as \"30s\" or \"5m\"", raw)
	}

	target.SetInt(int64(parsed))

	return nil
}

// textUnmarshaler returns target's own text decoder when it declares one. The
// method set of *T is checked, not T's, because UnmarshalText has to take a
// pointer to have anything to assign to.
func textUnmarshaler(target reflect.Value) (encoding.TextUnmarshaler, bool) {
	if !target.CanAddr() || !reflect.PointerTo(target.Type()).Implements(textUnmarshalerType) {
		return nil, false
	}

	unmarshaler, ok := target.Addr().Interface().(encoding.TextUnmarshaler)

	return unmarshaler, ok
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
// carry. A type that declares its own text form decides how it appears - which
// is how a secret renders as its mask - and a slice or map is joined with the
// separators its field declares, so the result can be pasted back into the
// environment it came from.
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
