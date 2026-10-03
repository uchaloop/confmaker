package confmaker

import (
	"errors"
	"fmt"
	"reflect"
	"slices"

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
	// Description is the optional envDescription tag on the field.
	Description string
	// Type is the Go type of the field it fills.
	Type string
	// Required reports whether the variable must be set: the field is declared
	// required or notEmpty.
	Required bool
	// NotEmpty reports whether the variable must also hold a non-empty value: the
	// field is declared notEmpty. Without it, required permits empty text,
	// provided the field type can parse it.
	NotEmpty bool
	// Secret reports whether the field holds a secret value.
	Secret bool
	// Default is the value the field holds after SetDefaults, rendered as the
	// text a variable would carry. It is empty for a secret.
	Default string
	// HasDefault reports whether the field is non-zero before the environment is
	// applied. A default of false, 0 or "" is indistinguishable from no default
	// and is reported as false. Secret fields always report false.
	HasDefault bool

	complexDefault bool
}

// ConfigManifest describes one registered configuration and its environment
// variables. It contains defaults, not values read from the environment.
type ConfigManifest struct {
	// InstanceName is the name passed to Loader.Register.
	InstanceName string
	// Prefix is the resolved ENV prefix, including any WithPrefix override.
	Prefix string
	// Variables lists fields in declaration order. Secret defaults are omitted.
	Variables []Variable
}

// Manifest describes a snapshot of all registrations, ordered by instance name.
// Each config's variables retain declaration order. Invalid registrations and
// conflicts are reported before any config methods run. Rendering errors are
// collected across configs; any error returns a nil result, never a partial list.
//
// Manifest does not read ENV, call Validate or change loader state.
// Loading options and their errors are ignored
// here. Each call runs SetDefaults and default marshalers on fresh instances;
// it never inspects or changes loaded values or makes a Handle ready.
//
// Manifest can run before, during or after Load, including after a failed load.
// Concurrent registrations completed after the snapshot belong to a later call.
// No loader lock is held while config methods run. These methods must be safe
// for concurrent calls if Manifest or Load are called concurrently. As with the
// package-level Manifest, a panic from user code propagates to the caller.
func (l *Loader) Manifest() ([]ConfigManifest, error) {
	l.mu.Lock()
	registrations := slices.Clone(l.registrations)
	l.mu.Unlock()

	sortRegistrations(registrations)
	var errs []error
	descriptors := make([]descriptor, 0, len(registrations))
	for _, registration := range registrations {
		if registration.err != nil {
			errs = append(errs, registration.err)

			continue
		}

		descriptors = append(descriptors, registration.descriptor)
	}

	if _, err := checkRegistrations(descriptors); err != nil {
		errs = append(errs, err)
	}

	if err := errors.Join(errs...); err != nil {
		return nil, err
	}

	manifest := make([]ConfigManifest, 0, len(descriptors))
	for _, config := range descriptors {
		variables, err := manifestVariables(config)
		if err != nil {
			errs = append(errs, err)

			continue
		}

		manifest = append(manifest, ConfigManifest{
			InstanceName: config.instanceName,
			Prefix:       config.prefix,
			Variables:    variables,
		})
	}

	if err := errors.Join(errs...); err != nil {
		return nil, err
	}

	return manifest, nil
}

// Manifest returns the variables a config of type T reads, in declaration order,
// for generating a .env.example, a config map or a documentation table. The
// required name and options match [Loader.Register], so the same arguments
// describe the same environment variables.
//
// Manifest does not read the environment. It does call T's SetDefaults, and
// renders each default with MarshalText or the field's plain syntax; a default
// that cannot be rendered is an error. The result is metadata: [Variable.Default]
// is ENV text, not escaped for a shell or YAML.
//
// A declaration [Loader.Load] would refuse is refused here too, before
// SetDefaults runs.
func Manifest[T any](name string, opts ...ConfigOption) ([]Variable, error) {
	config, err := makeDescriptor[T](name, opts)
	if err != nil {
		return nil, err
	}

	return manifestVariables(config)
}

// manifestVariables renders an independent instance using the same schema as Load.
func manifestVariables(config descriptor) ([]Variable, error) {
	variables, err := describeFields(reflect.ValueOf(config.defaults()).Elem(), config.fields)
	if err != nil {
		return nil, wrapConfigError(config.instanceName, err)
	}

	return variables, nil
}

// describeFields renders defaults only for consumers that need a manifest.
// Loading never calls a user-supplied text marshaler.
func describeFields(root reflect.Value, fields []fieldSpec) ([]Variable, error) {
	variables := make([]Variable, len(fields))
	var errs []error
	for i, field := range fields {
		variable := Variable{Name: field.Name, Type: field.Type, Description: field.Description, Required: field.Required, NotEmpty: field.NotEmpty, Secret: field.Secret}
		if !variable.Secret {
			target := root.FieldByIndex(field.index)
			var err error
			variable.Default, err = field.render(target)
			if err != nil {
				errs = append(errs, makeConfigError(ErrorDefaultRender, field.Name, field.field, fmt.Errorf("field %s (variable %q): cannot render default: %w", field.field, field.Name, err)))

				continue
			}

			variable.HasDefault = !target.IsZero()
			defaultType := target.Type()
			for defaultType.Kind() == reflect.Pointer {
				defaultType = defaultType.Elem()
			}

			switch defaultType.Kind() {
			case reflect.Struct, reflect.Slice, reflect.Array, reflect.Map:
				variable.complexDefault = true
			}
		}

		variables[i] = variable
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
