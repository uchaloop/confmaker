package confmaker

import (
	"errors"
	"fmt"
	"reflect"

	"github.com/uchaloop/secret/v2"
)

// secretValueType is the marker interface a secret type implements.
var secretValueType = reflect.TypeFor[secret.Value]()

// Variable describes one environment variable a config reads. The set of them
// comes from the same compiled schema used to load it, so a variable listed
// here is exactly a variable the config reads.
type Variable struct {
	// Name is the full variable name, prefix included.
	Name string
	// Type is the Go type of the field it fills.
	Type string
	// Required reports whether the variable must be set: the field is declared
	// required or notEmpty.
	Required bool
	// NotEmpty reports whether the variable must also hold a non-empty value: the
	// field is declared notEmpty. A required field accepts X= as a deliberate
	// empty value.
	NotEmpty bool
	// Secret reports whether the field holds a secret value.
	Secret bool
	// Default is the value the field holds after SetDefaults, rendered as the
	// text a variable would carry. It is empty for a secret.
	Default string
	// HasDefault reports whether the field is non-zero before the environment is
	// applied. A default of false, 0 or "" is indistinguishable from no default
	// and is reported as false.
	HasDefault bool
}

// Manifest returns the variables a config of type T reads, in declaration order,
// for generating a .env.example, a config map or a documentation table. The
// options are those of [Loader.Add], so WithName and WithPrefix give the same
// names as a registration with the same options.
//
// Manifest does not read the environment. It does call T's SetDefaults, and
// renders each default with MarshalText or the field's plain syntax; a default
// that cannot be rendered is an error. The result is metadata: [Variable.Default]
// is ENV text, not escaped for a shell or YAML.
//
// A declaration [Loader.Load] would refuse is refused here too, before
// SetDefaults runs.
func Manifest[T any](opts ...ConfigOption) ([]Variable, error) {
	set, err := resolveSettings[T](opts)
	if err != nil {
		return nil, err
	}

	// The schema is checked before SetDefaults runs: an invalid declaration
	// fails without calling user code.
	fields, err := compileSchema(reflect.TypeFor[T](), set.prefix)
	if err != nil {
		return nil, makeConfigError(set.name, err)
	}

	var cfg T
	setDefaults(&cfg)

	variables, err := describeFields(reflect.ValueOf(&cfg).Elem(), fields)
	if err != nil {
		return nil, makeConfigError(set.name, err)
	}

	return variables, nil
}

// describeFields renders defaults only for consumers that need a manifest.
// Loading a config never calls a user-supplied text marshaler.
func describeFields(root reflect.Value, fields []fieldSpec) ([]Variable, error) {
	variables := make([]Variable, len(fields))
	var errs []error
	for i, b := range fields {
		v := Variable{Name: b.Name, Type: b.Type, Required: b.Required, NotEmpty: b.NotEmpty, Secret: b.Secret}
		if !v.Secret {
			target := root.FieldByIndex(b.index)
			var err error
			v.Default, err = b.render(target)
			if err != nil {
				errs = append(errs, fmt.Errorf("field %s (variable %q): cannot render default: %w", b.field, b.Name, err))
				continue
			}

			v.HasDefault = !target.IsZero()
		}

		variables[i] = v
	}

	if err := errors.Join(errs...); err != nil {
		return nil, err
	}

	return variables, nil
}

// setDefaults establishes defaults independently for loading and manifest
// generation. The method must be deterministic and free of side effects.
// A config without it starts from its zero value.
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
	// The parser follows any number of pointers, so the check does too:
	// ***secret.Secret is as much a secret as secret.Secret.
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}

	return t.Implements(secretValueType) || reflect.PointerTo(t).Implements(secretValueType)
}
