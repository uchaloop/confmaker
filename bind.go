package confmaker

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
)

// fieldSpec describes a leaf without holding a config value or rendered default.
// A registration shares this immutable schema between loading and inspection.
type fieldSpec struct {
	Name, Type       string
	Description      string
	Required, Secret bool
	NotEmpty         bool
	field            string
	index            []int
	render           renderer
	parse            parser
}

// prepareConfig validates registration options and compiles the schema before
// any config methods run. Loading and manifest generation use the same path.
func prepareConfig[T any](name string, opts []ConfigOption) (configSettings, []fieldSpec, error) {
	settings, err := resolveSettings[T](name, opts)
	if err != nil {
		return settings, nil, attachConfigInstance(name, makeConfigError(ErrorDeclaration, "", "", err))
	}

	fields, err := compileSchema(reflect.TypeFor[T](), settings.prefix)
	if err != nil {
		return settings, nil, wrapConfigError(settings.name, err)
	}

	return settings, fields, nil
}

// compileSchema requires a struct type validated by prepareConfig.
// It returns one fieldSpec per variable it
// reads, under the given prefix. Every problem in the declaration is reported at
// once, so a config with two mistakes does not take two runs to fix.
//
// A field is a leaf when its env tag names a variable. A struct field without
// one is descended into, extending the prefix with its envPrefix tag; anything
// else without an env tag is not configuration and is skipped.
func compileSchema(root reflect.Type, prefix string) ([]fieldSpec, error) {
	var errs []error

	// Most fields of a config are configuration, so the top-level count is a
	// close lower bound on the leaves - and a nested struct grows the slice from
	// there rather than from nothing.
	fieldSpecs := make([]fieldSpec, 0, root.NumField())

	appendFields(&fieldSpecs, &errs, root, prefix, "", nil)

	errs = append(errs, checkCollisions(fieldSpecs)...)

	return fieldSpecs, errors.Join(errs...)
}

// checkCollisions reports variables two fields both claim. One value would fill
// both fields and the manifest would list the name twice - almost always an
// envPrefix left off a second nested struct.
func checkCollisions(fieldSpecs []fieldSpec) []error {
	claimed := make(map[string]string, len(fieldSpecs))

	var errs []error

	for _, b := range fieldSpecs {
		if first, taken := claimed[b.Name]; taken {
			errs = append(errs, makeConfigError(ErrorConflict, b.Name, b.field, fmt.Errorf(
				"variable %q is claimed by both %s and %s; give one of them an envPrefix",
				b.Name, first, b.field,
			)))

			continue
		}

		claimed[b.Name] = b.field
	}

	return errs
}

