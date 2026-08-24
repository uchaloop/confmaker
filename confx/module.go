package confx

import (
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"text/tabwriter"

	"go.uber.org/fx"
)

// descriptorGroup is the Fx value group every Provide call feeds with the
// manifest of the instance it builds. Module is its only consumer, so an
// application that does not use Module pays nothing for it.
const descriptorGroup = `group:"confx_descriptors"`

// moduleSettings holds the resolved options for Module.
type moduleSettings struct {
	allowed []string
	dump    io.Writer
}

// ModuleOption customizes Module.
type ModuleOption func(*moduleSettings)

// AllowUnknown exempts variables with the given prefixes from the strict check.
// Use it when a deployment shares one environment across several binaries and a
// variable that belongs to a sibling process would otherwise be reported.
//
// An empty prefix is ignored rather than honoured: it matches every variable and
// would turn the check off without saying so.
func AllowUnknown(prefixes ...string) ModuleOption {
	return func(s *moduleSettings) {
		for _, prefix := range prefixes {
			if len(prefix) != 0 {
				s.allowed = append(s.allowed, prefix)
			}
		}
	}
}

// WithDump writes the configuration manifest to w at startup: every variable the
// application reads, its type, and its current value. Secrets are reported as
// set or unset and never printed. Pair it with the application's own flag or
// debug switch.
func WithDump(w io.Writer) ModuleOption {
	return func(s *moduleSettings) {
		s.dump = w
	}
}

// Module checks the environment against the manifest the Provide calls register.
// A variable that starts with a prefix the application owns but matches no field
// fails the start, so a typo such as POSTGRES_HSOT is reported instead of
// silently leaving the field at its default.
//
// Register it before the Provide calls it should cover; the check then runs
// ahead of the constructors and reports the typo rather than the missing value
// it causes. Variables outside the application's own prefixes are never
// examined.
func Module(opts ...ModuleOption) fx.Option {
	set := moduleSettings{}
	for _, opt := range opts {
		opt(&set)
	}

	return fx.Module(
		"confmaker",

		fx.Invoke(
			fx.Annotate(
				func(descriptors []descriptor) error {
					if set.dump != nil {
						if err := writeDump(set.dump, descriptors); err != nil {
							return err
						}
					}

					return checkEnvironment(descriptors, set.allowed)
				},
				fx.ParamTags(descriptorGroup),
			),
		),
	)
}

// checkEnvironment reports variables that start with a registered prefix but
// match no declared field.
func checkEnvironment(descriptors []descriptor, allowed []string) error {
	known := make(map[string]bool)
	prefixes := make([]string, 0, len(descriptors))

	for _, d := range descriptors {
		prefixes = append(prefixes, d.prefix)

		for _, variable := range d.variables {
			known[variable.Name] = true
		}
	}

	var unknown []string

	for _, entry := range os.Environ() {
		name, _, _ := strings.Cut(entry, "=")
		if known[name] || hasAnyPrefix(name, allowed) || !hasAnyPrefix(name, prefixes) {
			continue
		}

		unknown = append(unknown, name)
	}

	if len(unknown) == 0 {
		return nil
	}

	sort.Strings(unknown)

	errs := make([]error, 0, len(unknown))
	for _, name := range unknown {
		errs = append(errs, fmt.Errorf("unknown configuration variable %q%s", name, hint(name, known)))
	}

	return errors.Join(errs...)
}

// hint suggests the declared variable a name most likely misspells, so a typo
// points at its own fix.
func hint(name string, known map[string]bool) string {
	const maxDistance = 3

	best, bestDistance := "", maxDistance+1

	for candidate := range known {
		distance := editDistance(name, candidate)
		// Map iteration is unordered, so ties are broken by name: the same typo
		// has to produce the same message on every run.
		if distance < bestDistance || (distance == bestDistance && candidate < best) {
			best, bestDistance = candidate, distance
		}
	}

	if len(best) == 0 {
		return ""
	}

	return fmt.Sprintf(" (did you mean %q?)", best)
}

// editDistance returns the Levenshtein distance between a and b.
func editDistance(a, b string) int {
	previous := make([]int, len(b)+1)
	current := make([]int, len(b)+1)

	for j := range previous {
		previous[j] = j
	}

	for i := 1; i <= len(a); i++ {
		current[0] = i

		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}

			current[j] = min(previous[j]+1, current[j-1]+1, previous[j-1]+cost)
		}

		previous, current = current, previous
	}

	return previous[len(b)]
}

func hasAnyPrefix(name string, prefixes []string) bool {
	for _, prefix := range prefixes {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}

	return false
}

// writeDump renders the manifest as a table, one section per instance. A secret
// is reported as set or unset; its value is never written.
func writeDump(w io.Writer, descriptors []descriptor) error {
	sorted := make([]descriptor, len(descriptors))
	copy(sorted, descriptors)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].label < sorted[j].label })

	table := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)

	fmt.Fprintln(table, "INSTANCE\tVARIABLE\tTYPE\tVALUE\tSOURCE")

	for _, d := range sorted {
		variables := make([]Variable, len(d.variables))
		copy(variables, d.variables)
		sort.Slice(variables, func(i, j int) bool { return variables[i].Name < variables[j].Name })

		for _, variable := range variables {
			value, source := describeValue(variable)
			fmt.Fprintf(table, "%s\t%s\t%s\t%s\t%s\n", d.label, variable.Name, variable.Type, value, source)
		}
	}

	return table.Flush()
}

// describeValue returns what to print for a variable and where the value comes
// from.
func describeValue(variable Variable) (value, source string) {
	if raw, ok := os.LookupEnv(variable.Name); ok {
		if variable.Secret {
			return "(set)", "env"
		}

		return raw, "env"
	}

	if variable.HasDefault {
		return variable.Default, "default"
	}

	if variable.Required {
		return "(unset)", "required"
	}

	return "(unset)", "zero value"
}
