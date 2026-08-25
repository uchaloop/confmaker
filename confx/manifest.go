package confx

import (
	"reflect"

	"github.com/uchaloop/secret/v2"
)

// secretValueType is the marker interface a secret type implements.
var secretValueType = reflect.TypeOf((*secret.Value)(nil)).Elem()

// Variable describes one environment variable a config reads. The set of them
// comes from binding the config, the same traversal that fills it, so a variable
// listed here is exactly a variable the config reads.
type Variable struct {
	// Name is the full variable name, prefix included.
	Name string
	// Type is the Go type of the field it fills.
	Type string
	// Required reports whether the field is declared required or notEmpty.
	Required bool
	// Secret reports whether the field holds a secret value.
	Secret bool
	// Default is the value the field holds before the environment is applied,
	// rendered as the text a variable would carry. It comes from the config's
	// SetDefaults method, when it has one.
	Default string
	// HasDefault reports whether a default was actually established - that is,
	// whether the field is non-zero before the environment is applied.
	HasDefault bool
}

// descriptor is the manifest one Provide call registers: the instance it builds
// and every variable that instance reads.
type descriptor struct {
	label     string
	prefix    string
	variables []Variable
}

// Manifest returns the environment variables a T built under name reads, in the
// order the type declares them, each with the default T establishes for it. It
// is the same manifest Module checks and WithDump prints, resolved without
// building an application - generate a .env.example, a deployment's ConfigMap,
// or a documentation table from it.
//
// The options are the ones Provide takes, so a manifest taken with WithPrefix
// matches the instance provided with the same option.
//
// A declaration the application would refuse - a default in a tag, a config
// nested through a pointer, two fields claiming one variable - is refused here
// too, so a generator fails instead of writing an empty file.
func Manifest[T any](name string, opts ...Option) ([]Variable, error) {
	prefix, err := resolvePrefix(name, opts)
	if err != nil {
		return nil, err
	}

	return manifestOf[T](prefix)
}

// manifestOf binds a defaulted T under prefix and returns the variables it
// reads. The defaults are established first so each variable carries the value
// its field holds before the environment is applied.
func manifestOf[T any](prefix string) ([]Variable, error) {
	var cfg T

	setDefaults(&cfg)

	bindings, err := bind(reflect.ValueOf(&cfg).Elem(), prefix)
	if err != nil {
		return nil, err
	}

	variables := make([]Variable, len(bindings))
	for i, b := range bindings {
		variables[i] = b.Variable
	}

	return variables, nil
}

// setDefaults lets a config establish its own defaults before anything else
// touches it. Declaring the method is optional; a config without one starts from
// its zero value.
func setDefaults[T any](cfg *T) {
	if d, ok := any(cfg).(interface{ SetDefaults() }); ok {
		d.SetDefaults()
	}
}

// isSecretType reports whether a field of type t holds a secret. With
// configuration read from the environment only, nothing but the environment can
// populate such a field, so the type is consulted purely so a value is reported
// as set or unset instead of being printed.
func isSecretType(t reflect.Type) bool {
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}

	return t.Implements(secretValueType)
}
