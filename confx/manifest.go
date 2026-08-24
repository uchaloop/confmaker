package confx

import (
	"reflect"
	"strings"

	"github.com/uchaloop/secret/v2"
)

// secretValueType is the marker interface a secret type implements. A field of
// such a type is never printed, only reported as set or unset.
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
// A nested collection of structs contributes no variables: the env parser
// numbers those per element (PREFIX_0_FIELD), so their names follow how many
// elements are configured rather than T.
func Manifest[T any](name string, opts ...Option) []Variable {
	prefix, _ := resolve(name, opts)

	return describe(reflect.TypeFor[T](), prefix)
}

// manifest walks configType and returns the variables it reads under prefix. It
// mirrors how the env parser traverses a struct: a field with an env tag is a
// leaf, an unexported field is skipped, and a struct field without an env tag is
// descended into, extending the prefix with its envPrefix tag.
func manifest(configType reflect.Type, prefix string) (variables []Variable, open bool) {
	walk := walker{seen: make(map[reflect.Type]bool)}
	walk.walk(configType, prefix)

	return walk.variables, walk.open
}

// describe is manifest without the open flag, for callers that only need the
// variables.
func describe(configType reflect.Type, prefix string) []Variable {
	variables, _ := manifest(configType, prefix)

	return variables
}

type walker struct {
	variables []Variable
	seen      map[reflect.Type]bool
	open      bool
}

func (w *walker) walk(configType reflect.Type, prefix string) {
	configType = deref(configType)
	if configType.Kind() != reflect.Struct {
		return
	}
	// A config that reaches itself would otherwise recurse forever.
	if w.seen[configType] {
		return
	}
	w.seen[configType] = true

	defer delete(w.seen, configType)

	for i := range configType.NumField() {
		field := configType.Field(i)
		// The env parser sets a field only when it is settable, which excludes
		// every unexported field - an embedded unexported type included.
		if len(field.PkgPath) != 0 {
			continue
		}

		if tag, ok := field.Tag.Lookup("env"); ok {
			if variable, ok := describeField(field, tag, prefix); ok {
				w.variables = append(w.variables, variable)
			}

			continue
		}

		fieldType := deref(field.Type)

		switch {
		case fieldType.Kind() == reflect.Struct && !isSecretType(fieldType):
			w.walk(fieldType, prefix+field.Tag.Get("envPrefix"))
		case isStructCollection(fieldType):
			w.open = true
		}
	}
}

// isStructCollection reports whether t holds structs the env parser would number
// per element. Their variable names depend on how many elements are configured,
// so no type walk can list them.
func isStructCollection(t reflect.Type) bool {
	switch t.Kind() {
	case reflect.Slice, reflect.Array, reflect.Map:
		element := deref(t.Elem())

		return element.Kind() == reflect.Struct && !isSecretType(element)
	default:
		return false
	}
}

// describeField builds the Variable for a leaf field from its env tag. A tag
// carrying only options ("," or ",required") names no variable and is skipped,
// matching the env parser.
func describeField(field reflect.StructField, tag, prefix string) (Variable, bool) {
	name, options, _ := strings.Cut(tag, ",")
	if len(name) == 0 {
		return Variable{}, false
	}

	variable := Variable{
		Name:   prefix + name,
		Type:   field.Type.String(),
		Secret: isSecretType(deref(field.Type)),
	}
	for _, option := range strings.Split(options, ",") {
		if option == "required" || option == "notEmpty" {
			variable.Required = true
		}
	}
	variable.Default, variable.HasDefault = field.Tag.Lookup("envDefault")

	return variable, true
}

func deref(t reflect.Type) reflect.Type {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}

	return t
}

func isSecretType(t reflect.Type) bool {
	return t.Kind() != reflect.Pointer && t.Implements(secretValueType)
}
