package confmaker

import (
	"encoding"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"
)

// These options are immutable and shared by both directions of the field codec.
var jsonFieldOptions = json.JoinOptions(
	json.RejectUnknownMembers(true),
	json.Deterministic(true),
	json.FormatNilSliceAsNull(true),
	json.FormatNilMapAsNull(true),
	json.WithUnmarshalers(json.JoinUnmarshalers(
		json.UnmarshalFromFunc[any](func(dec *jsontext.Decoder, target any) error {
			if dec.PeekKind() == 'n' {
				t := reflect.TypeOf(target).Elem()
				switch t.Kind() {
				case reflect.Pointer, reflect.Slice, reflect.Map:
				default:
					return fmt.Errorf("null is not allowed for %s", t)
				}
			}

			unmarshaler, ok := target.(encoding.TextUnmarshaler)
			if !ok {
				return errors.ErrUnsupported
			}

			if dec.PeekKind() != '"' {
				return fmt.Errorf("a text value must be a JSON string")
			}

			token, err := dec.ReadToken()
			if err != nil {
				return err
			}

			return unmarshaler.UnmarshalText([]byte(token.String()))
		}),
		json.UnmarshalFromFunc[*time.Duration](func(dec *jsontext.Decoder, target *time.Duration) error {
			// A bare number has no unit: 30000 could mean nanoseconds or
			// milliseconds, so only the text form is accepted.
			if dec.PeekKind() != '"' {
				raw, err := dec.ReadValue()
				if err != nil {
					return err
				}

				return fmt.Errorf(`a duration must be a JSON string such as "30s", got %s`, raw)
			}

			// Read the string token straight from the decoder, without decoding
			// the value a second time.
			token, err := dec.ReadToken()
			if err != nil {
				return err
			}

			parsed, err := time.ParseDuration(token.String())
			if err != nil {
				return err
			}

			*target = parsed

			return nil
		}),
	)),
	json.WithMarshalers(json.JoinMarshalers(
		json.MarshalToFunc[any](func(enc *jsontext.Encoder, source any) error {
			value := reflect.ValueOf(source).Elem()
			if !decodesJSONText(value.Type()) {
				return errors.ErrUnsupported
			}

			text, ok, err := textOf(value)
			if err != nil {
				return err
			}

			if !ok {
				return fmt.Errorf("text type %s requires encoding.TextMarshaler to render a JSON default", value.Type())
			}

			return enc.WriteToken(jsontext.String(text))
		}),
		json.MarshalToFunc[time.Duration](func(enc *jsontext.Encoder, value time.Duration) error {
			return enc.WriteToken(jsontext.String(value.String()))
		}),
	)),
)

func jsonParser(t reflect.Type) (parser, error) {
	empty, err := emptyJSONHint(t)
	if err != nil {
		return nil, err
	}

	if err := checkJSONType(t, make(map[reflect.Type]bool)); err != nil {
		return nil, err
	}

	return func(target reflect.Value, raw string) error {
		// An empty variable is a mistake, not an empty value: say how to write one.
		if len(strings.TrimSpace(raw)) == 0 {
			return fmt.Errorf("JSON value is empty; %s", empty)
		}

		// Decode into a fresh value: objects must replace defaults, never merge them.
		next := reflect.New(t)
		if err := json.Unmarshal([]byte(raw), next.Interface(), jsonFieldOptions); err != nil {
			return describeJSONError(err)
		}

		target.Set(next.Elem())

		return nil
	}, nil
}

// emptyJSONHint accepts only the shapes JSON adds something for and returns what
// to write for an empty value of t. A scalar reads plainly without envFormat.
func emptyJSONHint(t reflect.Type) (string, error) {
	if t.Kind() == reflect.Pointer {
		switch t.Elem().Kind() {
		case reflect.Slice, reflect.Array, reflect.Map:
			return "write null for no value", nil
		}
	}

	switch t.Kind() {
	case reflect.Slice:
		return "write [] for an empty list or null for none", nil
	case reflect.Array:
		return "write [] with every element", nil
	case reflect.Map:
		return "write {} for an empty map or null for none", nil
	}

	return "", fmt.Errorf("envFormat json reads a slice, array or map, not %s; drop envFormat to read it as plain text", t)
}

