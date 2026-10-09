package confexport

import (
	"fmt"
	c "github.com/uchaloop/confmaker/v2"
	"io"
	"strings"
)

// WriteEnvExample writes a prepared manifest as a reference ENV template.
// Safe rendered text becomes an assignment; other values become placeholders.
// Sensitive values are omitted. No user code executes; writer errors may leave
// partial output. This is not a promise of compatibility with every dotenv parser.
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
