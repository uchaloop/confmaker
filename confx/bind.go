package confx

import (
	"errors"
	"fmt"
	"os"
	"reflect"
	"strings"
)

// binding is one leaf of a config: the variable it reads and the field that
// variable fills. Binding a config produces the whole set at once, which is what
// lets the same traversal fill the fields and describe them.
type binding struct {
	Variable

	target          reflect.Value
	notEmpty        bool
	separator       string
	keyValSeparator string
}

// bind walks an addressable struct and returns one binding per variable it
// reads, under the given prefix. Every problem in the declaration is reported at
// once, so a config with two mistakes does not take two runs to fix.
//
// A field is a leaf when its env tag names a variable. A struct field without
// one is descended into, extending the prefix with its envPrefix tag; anything
// else without an env tag is not configuration and is skipped.
func bind(root reflect.Value, prefix string) ([]binding, error) {
	if root.Kind() != reflect.Struct {
		return nil, fmt.Errorf("a config must be a struct, got %s", root.Type())
	}

	var (
		bindings []binding
		errs     []error
	)

	appendBindings(&bindings, &errs, root, prefix)

	return bindings, errors.Join(errs...)
}

func appendBindings(bindings *[]binding, errs *[]error, structValue reflect.Value, prefix string) {
	structType := structValue.Type()

	for i := range structType.NumField() {
		field := structType.Field(i)
		// An unexported field cannot be assigned, an embedded unexported type
		// included, so it is not configuration.
		if len(field.PkgPath) != 0 {
			continue
		}

		name, options, _ := strings.Cut(field.Tag.Get("env"), ",")
		// `env:"-"` takes the field out of the config entirely, nested fields
		// included.
		if name == "-" {
			continue
		}

		if _, declared := field.Tag.Lookup("envDefault"); declared {
			*errs = append(*errs, fmt.Errorf(
				"field %s declares envDefault; a default belongs in SetDefaults, where code and tests can see it",
				field.Name,
			))

			continue
		}

		value := structValue.Field(i)

		if len(name) != 0 {
			*bindings = append(*bindings, bindLeaf(field, name, options, prefix, value))

			continue
		}

		switch {
		case field.Type.Kind() == reflect.Struct:
			appendBindings(bindings, errs, value, prefix+field.Tag.Get("envPrefix"))
		case holdsStruct(field.Type):
			*errs = append(*errs, fmt.Errorf(
				"field %s nests a struct through %s; nest by value so the variables it reads are known from the type",
				field.Name, field.Type.Kind(),
			))
		}
	}
}

// bindLeaf builds the binding of a field that names a variable.
func bindLeaf(field reflect.StructField, name, options, prefix string, value reflect.Value) binding {
	required := hasOption(options, "required")
	notEmpty := hasOption(options, "notEmpty")

	return binding{
		Variable: Variable{
			Name:       prefix + name,
			Type:       field.Type.String(),
			Required:   required || notEmpty,
			Secret:     isSecretType(field.Type),
			Default:    renderValue(value, separatorOf(field), keyValSeparatorOf(field)),
			HasDefault: !value.IsZero(),
		},
		target:          value,
		notEmpty:        notEmpty,
		separator:       separatorOf(field),
		keyValSeparator: keyValSeparatorOf(field),
	}
}

// apply reads the environment into the bound fields. A variable that is not set
// leaves its field untouched - that is what preserves the values SetDefaults
// established - and every problem is reported together.
func apply(bindings []binding) error {
	var errs []error

	for _, b := range bindings {
		raw, set := os.LookupEnv(b.Name)

		switch {
		case !set && b.Required:
			errs = append(errs, fmt.Errorf("required variable %q is not set", b.Name))

			continue
		case !set:
			continue
		case b.notEmpty && len(raw) == 0:
			errs = append(errs, fmt.Errorf("variable %q is set but empty", b.Name))

			continue
		}

		if err := setField(b.target, raw, b.separator, b.keyValSeparator); err != nil {
			errs = append(errs, describeSetError(b, err))
		}
	}

	return errors.Join(errs...)
}

// describeSetError reports a value that would not parse. The value of a secret
// is left out: the variable is named, never its contents.
func describeSetError(b binding, err error) error {
	if b.Secret {
		return fmt.Errorf("variable %q holds a value that is not valid for %s", b.Name, b.Type)
	}

	return fmt.Errorf("variable %q: %w", b.Name, err)
}

// holdsStruct reports whether t reaches a struct through a pointer or a
// collection - shapes whose variables cannot be known from the type alone,
// because they depend on what is allocated or how many elements exist.
func holdsStruct(t reflect.Type) bool {
	switch t.Kind() {
	case reflect.Pointer, reflect.Slice, reflect.Array:
		return t.Elem().Kind() == reflect.Struct
	default:
		return false
	}
}

func separatorOf(field reflect.StructField) string {
	if separator, ok := field.Tag.Lookup("envSeparator"); ok {
		return separator
	}

	return defaultSeparator
}

func keyValSeparatorOf(field reflect.StructField) string {
	if separator, ok := field.Tag.Lookup("envKeyValSeparator"); ok {
		return separator
	}

	return defaultKeyValSeparator
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