// describeJSONError says where in the value the problem is. The pointer names the
// member or element ("/1/url"); a syntax error adds the byte offset, since the
// text around it may not form a member at all.
func describeJSONError(err error) error {
	var syntactic *jsontext.SyntacticError
	if errors.As(err, &syntactic) {
		return jsonValueError{
			message: fmt.Sprintf("invalid JSON at %s, offset %d: %v", jsonLocation(syntactic.JSONPointer), syntactic.ByteOffset, syntactic.Err),
			err:     err,
		}
	}

	var semantic *json.SemanticError
	if errors.As(err, &semantic) {
		if semantic.Err != nil {
			return jsonValueError{
				message: fmt.Sprintf("JSON at %s: %v", jsonLocation(semantic.JSONPointer), semantic.Err),
				err:     err,
			}
		}

		return jsonValueError{
			message: fmt.Sprintf("JSON at %s: %s cannot be read into %s", jsonLocation(semantic.JSONPointer), jsonKindName(semantic.JSONKind), semantic.GoType),
			err:     err,
		}
	}

	return err
}

// jsonValueError rewords a JSON error for a deployment while keeping the original
// reachable, so errors.As still finds *json.SemanticError or
// *jsontext.SyntacticError with the pointer, offset and Go type.
type jsonValueError struct {
	message string
	err     error
}

func (e jsonValueError) Error() string { return e.message }

func (e jsonValueError) Unwrap() error { return e.err }

func jsonLocation(pointer jsontext.Pointer) string {
	if len(pointer) == 0 {
		return "the top level"
	}

	return fmt.Sprintf("%q", string(pointer))
}

func jsonKindName(kind jsontext.Kind) string {
	switch kind {
	case 'n':
		return "null"
	case 'f', 't':
		return "a boolean"
	case '"':
		return "a string"
	case '0':
		return "a number"
	case '{':
		return "an object"
	case '[':
		return "an array"
	default:
		return "a value"
	}
}

func renderJSON(value reflect.Value) (string, error) {
	// Marshal through a pointer, so pointer-receiver methods are found. A field
	// of a loaded config is addressable; only other values need a copy.
	value = addressableValue(value)

	raw, err := json.Marshal(value.Addr().Interface(), jsonFieldOptions)
	if err != nil {
		return string(raw), err
	}

	return string(raw), nil
}

// Inspect the complete type graph, including ignored and private fields: custom
// marshalers may expose those. A visited set permits recursive config types.
// decodesJSONText identifies collection elements with a scalar text form.
func decodesJSONText(t reflect.Type) bool {
	return reflect.PointerTo(t).Implements(textUnmarshalerType)
}

func checkJSONType(t reflect.Type, seen map[reflect.Type]bool) error {
	if seen[t] {
		return nil
	}

	seen[t] = true
	// Dynamic values would hide their types (including secrets) until loading.
	if t.Kind() == reflect.Interface {
		return fmt.Errorf("JSON requires concrete types, got %s", t)
	}

	if decodesJSONText(t) {
		return nil
	}

	switch t.Kind() {
	case reflect.Slice, reflect.Array:
		// JSON carries bytes as base64 text, which nobody writes by hand.
		if t.Elem().Kind() == reflect.Uint8 && !decodesJSONText(t.Elem()) {
			return fmt.Errorf("JSON would read %s as base64 text; use a string", t)
		}

		return checkJSONType(t.Elem(), seen)
	case reflect.Pointer:
		return checkJSONType(t.Elem(), seen)
	case reflect.Map:
		key := t.Key()
		if key.Kind() != reflect.String && !reflect.PointerTo(key).Implements(textUnmarshalerType) {
			switch key.Kind() {
			case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
				reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			default:
				return fmt.Errorf("JSON map key %s must have a string, integer, or text representation", key)
			}
		}

		if err := checkJSONType(key, seen); err != nil {
			return err
		}

		return checkJSONType(t.Elem(), seen)
	case reflect.Struct:
		return fmt.Errorf("JSON structs are not supported: %s; use nested fields with envPrefix or a text type", t)
	case reflect.String, reflect.Bool,
		reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64:
	default:
		return fmt.Errorf("%s cannot be read from JSON", t)
	}

	return nil
}