func appendFields(fieldSpecs *[]fieldSpec, errs *[]error, structType reflect.Type, prefix, path string, parent []int) {
	for field := range structType.Fields() {
		// An unexported field cannot be assigned, an embedded unexported type
		// included, so it is not configuration.
		if len(field.PkgPath) != 0 {
			continue
		}

		fieldPath := path + field.Name
		name, suffix, hasOptions := strings.Cut(field.Tag.Get("env"), ",")
		appendFieldError := func(err error) {
			variableName := ""
			if len(name) != 0 {
				variableName = prefix + name
			}
			*errs = append(*errs, makeConfigError(ErrorDeclaration, variableName, fieldPath, err))
		}
		// `env:"-"` takes the field out of the config entirely, nested fields
		// included.
		if name == "-" {
			continue
		}

		if _, declared := field.Tag.Lookup("envDefault"); declared {
			appendFieldError(fmt.Errorf(
				"field %s declares envDefault; a default belongs in SetDefaults, where code and tests can see it",
				fieldPath,
			))

			continue
		}

		index := make([]int, len(parent)+len(field.Index))
		copy(index, parent)
		copy(index[len(parent):], field.Index)
		_, prefixDeclared := field.Tag.Lookup("envPrefix")
		_, formatDeclared := field.Tag.Lookup("envFormat")
		_, separatorDeclared := field.Tag.Lookup("envSeparator")
		_, keyValSeparatorDeclared := field.Tag.Lookup("envKeyValSeparator")

		switch {
		case len(name) == 0 && formatDeclared:
			appendFieldError(fmt.Errorf("field %s declares envFormat but names no variable", fieldPath))
		case len(name) == 0 && (separatorDeclared || keyValSeparatorDeclared):
			appendFieldError(fmt.Errorf("field %s declares a separator but names no variable", fieldPath))
		case len(name) != 0 && prefixDeclared:
			// envPrefix extends the prefix a nested struct is read under. On a
			// field that names its own variable it reads nothing, so a prefix
			// meant to apply would silently not.
			appendFieldError(fmt.Errorf(
				"field %s names a variable and declares envPrefix; write the prefix into the variable name instead",
				fieldPath,
			))
		case len(name) != 0:
			leaf, err := compileField(field, name, suffix, prefix, fieldPath, index)
			if err != nil {
				*errs = append(*errs, makeConfigError(ErrorDeclaration, prefix+name, fieldPath, err))
			} else {
				*fieldSpecs = append(*fieldSpecs, leaf)
			}
		case hasOptions:
			// A tag of options alone reads nothing. Treating it as an untagged
			// field would drop a field its author plainly meant to configure.
			appendFieldError(fmt.Errorf(
				"field %s carries env options but names no variable", fieldPath,
			))
		case field.Type.Kind() == reflect.Struct:
			appendFields(fieldSpecs, errs, field.Type, prefix+field.Tag.Get("envPrefix"), fieldPath+".", index)
		case nestsConfig(field.Type):
			appendFieldError(fmt.Errorf(
				"field %s nests a config through %s; nest by value so the variables it reads are known from the type",
				fieldPath, field.Type.Kind(),
			))
		case prefixDeclared:
			// The field is not configuration at all: it names no variable and
			// is not a struct to descend into, so the prefix extends nothing
			// and the field is skipped entirely. Saying so beats dropping it.
			appendFieldError(fmt.Errorf(
				"field %s declares envPrefix but is not a struct nested by value; only such a struct extends the prefix",
				fieldPath,
			))
		}
	}
}

// compileField builds the fieldSpec of a field that names a variable, choosing the
// parser its type calls for.
func compileField(
	field reflect.StructField,
	name, suffix, prefix, fieldPath string,
	index []int,
) (fieldSpec, error) {
	opts, err := parseEnvTagOptions(suffix)
	if err != nil {
		return fieldSpec{}, fmt.Errorf("field %s: %w", fieldPath, err)
	}

	if err := checkPointerChains(field.Type, make(map[reflect.Type]bool)); err != nil {
		return fieldSpec{}, fmt.Errorf("field %s: %w", fieldPath, err)
	}

	if holdsSecretInCollection(field.Type) {
		return fieldSpec{}, fmt.Errorf(
			"field %s holds a secret in a %s; give each secret its own variable",
			fieldPath, field.Type.Kind(),
		)
	}

	// The environment cannot carry a name with = or NUL: such a variable could
	// be passed through WithEnv but never set in a process.
	fullName := prefix + name
	if strings.ContainsAny(fullName, "=\x00") {
		return fieldSpec{}, fmt.Errorf("field %s reads %q, which no environment variable can be named", fieldPath, fullName)
	}

	secret := isSecretType(field.Type)
	codec, err := makeFieldCodec(field)
	if err != nil {
		return fieldSpec{}, fmt.Errorf("field %s: %w", fieldPath, err)
	}

	return fieldSpec{
		Name: fullName, Type: field.Type.String(), Description: field.Tag.Get("envDescription"),
		Required: opts.required || opts.notEmpty, NotEmpty: opts.notEmpty, Secret: secret,
		field: fieldPath, index: index, parse: codec.parse,
		render: codec.render,
	}, nil
}

// applyAndValidate applies the environment over the defaults already in cfg and
// validates the result. Validate runs only when every variable parsed: a
// half-filled config would report problems that are not there.
func applyAndValidate[T any](cfg *T, fields []fieldSpec, name string, env environment, report *ConfigReport) error {
	if err := applyEnvironment(reflect.ValueOf(cfg).Elem(), fields, env, report); err != nil {
		return wrapConfigError(name, err)
	}

	// Check cfg (a *T), not *cfg: *T's method set includes both value- and
	// pointer-receiver Validate methods, so a library that declares Validate on a
	// pointer receiver is still validated.
	if v, ok := any(cfg).(interface{ Validate() error }); ok {
		if err := v.Validate(); err != nil {
			return wrapConfigError(name, makeConfigError(ErrorValidation, "", "", err))
		}
	}

	return nil
}

