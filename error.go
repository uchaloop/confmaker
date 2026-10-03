package confmaker

import (
	"errors"
	"fmt"
	"strings"
)

// configError labels each error line with its instance name.
type configError struct {
	name string
	err  error
}

func wrapConfigError(name string, err error) error {
	return configError{name: name, err: attachConfigInstance(name, err)}
}

func (e configError) Error() string {
	lines := strings.Split(e.err.Error(), "\n")
	for i, line := range lines {
		lines[i] = fmt.Sprintf("config %q: %s", e.name, line)
	}

	return strings.Join(lines, "\n")
}

// Unwrap keeps errors.Is and errors.As reaching what the stage reported, each
// error inside a join included.
func (e configError) Unwrap() error { return e.err }

// ErrorKind identifies the stage or condition that produced a ConfigError.
// The zero value does not identify a category.
type ErrorKind string

const (
	// ErrorDeclaration identifies invalid config types, tags, names or options.
	ErrorDeclaration ErrorKind = "declaration"
	// ErrorConflict identifies competing names, prefixes or variables.
	ErrorConflict ErrorKind = "conflict"
	// ErrorRequired identifies an absent required variable.
	ErrorRequired ErrorKind = "required"
	// ErrorEmpty identifies an empty variable declared notEmpty.
	ErrorEmpty ErrorKind = "empty"
	// ErrorParse identifies a value that could not be parsed.
	ErrorParse ErrorKind = "parse"
	// ErrorUnknownVariable identifies an undeclared variable under a config prefix.
	ErrorUnknownVariable ErrorKind = "unknown_variable"
	// ErrorValidation identifies an error returned by Validate.
	ErrorValidation ErrorKind = "validation"
	// ErrorDefaultRender identifies a default that could not be rendered.
	ErrorDefaultRender ErrorKind = "default_render"
)

// ConfigError describes one configuration problem. Context fields are empty
// when unavailable; conflicts involving multiple instances have no single owner.
// No field holds an ENV value. Error messages and causes for ordinary fields
// and user validation may contain values; secret parse causes are discarded.
// Use errors.As to find the first problem or ConfigErrors to collect all of them.
type ConfigError struct {
	Kind         ErrorKind
	InstanceName string
	VariableName string
	FieldPath    string
	err          error
}

// Error returns the problem's message. The surrounding load error adds the
// instance label when applicable.
func (e *ConfigError) Error() string {
	if e.err == nil {
		return string(e.Kind)
	}

	return e.err.Error()
}

// Unwrap preserves the original cause, except for secret parse errors, whose
// unsafe cause is replaced with a value-free message.
func (e *ConfigError) Unwrap() error { return e.err }

func makeConfigError(kind ErrorKind, variableName, fieldPath string, err error) *ConfigError {
	return &ConfigError{Kind: kind, VariableName: variableName, FieldPath: fieldPath, err: err}
}

// ConfigErrors returns all structured configuration problems in err, including
// wrapped and joined errors. It returns nil if none are present. Validation
// errors remain one problem even when Validate returns joined errors.
// Results preserve error order and repeated occurrences. Treat the returned
// pointers as read-only; they refer to the original errors.
func ConfigErrors(err error) []*ConfigError {
	var result []*ConfigError
	var visit func(error)
	visit = func(err error) {
		if err == nil {
			return
		}

		if problem, ok := err.(*ConfigError); ok {
			if problem != nil {
				result = append(result, problem)
			}

			return
		}

		switch wrapped := err.(type) {
		case interface{ Unwrap() []error }:
			for _, child := range wrapped.Unwrap() {
				visit(child)
			}
		case interface{ Unwrap() error }:
			visit(wrapped.Unwrap())
		}
	}

	visit(err)

	return result
}

// attachConfigInstance copies library-owned problems before attributing them.
// It does not rewrite user causes inside a ConfigError.
func attachConfigInstance(name string, err error) error {
	switch problem := err.(type) {
	case *ConfigError:
		copy := *problem
		copy.InstanceName = name
		return &copy
	case interface{ Unwrap() []error }:
		children := problem.Unwrap()
		copies := make([]error, len(children))
		for i, child := range children {
			copies[i] = attachConfigInstance(name, child)
		}

		return errors.Join(copies...)
	default:
		return err
	}
}

// scalarParseError keeps a concise diagnostic and the standard parser's cause.
// Secret field handling discards this entire chain before reporting the error.
type scalarParseError struct {
	message string
	cause   error
}

func makeScalarParseError(cause error, format string, args ...any) *scalarParseError {
	return &scalarParseError{message: fmt.Sprintf(format, args...), cause: cause}
}

func (e *scalarParseError) Error() string { return e.message }

func (e *scalarParseError) Unwrap() error { return e.cause }
