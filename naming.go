package confmaker

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
)

// ConfigNamer is implemented by a config that names its default instance, for
// example "postgres". A config implements it without importing this package.
// ConfigName is called on a zero value, before SetDefaults, with a value or
// pointer receiver, and must return a constant name. [WithName] takes precedence.
type ConfigNamer interface {
	ConfigName() string
}

// applyAndValidate applies the environment over the defaults already in cfg and
// validates the result. Validate runs only when every variable parsed: a
// half-filled config would report problems that are not there.
func applyAndValidate[T any](cfg *T, fields []fieldSpec, name string, env environment) error {
	if err := apply(reflect.ValueOf(cfg).Elem(), fields, env); err != nil {
		return makeConfigError(name, err)
	}

	// Check cfg (a *T), not *cfg: *T's method set includes both value- and
	// pointer-receiver Validate methods, so a library that declares Validate on a
	// pointer receiver is still validated.
	if v, ok := any(cfg).(interface{ Validate() error }); ok {
		if err := v.Validate(); err != nil {
			return makeConfigError(name, err)
		}
	}

	return nil
}

// configError attributes an error to the instance it came from. Every stage
// reports all of its problems at once, and a joined error renders one per line -
// so the name goes on each of them, not only on the first. A line that scrolls
// past on its own still says which config it is about, which matters when one
// process builds several.
//
// The name is put on the rendered lines rather than on the errors behind them,
// because a stage is free to wrap its join in context of its own: labelling the
// parts would report "config: max_conns must be positive" and lose the "pool:"
// that said where in the config it is.
type configError struct {
	name string
	err  error
}

func makeConfigError(name string, err error) error {
	return configError{name: name, err: err}
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

// checkName rejects an instance name that would not make a sensible prefix or a
// sensible label in errors. One written two ways, or with a space in it, which
// no variable can carry, would silently read nothing.
func checkName(name string) error {
	if len(name) == 0 {
		return errors.New("an instance name is required; use WithName or a non-empty ConfigName")
	}

	for _, r := range name {
		valid := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' || r == '-' || r == '.'
		if !valid {
			return fmt.Errorf(
				"instance name %q may hold only lowercase letters, digits, and _ - . (found %q)",
				name, r,
			)
		}
	}

	if strings.ContainsAny(name[:1]+name[len(name)-1:], "_-.") {
		return fmt.Errorf("instance name %q may not start or end with a separator", name)
	}

	return nil
}

// checkPrefix rejects a prefix that would not read the variables it is meant to.
// An empty one would claim every variable in the environment and could never be
// checked for typos; one that does not end in an underscore would run into the
// field's own name, turning HOST into REPORTINGHOST.
func checkPrefix(prefix string) error {
	if len(prefix) == 0 {
		return errors.New("an environment prefix is required; every variable an application reads is prefixed")
	}

	for _, r := range prefix {
		if valid := (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_'; !valid {
			return fmt.Errorf(
				"environment prefix %q may hold only upper-case letters, digits, and _ (found %q)",
				prefix, r,
			)
		}
	}

	if !strings.HasSuffix(prefix, "_") {
		return fmt.Errorf("environment prefix %q must end with _, which separates it from the field's own name", prefix)
	}

	return nil
}

// defaultPrefix turns an instance name into an env prefix. A variable is written
// with underscores whatever the name uses, so "main" -> "MAIN_",
// "read-replica" -> "READ_REPLICA_", "db.main" -> "DB_MAIN_".
//
// The name is walked a byte at a time because checkName has already established
// its alphabet: lower-case letters, digits, and the three separators. That makes
// the mapping the whole rule rather than a replacer and a case fold, neither of
// which could say what a name is allowed to hold.
func defaultPrefix(name string) string {
	prefix := make([]byte, len(name)+1)

	for i := range len(name) {
		switch c := name[i]; {
		case c >= 'a' && c <= 'z':
			prefix[i] = c - ('a' - 'A')
		case c == '-' || c == '.':
			prefix[i] = '_'
		default:
			prefix[i] = c
		}
	}

	prefix[len(name)] = '_'

	return string(prefix)
}