// applyEnvironment reads environment variables into fields at their compiled paths.
// A variable that is not set
// leaves its field untouched - that is what preserves the values SetDefaults
// established - and every problem is reported together.
func applyEnvironment(root reflect.Value, fieldSpecs []fieldSpec, env environment, report *ConfigReport) error {
	var errs []error

	for i, b := range fieldSpecs {
		raw, set := env[b.Name]
		var variable *VariableReport
		if report != nil {
			variable = &report.Variables[i]
			variable.Status = VariableNotProcessed
			switch {
			case set:
				variable.Source = SourceEnv
			case b.Required:
				variable.Source = SourceMissing
			case root.FieldByIndex(b.index).IsZero():
				variable.Source = SourceZero
			default:
				variable.Source = SourceDefault
			}
		}

		switch {
		case !set && b.Required:
			if variable != nil {
				variable.Status = VariableFailed
			}
			errs = append(errs, makeConfigError(ErrorRequired, b.Name, b.field, fmt.Errorf("required variable %q is not set", b.Name)))

			continue
		case !set:
			if variable != nil {
				variable.Status = VariableSucceeded
			}
			continue
		case b.NotEmpty && len(raw) == 0:
			if variable != nil {
				variable.Status = VariableFailed
			}
			errs = append(errs, makeConfigError(ErrorEmpty, b.Name, b.field, fmt.Errorf("variable %q is set but empty", b.Name)))

			continue
		}

		if err := b.parse(root.FieldByIndex(b.index), raw); err != nil {
			if variable != nil {
				variable.Status = VariableFailed
			}
			errs = append(errs, describeParseError(b, err))
		} else if variable != nil {
			variable.Status = VariableSucceeded
		}
	}

	return errors.Join(errs...)
}

// describeParseError reports a value that would not parse. The value of a secret
// is left out: the variable is named, never its contents.
func describeParseError(b fieldSpec, err error) error {
	if b.Secret {
		return makeConfigError(ErrorParse, b.Name, b.field, fmt.Errorf("variable %q holds a value that is not valid for %s", b.Name, b.Type))
	}

	return makeConfigError(ErrorParse, b.Name, b.field, fmt.Errorf("variable %q: %w", b.Name, err))
}

