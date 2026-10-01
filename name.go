package confmaker

import (
	"errors"
	"fmt"
	"strings"
)

// checkName rejects an instance name that would not make a sensible prefix or a
// sensible label in errors. One written two ways, or with a space in it, which
// no variable can carry, would silently read nothing.
func checkName(name string) error {
	if len(name) == 0 {
		return errors.New("an instance name is required")
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
