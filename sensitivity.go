package confmaker

import "reflect"

var secretValueType = reflect.TypeFor[SensitiveValue]()

// SensitiveValue marks a value that must not be disclosed by configuration tooling.
// The marker is inspected by type and is never called.
type SensitiveValue interface{ IsSensitive() }

func isSecretType(t reflect.Type) bool { return isSensitiveType(t, make(map[reflect.Type]bool)) }
func isSensitiveType(t reflect.Type, seen map[reflect.Type]bool) bool {
	if seen[t] {
		return false
	}
	seen[t] = true
	if t.Implements(secretValueType) || reflect.PointerTo(t).Implements(secretValueType) {
		return true
	}

	if isTextForm(t) {
		return false
	}
	switch t.Kind() {
	case reflect.Pointer, reflect.Slice, reflect.Array:
		return isSensitiveType(t.Elem(), seen)
	case reflect.Map:
		return isSensitiveType(t.Key(), seen) || isSensitiveType(t.Elem(), seen)
	}

	return false
}