// holdsSecretInCollection reports whether t is a collection of secrets. One
// arrives as the whole value of one variable, and inside a collection it would
// be split on a separator it has no way to escape - so a token holding a comma
// would become two unusable secrets. Nothing would say so either: the value a
// secret carries is never printed, and neither is the reason it would not parse.
//
// isSecretType unwraps a pointer, so a collection of pointers to secrets counts
// the same way.
func holdsSecretInCollection(t reflect.Type) bool {
	switch t.Kind() {
	case reflect.Slice, reflect.Array:
		return isSecretType(t.Elem())
	case reflect.Map:
		return isSecretType(t.Key()) || isSecretType(t.Elem())
	default:
		return false
	}
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

// declaresVariables follows nested shapes with cycle protection. Untagged
// fields only count as configuration when their type graph declares ENV fields.
func declaresVariables(t reflect.Type) bool {
	return typeDeclaresVariables(t, make(map[reflect.Type]bool))
}

func typeDeclaresVariables(t reflect.Type, seen map[reflect.Type]bool) bool {
	if seen[t] {
		return false
	}
	seen[t] = true

	switch t.Kind() {
	case reflect.Pointer, reflect.Slice, reflect.Array, reflect.Map:
		return typeDeclaresVariables(t.Elem(), seen)
	case reflect.Struct:
		for field := range t.Fields() {
			if len(field.PkgPath) != 0 {
				continue
			}

			name, _, _ := strings.Cut(field.Tag.Get("env"), ",")
			if name == "-" {
				continue
			}
			if len(name) != 0 || typeDeclaresVariables(field.Type, seen) {
				return true
			}
		}
	}

	return false
}

// checkPointerChains rejects pointer-only cycles, while allowing recursive
// structs and collections whose pointers eventually reach a concrete shape.
func checkPointerChains(t reflect.Type, seen map[reflect.Type]bool) error {
	if seen[t] {
		return nil
	}
	seen[t] = true

	chain := make(map[reflect.Type]bool)
	for t.Kind() == reflect.Pointer {
		if chain[t] {
			return fmt.Errorf("cyclic pointer chain at %s cannot represent an ENV value", t)
		}
		chain[t] = true
		t = t.Elem()
	}

	switch t.Kind() {
	case reflect.Slice, reflect.Array:
		return checkPointerChains(t.Elem(), seen)
	case reflect.Map:
		if err := checkPointerChains(t.Key(), seen); err != nil {
			return err
		}
		return checkPointerChains(t.Elem(), seen)
	case reflect.Struct:
		for field := range t.Fields() {
			if err := checkPointerChains(field.Type, seen); err != nil {
				return err
			}
		}
	}

	return nil
}

// envTagOptions are what an env tag's comma-separated suffix carries.
type envTagOptions struct {
	required bool
	notEmpty bool
}

// parseEnvTagOptions reads that suffix. An option the tag does not define is an
// error: an unrecognised one would otherwise be ignored, so a misspelled
// "require" would leave the field optional and say nothing.
func parseEnvTagOptions(suffix string) (envTagOptions, error) {
	var (
		parsed envTagOptions
		errs   []error
	)

	for rest := suffix; len(rest) != 0; {
		var current string

		current, rest, _ = strings.Cut(rest, ",")

		switch current {
		case "":
		case "required":
			parsed.required = true
		case "notEmpty":
			parsed.notEmpty = true
		default:
			errs = append(errs, fmt.Errorf(
				"unknown env option %q; the tag takes required and notEmpty",
				current,
			))
		}
	}

	return parsed, errors.Join(errs...)
}

// renderer turns a field's default back into the text its variable would carry.
type renderer func(reflect.Value) (string, error)

// fieldCodec pairs the parser and renderer for the same ENV representation.
type fieldCodec struct {
	parse  parser
	render renderer
}

// makeFieldCodec chooses how a field is read and rendered: envFormat:"json", or the
// plain syntax with the field's separators.
func makeFieldCodec(field reflect.StructField) (fieldCodec, error) {
	format, explicit := field.Tag.Lookup("envFormat")
	if explicit {
		if format != "json" {
			return fieldCodec{}, fmt.Errorf("unknown envFormat %q; supported format is json", format)
		}

		for _, tag := range []string{"envSeparator", "envKeyValSeparator"} {
			if _, exists := field.Tag.Lookup(tag); exists {
				return fieldCodec{}, fmt.Errorf("envFormat json cannot be combined with %s", tag)
			}
		}

		parse, err := jsonParser(field.Type)

		if err != nil {
			return fieldCodec{}, err
		}

		return fieldCodec{parse: parse, render: renderJSON}, nil
	}

	// Lookup, not Get: an envSeparator written empty is an error, not the default.
	separator, separatorDeclared := field.Tag.Lookup("envSeparator")
	if !separatorDeclared {
		separator = defaultSeparator
	}

	keyValSeparator, keyValSeparatorDeclared := field.Tag.Lookup("envKeyValSeparator")
	if !keyValSeparatorDeclared {
		keyValSeparator = defaultKeyValSeparator
	}

	// A separator on a field that is not split would be ignored in silence. A
	// type with its own text form reads the whole value, so it is not split.
	splits := !declaresTextForm(field.Type)
	if separatorDeclared && !(splits && (field.Type.Kind() == reflect.Slice || field.Type.Kind() == reflect.Map)) {
		return fieldCodec{}, fmt.Errorf("envSeparator applies to a slice or map read in the plain syntax, not %s", field.Type)
	}

	if keyValSeparatorDeclared && !(splits && field.Type.Kind() == reflect.Map) {
		return fieldCodec{}, fmt.Errorf("envKeyValSeparator applies to a map read in the plain syntax, not %s", field.Type)
	}

	parse, err := fieldParser(field.Type, separator, keyValSeparator)

	if err != nil {
		return fieldCodec{}, err
	}

	return fieldCodec{
		parse: parse,
		render: func(value reflect.Value) (string, error) {
			return renderValue(value, separator, keyValSeparator)
		},
	}, nil
}
