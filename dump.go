package confmaker

import (
	"cmp"
	"fmt"
	"io"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"text/tabwriter"
	"unicode"
)

// dumpDefaults describes each config's defaults and writes the dump. A config
// whose defaults cannot be rendered is left out of the table and reported.
func dumpDefaults(w io.Writer, descriptors []descriptor, configs []any, env environment) []error {
	var errs []error

	sections := make([]dumpSection, 0, len(descriptors))
	for i, d := range descriptors {
		variables, err := describeFields(reflect.ValueOf(configs[i]).Elem(), d.fields)
		if err != nil {
			errs = append(errs, makeConfigError(d.label, err))
			continue
		}

		sections = append(sections, dumpSection{label: d.label, variables: variables})
	}

	if err := writeDump(w, sections, env); err != nil {
		errs = append(errs, fmt.Errorf("write configuration dump: %w", err))
	}

	return errs
}

// writeDump renders the manifest as a table, one section per instance, in the
// order given: Load passes them sorted by instance name. Variables are sorted
// within a section. A secret is reported as set or unset; its value is never
// written. The sections' variables are sorted in place.
func writeDump(w io.Writer, sections []dumpSection, env environment) error {
	table := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)

	fmt.Fprintln(table, "INSTANCE\tVARIABLE\tTYPE\tVALUE\tSOURCE")

	for _, section := range sections {
		slices.SortFunc(section.variables, func(a, b Variable) int { return cmp.Compare(a.Name, b.Name) })

		for _, variable := range section.variables {
			value, source := describeValue(variable, env)
			fmt.Fprintf(table, "%s\t%s\t%s\t%s\t%s\n", dumpCell(section.label), dumpCell(variable.Name), dumpCell(variable.Type), dumpCell(value), source)
		}
	}

	return table.Flush()
}

// dumpSection is one instance's variables in a dump.
type dumpSection struct {
	label     string
	variables []Variable
}

// describeValue returns what to print for a variable and where the value comes
// from.
func describeValue(variable Variable, env environment) (value, source string) {
	if raw, ok := env[variable.Name]; ok {
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

// dumpCell keeps a value on one table row and makes terminal controls visible.
func dumpCell(value string) string {
	if strings.ContainsFunc(value, func(r rune) bool { return !unicode.IsPrint(r) }) {
		return strconv.Quote(value)
	}

	return value
}
