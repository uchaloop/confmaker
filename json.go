package confmaker

import (
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

			// Leave decoding to the duration adapter or the native typed decoder.
			return errors.ErrUnsupported
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
	json.WithMarshalers(json.MarshalToFunc[time.Duration](func(enc *jsontext.Encoder, value time.Duration) error {
		return enc.WriteToken(jsontext.String(value.String()))
	})),
)

var (
	jsonUnmarshalerType     = reflect.TypeFor[json.Unmarshaler]()
	jsonUnmarshalerFromType = reflect.TypeFor[json.UnmarshalerFrom]()
)

func jsonParser(t reflect.Type) (parser, error) {
	empty, err := emptyJSONHint(t)
	if err != nil {
		return nil, err
	}

	if err := checkJSONSecrets(t, make(map[reflect.Type]bool)); err != nil {
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
		case reflect.Struct, reflect.Slice, reflect.Array, reflect.Map:
			return "write null for no value", nil
		}
	}

	switch t.Kind() {
	case reflect.Struct:
		return "write {} for an object with zero fields", nil
	case reflect.Slice:
		return "write [] for an empty list or null for none", nil
	case reflect.Array:
		return "write [] with every element", nil
	case reflect.Map:
		return "write {} for an empty map or null for none", nil
	}

	return "", fmt.Errorf("envFormat json reads a struct, slice, array or map, not %s; drop envFormat to read it as plain text", t)
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
	if !value.CanAddr() {
		copy := reflect.New(value.Type()).Elem()
		copy.Set(value)
		value = copy
	}

	raw, err := json.Marshal(value.Addr().Interface(), jsonFieldOptions)

	return string(raw), err
}

// Inspect the complete type graph, including ignored and private fields: custom
// marshalers may expose those. A visited set permits recursive config types.
func checkJSONSecrets(t reflect.Type, seen map[reflect.Type]bool) error {
	if seen[t] {
		return nil
	}

	seen[t] = true
	if t.Implements(secretValueType) || reflect.PointerTo(t).Implements(secretValueType) {
		return fmt.Errorf("JSON contains secret type %s; give each secret its own variable", t)
	}

	switch t.Kind() {
	case reflect.Pointer, reflect.Slice, reflect.Array:
		return checkJSONSecrets(t.Elem(), seen)
	case reflect.Map:
		if err := checkJSONSecrets(t.Key(), seen); err != nil {
			return err
		}

		return checkJSONSecrets(t.Elem(), seen)
	case reflect.Struct:
		for field := range t.Fields() {
			if err := checkJSONSecrets(field.Type, seen); err != nil {
				return fmt.Errorf("%s: %w", field.Name, err)
			}
		}
	}

	return nil
}

// decodesItselfFromJSON reports a type that owns its JSON form. Its fields are
// its own business, so they are not inspected; secrets inside it still are.
func decodesItselfFromJSON(t reflect.Type) bool {
	p := reflect.PointerTo(t)
	return p.Implements(jsonUnmarshalerType) || p.Implements(jsonUnmarshalerFromType) || p.Implements(textUnmarshalerType)
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

	if decodesItselfFromJSON(t) {
		return nil
	}

	switch t.Kind() {
	case reflect.Slice, reflect.Array:
		// JSON carries bytes as base64 text, which nobody writes by hand.
		if t.Elem().Kind() == reflect.Uint8 && !decodesItselfFromJSON(t.Elem()) {
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
		for field := range t.Fields() {
			// The rule json/v2 applies: only a tag of exactly "-" ignores a
			// field; "-," and "-,omitempty" name it "-".
			if len(field.PkgPath) != 0 || field.Tag.Get("json") == "-" {
				continue
			}

			if err := checkJSONType(field.Type, seen); err != nil {
				return fmt.Errorf("%s: %w", field.Name, err)
			}
		}

		// Let json/v2 judge the object form itself - names that conflict, inline
		// fields, tag options - at registration rather than when a variable is
		// first set. Decoding {} builds the struct's fields and runs no user
		// code: a type that decodes itself does not reach this case, and no
		// member is decoded.
		if err := json.Unmarshal([]byte("{}"), reflect.New(t).Interface(), jsonFieldOptions); err != nil {
			var semantic *json.SemanticError
			if errors.As(err, &semantic) && semantic.Err != nil {
				err = semantic.Err
			}

			return fmt.Errorf("%s has no valid JSON object form: %w", t, err)
		}
	case reflect.String, reflect.Bool,
		reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64:
	default:
		return fmt.Errorf("%s cannot be read from JSON", t)
	}

	return nil
}
