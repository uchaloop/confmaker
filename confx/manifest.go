package confx

import (
	"reflect"
	"strings"

	"github.com/uchaloop/secret/v2"
)

// secretValueType is the marker interface a secret type implements.
var secretValueType = reflect.TypeOf((*secret.Value)(nil)).Elem()

// Variable describes one environment variable a config type reads. Provide
// derives the set of them from T's tags, which is what makes the strict check
// and the dump possible without a configuration file.
type Variable struct {
	// Name is the full variable name, prefix included.
	Name string
	// Type is the Go type of the field it fills.
	Type string
	// Required reports whether the field is declared required or notEmpty.
	Required bool
	// Secret reports whether the field holds a secret value.
	Secret bool
	// Default is the envDefault declared on the field, if any.
	Default string
	// HasDefault distinguishes an absent envDefault from an empty one.
	HasDefault bool
}

// descriptor is the manifest one Provide call registers: the instance it builds
// and every variable that instance reads.
type descriptor struct {
	label     string
	prefix    string
	variables []Variable
	// open reports that the config reads variables no type walk can enumerate -
	// a collection of structs, which the env parser numbers per element
	// (PREFIX_0_FIELD). The strict check leaves such a prefix alone rather than
	// report those variables as unknown.
	open bool
}

// Manifest returns the environment variables a T built under name reads, in the
// order the type declares them. It is the same manifest Module checks and
// WithDump prints, resolved without building an application - generate a
// .env.example, a deployment's ConfigMap, or a documentation table from it.
//
// The options are the ones Provide takes, so a manifest taken with WithPrefix
// matches the instance provided with the same option.
//
// Two kinds of field contribute nothing, because the parser reads nothing from
// them either: a nested slice of structs, whose variables the parser numbers per
// element (PREFIX_0_FIELD) rather than by type, and a pointer to a struct
// without the init option, which stays nil in a config built from its zero
// value.
func Manifest[T any](name string, opts ...Option) []Variable {
	variables, _ := manifest(reflect.TypeFor[T](), prefixFor(name, opts))

	return variables
}

// manifest walks configType and returns the variables it reads under prefix. It
// mirrors how the env parser traverses a struct: a field with an env tag is a
// leaf, an unexported field is skipped, and a struct field without an env tag is
// descended into, extending the prefix with its envPrefix tag.
func manifest(configType reflect.Type, prefix string) (variables []Variable, open bool) {
	// A T the parser cannot read at all - a pointer, a map, anything but a struct
	// - yields no variables. Reporting an empty manifest would make the strict
	// check call every variable under the prefix unknown and blame the
	// environment for what is a mistake in the code, so the prefix is left alone
	// and the constructor gets to report the real problem.
	if configType.Kind() != reflect.Struct {
		return nil, true
	}

	walk := walker{seen: make(map[reflect.Type]bool)}
	walk.walk(configType, prefix)

	return walk.variables, walk.open
}

type walker struct {
	variables []Variable
	seen      map[reflect.Type]bool
	open      bool
}

func (w *walker) walk(configType reflect.Type, prefix string) {
	if configType.Kind() != reflect.Struct {
		return
	}
	// A config reachable from itself through an allocated pointer would
	// otherwise recurse forever.
	if w.seen[configType] {
		return
	}
	w.seen[configType] = true

	defer delete(w.seen, configType)

	for i := range configType.NumField() {
		w.field(configType.Field(i), prefix)
	}
}

// field records what the env parser would read from one struct field. It follows
// the parser's own traversal, because a manifest that claims a variable the
// parser ignores would make the strict check accept a name that has no effect.
func (w *walker) field(field reflect.StructField, prefix string) {
	// The parser sets a field only when it is settable, which excludes every
	// unexported field - an embedded unexported type included.
	if len(field.PkgPath) != 0 {
		return
	}

	name, options, _ := strings.Cut(field.Tag.Get("env"), ",")
	// `env:"-"` takes the field out of the parse entirely, nested fields included.
	if name == "-" {
		return
	}

	if len(name) != 0 {
		w.variables = append(w.variables, describeField(field, name, options, prefix))
	}

	nested := prefix + field.Tag.Get("envPrefix")
	fieldType := field.Type

	switch {
	case fieldType.Kind() == reflect.Struct:
		// The parser descends into a struct field even when it has just set that
		// field from a variable of its own.
		w.walk(fieldType, nested)
	case isPointerToStruct(fieldType) && hasOption(options, "init"):
		// Every config starts from its zero value, so a pointer field is nil and
		// the parser steps over it. Only the init option makes it allocate the
		// struct and read the fields inside.
		w.walk(fieldType.Elem(), nested)
	case isSliceOfStructs(fieldType):
		// The parser numbers these per element (PREFIX_0_FIELD), so their names
		// follow how many elements are configured rather than the type.
		w.open = true
	}
}

// describeField builds the Variable for a field the parser fills from name.
func describeField(field reflect.StructField, name, options, prefix string) Variable {
	variable := Variable{
		Name:     prefix + name,
		Type:     field.Type.String(),
		Required: hasOption(options, "required") || hasOption(options, "notEmpty"),
		Secret:   isSecretType(field.Type),
	}
	variable.Default, variable.HasDefault = field.Tag.Lookup("envDefault")

	return variable
}

// hasOption reports whether the comma-separated options of an env tag contain
// option.
func hasOption(options, option string) bool {
	for rest := options; len(rest) != 0; {
		var current string

		current, rest, _ = strings.Cut(rest, ",")
		if current == option {
			return true
		}
	}

	return false
}

func isPointerToStruct(t reflect.Type) bool {
	return t.Kind() == reflect.Pointer && t.Elem().Kind() == reflect.Struct
}

// isSliceOfStructs matches what the env parser numbers per element: a []struct,
// or a pointer to one.
func isSliceOfStructs(t reflect.Type) bool {
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}

	return t.Kind() == reflect.Slice && t.Elem().Kind() == reflect.Struct
}

// isSecretType reports whether a field of type t holds a secret. It is the one
// thing the manifest needs to know about secrets: with configuration read from
// the environment only, nothing else can populate such a field, so the type is
// consulted purely so the dump reports it as set or unset instead of printing
// it.
func isSecretType(t reflect.Type) bool {
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}

	return t.Implements(secretValueType)
}
