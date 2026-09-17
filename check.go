package confmaker

import (
	"errors"
	"fmt"
	"slices"
	"strings"
)

// checkRegistrations refuses instances that share a name, a prefix or a variable,
// and returns every declared variable mapped to the instance that reads it.
func checkRegistrations(descriptors []descriptor) (map[string]string, error) {
	if err := checkPrefixesAreDistinct(descriptors); err != nil {
		return nil, err
	}

	return claimedVariables(descriptors)
}

// checkUnknown reports variables under a registered prefix that no field declares.
func checkUnknown(descriptors []descriptor, known map[string]string, allowed []string, env environment) error {
	prefixes := make([]string, 0, len(descriptors))
	for _, d := range descriptors {
		prefixes = append(prefixes, d.prefix)
	}

	var unknown []string

	for name := range env {
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
// caught when the schema is compiled; between two it takes the whole application in
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
		for _, variable := range d.fields {
			switch first, taken := claimed[variable.Name]; {
			case !taken:
				claimed[variable.Name] = d.label
			default:
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
	names := make(map[string]struct{}, len(descriptors))
	owner := make(map[string]string, len(descriptors))

	var errs []error

	for _, d := range descriptors {
		if _, exists := names[d.label]; exists {
			errs = append(errs, fmt.Errorf("instance name %q is registered more than once; give each config a distinct name", d.label))
		}

		names[d.label] = struct{}{}
		switch first, taken := owner[d.prefix]; {
		case !taken:
			owner[d.prefix] = d.label
		default:
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
// the diagonal can hold a path within budget. Only the cells bordering the band
// are set to the sentinel, so a row costs the band rather than the whole width.
// A row whose every cell is over budget ends the scan, because no later row can
// bring the total back down.
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
		// The next row reads at most one cell past each edge of this band, so
		// those two cells are the only ones outside it that must hold the
		// sentinel; the rest are never read.
		if first > 1 {
			current[first-1] = limit + 1
		}

		if last < len(b) {
			current[last+1] = limit + 1
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
