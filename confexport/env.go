package confexport

import (
	"fmt"
	c "github.com/uchaloop/confmaker/v2"
	"io"
	"strings"
)

// WriteEnvExample writes a reference .env.example for the same snapshot as
// [Loader.Manifest]. It does not read ENV, load configs or open files.
// Descriptions become comments. Only non-secret, non-zero scalar defaults made
// of ASCII letters, digits and _./:@%+,- are emitted as active assignments.
// Other fields become commented placeholders; complex defaults are not printed.
// Required fields are marked even when they have a default. Zero defaults cannot
// be distinguished from absent defaults and also become placeholders.
//
// Names outside [A-Za-z_][A-Za-z0-9_]* are quoted in comments, without changing
// the names accepted by Load. The output is a reference, not a shell script or
// a promise of compatibility with every dotenv parser.
// Manifest errors leave the writer untouched. Writer errors are returned and
// may leave partial output. Defaults and marshalers have the same lifecycle as
// Manifest. Secret values are never rendered.
func WriteEnvExample(writer io.Writer, manifest c.ManifestResult) error {

	var output strings.Builder
	for _, config := range manifest.Configs {
		fmt.Fprintf(&output, "# Configuration: %q\n", config.InstanceName)

		for _, variable := range config.Variables {
			writeExampleComment(&output, variable.Description)

			if variable.Required {
				if variable.NotEmpty {
					output.WriteString("# Required; must not be empty.\n")
				} else {
					output.WriteString("# Required.\n")
				}
			}

			if variable.Secret {
				output.WriteString("# Secret; supply your own value.\n")
			}

			switch {
			case !isEnvExampleVariableName(variable.Name):
				fmt.Fprintf(&output, "# Variable %q: set through your environment.\n", variable.Name)
			case !variable.Secret && (variable.Default.State == c.DefaultRendered) && isEnvExampleDefaultSafe(variable.Default.Text):
				fmt.Fprintf(&output, "%s=%s\n", variable.Name, variable.Default.Text)
			default:
				fmt.Fprintf(&output, "# %s=\n", variable.Name)
			}

			output.WriteByte('\n')
		}
	}

	n, err := io.WriteString(writer, output.String())
	if err != nil {
		return err
	}

	if n != output.Len() {
		return io.ErrShortWrite
	}

	return nil
}

func writeExampleComment(output *strings.Builder, description string) {
	if len(description) == 0 {
		return
	}

	description = strings.ReplaceAll(description, "\r\n", "\n")
	description = strings.ReplaceAll(description, "\r", "\n")
	for _, line := range strings.Split(description, "\n") {
		output.WriteString("# ")
		for _, char := range line {
			if char < 32 || char == 127 || char == '\u2028' || char == '\u2029' {
				fmt.Fprintf(output, "\\u%04x", char)

				continue
			}

			output.WriteRune(char)
		}

		output.WriteByte('\n')
	}
}

func isEnvExampleVariableName(name string) bool {
	if len(name) == 0 {
		return false
	}

	for i, char := range name {
		if char == '_' || char >= 'A' && char <= 'Z' || char >= 'a' && char <= 'z' || i > 0 && char >= '0' && char <= '9' {
			continue
		}

		return false
	}

	return true
}

func isEnvExampleDefaultSafe(value string) bool {
	if len(value) == 0 {
		return false
	}

	for _, char := range value {
		if char >= 'A' && char <= 'Z' || char >= 'a' && char <= 'z' || char >= '0' && char <= '9' || strings.ContainsRune("_./:@%+,-", char) {
			continue
		}

		return false
	}

	return true
}
