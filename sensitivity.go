package confmaker

import "reflect"

// SensitiveValue marks a value that must not be disclosed by configuration tooling.
// The marker is inspected by type and is never called.
var secretValueType = reflect.TypeFor[SensitiveValue]()

type SensitiveValue interface{ IsSensitive() }

func isSecretType(t reflect.Type) bool { return sensitiveType(t, make(map[reflect.Type]bool)) }
func sensitiveType(t reflect.Type, seen map[reflect.Type]bool) bool {
	if seen[t] {
		return false
	}
	seen[t] = true
	if t.Implements(secretValueType) || reflect.PointerTo(t).Implements(secretValueType) {
		return true
	}
	if declaresTextForm(t) {
		return false
	}
	switch t.Kind() {
	case reflect.Pointer, reflect.Slice, reflect.Array:
		return sensitiveType(t.Elem(), seen)
	case reflect.Map:
		return sensitiveType(t.Key(), seen) || sensitiveType(t.Elem(), seen)
	}
	return false
}
