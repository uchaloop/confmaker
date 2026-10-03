package confmaker

import (
	"errors"
	"fmt"
	"strings"
)

// checkName validates the instance name.
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

// checkPrefix validates an explicit ENV prefix.
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

// defaultPrefix uppercases the name and replaces separators with underscores.
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
