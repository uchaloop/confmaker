package confx

import (
	"cmp"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
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
// fails the start, so a typo such as STORE_HSOT is reported instead of
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
	if err := checkPrefixesAreDistinct(descriptors); err != nil {
		return err
	}

	known, err := claimedVariables(descriptors)
	if err != nil {
		return err
	}

	prefixes := make([]string, 0, len(descriptors))
	for _, d := range descriptors {
		prefixes = append(prefixes, d.prefix)
	}

	var unknown []string

	for _, entry := range os.Environ() {
		name, _, _ := strings.Cut(entry, "=")

		if _, declared := known[name]; declared {
			continue
		}
		if hasAnyPrefix(name, allowed) || !hasAnyPrefix(name, prefixes) {
			continue
		}

		unknown = append(unknown, name)
	}

	if len(unknown) == 0 {
		return nil
	}

	slices.Sort(unknown)

	errs := make([]error, 0, len(unknown))
	for _, name := range unknown {
		errs = append(errs, fmt.Errorf("unknown configuration variable %q%s", name, hint(name, known)))
	}

	return errors.Join(errs...)
}

// claimedVariables maps every declared variable to the instance that reads it,
// and refuses a name two instances both claim. Within one config a collision is
// caught when the config is bound; between two it takes the whole application in
// view, and it arises the same way - one instance's prefix running into
// another's, so "db" reading MAIN_HOST and "db_main" reading HOST both arrive at
// DB_MAIN_HOST.
//
// One value would fill two configs that nothing keeps in step, and a misspelling
// could not be attributed to either. A value two libraries genuinely share is one
// config, provided once and injected wherever it is needed.
func claimedVariables(descriptors []descriptor) (map[string]string, error) {
	claimed := make(map[string]string)

	var errs []error

	for _, d := range descriptors {
		for _, variable := range d.variables {
			switch first, taken := claimed[variable.Name]; {
			case !taken:
				claimed[variable.Name] = d.label
			case first != d.label:
				errs = append(errs, fmt.Errorf(
					"variable %q is read by both %q and %q; provide one config and inject it where both need it",
					variable.Name, first, d.label,
				))
			}
		}
	}

	return claimed, errors.Join(errs...)
}

// checkPrefixesAreDistinct refuses two instances reading one prefix. The scan
// accepts a variable that any instance declares, so instances sharing a prefix
// would cover for each other's typos: a name meant for one and misspelled into
// the other's would pass unnoticed.
//
// A name reaches its prefix through the separators it uses - read-replica,
// read_replica and read.replica all read READ_REPLICA_ - so two names that look
// distinct can arrive at one.
func checkPrefixesAreDistinct(descriptors []descriptor) error {
	owner := make(map[string]string, len(descriptors))

	var errs []error

	for _, d := range descriptors {
		switch first, taken := owner[d.prefix]; {
		case !taken:
			owner[d.prefix] = d.label
		case first != d.label:
			errs = append(errs, fmt.Errorf(
				"instances %q and %q both read the prefix %q; one of them cannot be checked for typos",
				first, d.label, d.prefix,
			))
		}
	}

	return errors.Join(errs...)
}

// maxHintDistance is how far a suggestion may sit from the name it explains.
// Three edits is a typo; further away is a different name, and pointing at it
// would send a deployment to fix the wrong variable.
const maxHintDistance = 3

// hint suggests the declared variable a name most likely misspells, so a typo
// points at its own fix.
func hint(name string, known map[string]string) string {
	var (
		rows         editRows
		best         string
		bestDistance = maxHintDistance + 1
	)

	for candidate := range known {
		// The budget stays the same for every candidate rather than tightening
		// to the best distance so far: a scan that stops early reports the
		// budget rather than the real distance, and tie-breaking on that number
		// would let an unrelated name displace a close one.
		distance := rows.editDistance(name, candidate, maxHintDistance)
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

// editRows carries the two rows an edit distance is computed over. One scan
// compares a name against every variable the application declares, and the rows
// are the same size throughout, so they are filled again rather than allocated
// again.
type editRows struct {
	previous, current []int
}

// editDistance returns the Levenshtein distance between a and b, or limit+1 once
// it is certain the distance is over the limit.
//
// Every edit shifts a position by one, so only a band of width 2*limit+1 around
// the diagonal can hold a path within budget; outside it the cells are filled
// with the sentinel rather than computed. A row whose every cell is over budget
// ends the scan, because no later row can bring the total back down.
func (e *editRows) editDistance(a, b string, limit int) int {
	if len(a) < len(b) {
		a, b = b, a
	}
	// One edit changes the length by at most one, so a difference wider than the
	// budget settles it without filling a single row.
	if len(a)-len(b) > limit {
		return limit + 1
	}

	e.resize(len(b) + 1)

	previous, current := e.previous, e.current
	for j := range previous {
		previous[j] = j
	}

	for i := 1; i <= len(a); i++ {
		current[0] = i

		first, last := max(1, i-limit), min(len(b), i+limit)
		for j := 1; j < first; j++ {
			current[j] = limit + 1
		}

		for j := last + 1; j <= len(b); j++ {
			current[j] = limit + 1
		}

		lowest := current[0]

		for j := first; j <= last; j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}

			current[j] = min(previous[j]+1, current[j-1]+1, previous[j-1]+cost)
			lowest = min(lowest, current[j])
		}

		if lowest > limit {
			return limit + 1
		}

		previous, current = current, previous
	}

	// A scan that reached the end still reports the budget rather than a
	// distance beyond it, so every candidate out of reach compares equal.
	return min(previous[len(b)], limit+1)
}

func (e *editRows) resize(size int) {
	if cap(e.previous) < size {
		e.previous = make([]int, size)
		e.current = make([]int, size)
	}

	e.previous = e.previous[:size]
	e.current = e.current[:size]
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
	sorted := slices.Clone(descriptors)
	slices.SortFunc(sorted, func(a, b descriptor) int { return cmp.Compare(a.label, b.label) })

	table := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)

	fmt.Fprintln(table, "INSTANCE\tVARIABLE\tTYPE\tVALUE\tSOURCE")

	for _, d := range sorted {
		variables := slices.Clone(d.variables)
		slices.SortFunc(variables, func(a, b Variable) int { return cmp.Compare(a.Name, b.Name) })

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
