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

	// field is the path of the struct field this variable fills, for an error
	// that has to point at a declaration rather than at the environment.
	field    string
	target   reflect.Value
	notEmpty bool
	// parse was chosen from the field's type when the config was bound, so
	// applying the environment is only a lookup and a call.
	parse parser
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

	appendBindings(&bindings, &errs, root, prefix, "")

	errs = append(errs, checkCollisions(bindings)...)

	return bindings, errors.Join(errs...)
}

// checkCollisions reports variables two fields both claim. One value would fill
// both fields and the manifest would list the name twice - almost always an
// envPrefix left off a second nested struct.
func checkCollisions(bindings []binding) []error {
	claimed := make(map[string]string, len(bindings))

	var errs []error

	for _, b := range bindings {
		if first, taken := claimed[b.Name]; taken {
			errs = append(errs, fmt.Errorf(
				"variable %q is claimed by both %s and %s; give one of them an envPrefix",
				b.Name, first, b.field,
			))

			continue
		}

		claimed[b.Name] = b.field
	}

	return errs
}

func appendBindings(bindings *[]binding, errs *[]error, structValue reflect.Value, prefix, path string) {
	structType := structValue.Type()

	for i := range structType.NumField() {
		field := structType.Field(i)
		// An unexported field cannot be assigned, an embedded unexported type
		// included, so it is not configuration.
		if len(field.PkgPath) != 0 {
			continue
		}

		fieldPath := path + field.Name

		name, suffix, hasOptions := strings.Cut(field.Tag.Get("env"), ",")
		// `env:"-"` takes the field out of the config entirely, nested fields
		// included.
		if name == "-" {
			continue
		}

		if _, declared := field.Tag.Lookup("envDefault"); declared {
			*errs = append(*errs, fmt.Errorf(
				"field %s declares envDefault; a default belongs in SetDefaults, where code and tests can see it",
				fieldPath,
			))

			continue
		}

		value := structValue.Field(i)

		switch {
		case len(name) != 0:
			leaf, err := bindLeaf(field, name, suffix, prefix, fieldPath, value)
			if err != nil {
				*errs = append(*errs, err)
			} else {
				*bindings = append(*bindings, leaf)
			}
		case hasOptions:
			// A tag of options alone reads nothing. Treating it as an untagged
			// field would drop a field its author plainly meant to configure.
			*errs = append(*errs, fmt.Errorf(
				"field %s carries env options but names no variable", fieldPath,
			))
		case field.Type.Kind() == reflect.Struct:
			appendBindings(bindings, errs, value, prefix+field.Tag.Get("envPrefix"), fieldPath+".")
		case nestsConfig(field.Type):
			*errs = append(*errs, fmt.Errorf(
				"field %s nests a config through %s; nest by value so the variables it reads are known from the type",
				fieldPath, field.Type.Kind(),
			))
		}
	}
}

// bindLeaf builds the binding of a field that names a variable, choosing the
// parser its type calls for.
func bindLeaf(
	field reflect.StructField,
	name, suffix, prefix, fieldPath string,
	value reflect.Value,
) (binding, error) {
	opts, err := parseOptions(suffix)
	if err != nil {
		return binding{}, fmt.Errorf("field %s: %w", fieldPath, err)
	}

	var (
		secret          = isSecretType(field.Type)
		separator       = separatorOf(field)
		keyValSeparator = keyValSeparatorOf(field)
	)

	parse, err := fieldParser(field.Type, separator, keyValSeparator)
	if err != nil {
		return binding{}, fmt.Errorf("field %s: %w", fieldPath, err)
	}

	variable := Variable{
		Name:     prefix + name,
		Type:     field.Type.String(),
		Required: opts.require || opts.notEmpty,
		Secret:   secret,
	}
	// A secret publishes no default. Its rendering would be a mask, which reads
	// as a value and would be pasted into a deployment as one.
	if !secret {
		variable.Default = renderValue(value, separator, keyValSeparator)
		variable.HasDefault = !value.IsZero()
	}

	return binding{
		Variable: variable,
		field:    fieldPath,
		target:   value,
		notEmpty: opts.notEmpty,
		parse:    parse,
	}, nil
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

		if err := b.parse(b.target, raw); err != nil {
			errs = append(errs, describeParseError(b, err))
		}
	}

	return errors.Join(errs...)
}

// describeParseError reports a value that would not parse. The value of a secret
// is left out: the variable is named, never its contents.
func describeParseError(b binding, err error) error {
	if b.Secret {
		return fmt.Errorf("variable %q holds a value that is not valid for %s", b.Name, b.Type)
	}

	return fmt.Errorf("variable %q: %w", b.Name, err)
}

// nestsConfig reports whether t reaches a config through a pointer or a
// collection - a shape whose variables cannot be known from the type alone,
// because they depend on what is allocated or how many elements exist.
//
// Only a struct that names variables of its own counts. A field holding a
// decimal, a nullable, or a timestamp reaches a struct too, and one without an
// env tag is simply not configuration, the same as any other untagged field.
func nestsConfig(t reflect.Type) bool {
	switch t.Kind() {
	case reflect.Pointer, reflect.Slice, reflect.Array, reflect.Map:
		return declaresVariables(t.Elem())
	default:
		return false
	}
}

// declaresVariables reports whether t, or a struct nested in it by value, names
// an environment variable.
func declaresVariables(t reflect.Type) bool {
	if t.Kind() != reflect.Struct {
		return false
	}

	for i := range t.NumField() {
		field := t.Field(i)
		if len(field.PkgPath) != 0 {
			continue
		}

		name, _, _ := strings.Cut(field.Tag.Get("env"), ",")

		switch {
		case name == "-":
			continue
		case len(name) != 0:
			return true
		case declaresVariables(field.Type):
			return true
		}
	}

	return false
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

// options are what an env tag's comma-separated suffix carries.
type options struct {
	require  bool
	notEmpty bool
}

// parseOptions reads that suffix. An option the tag does not define is an
// error: an unrecognised one would otherwise be ignored, so a misspelled
// "require" would leave the field optional and say nothing.
func parseOptions(suffix string) (options, error) {
	var (
		parsed options
		errs   []error
	)

	for rest := suffix; len(rest) != 0; {
		var current string

		current, rest, _ = strings.Cut(rest, ",")

		switch current {
		case "":
		case "require":
			parsed.require = true
		case "notEmpty":
			parsed.notEmpty = true
		default:
			errs = append(errs, fmt.Errorf(
				"unknown env option %q; the tag takes require and notEmpty",
				current,
			))
		}
	}

	return parsed, errors.Join(errs...)
}
