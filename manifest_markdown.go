package confmaker

import (
	"fmt"
	"io"
	"strings"
)

// WriteManifestMarkdown writes a section per config with its prefix and a
// GitHub-flavored Markdown table of variables in manifest order. Columns show
// ENV name, Go type, requirement, secret status, default and description.
// Missing or secret defaults are shown as an em dash. Text is escaped for table
// cells; description and default line breaks become HTML br elements.
//
// It uses Loader.Manifest's snapshot and lifecycle without reading ENV or
// loading configs. Manifest errors leave writer untouched. Writer errors are
// returned and may leave partial output. The caller owns file handling.
func (l *Loader) WriteManifestMarkdown(writer io.Writer) error {
	configs, err := l.Manifest(IncludeDefaults())
	if err != nil {
		return err
	}

	var output strings.Builder
	output.WriteString("# Configuration manifest\n")
	for _, config := range configs.Configs {
		fmt.Fprintf(&output, "\n## %s\n\nPrefix: %s\n\n", escapeManifestMarkdownText(config.InstanceName), manifestMarkdownCode(config.Prefix))
		output.WriteString("| ENV | Type | Requirement | Secret | Default | Description |\n")
		output.WriteString("| --- | --- | --- | --- | --- | --- |\n")
		for _, variable := range config.Variables {
			requirement := "Optional"
			if variable.NotEmpty {
				requirement = "Required, non-empty"
			} else if variable.Required {
				requirement = "Required"
			}

			secret := "No"
			if variable.Secret {
				secret = "Yes"
			}

			defaultText := "—"
			if !variable.Secret && (variable.Default.State == DefaultRendered) {
				defaultText = escapeManifestMarkdownText(variable.Default.Text)
			}

			fmt.Fprintf(&output, "| %s | %s | %s | %s | %s | %s |\n",
				manifestMarkdownCode(variable.Name), manifestMarkdownCode(variable.Type), requirement,
				secret, defaultText, escapeManifestMarkdownText(variable.Description))
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

// escapeManifestMarkdownText keeps user text literal inside headings and table cells.
func escapeManifestMarkdownText(value string) string {
	value = strings.ReplaceAll(value, "\r\n", "\n")
	value = strings.ReplaceAll(value, "\r", "\n")
	var output strings.Builder
	for _, char := range value {
		switch {
		case char == '\n':
			output.WriteString("<br>")
		case char < 32 || char == 127:
			fmt.Fprintf(&output, "&#92;u%04x", char)
		case strings.ContainsRune("&<>|`\\*_[]!#", char):
			fmt.Fprintf(&output, "&#%d;", char)
		default:
			output.WriteRune(char)
		}
	}

	return output.String()
}

// manifestMarkdownCode renders identifiers and types as readable inline code.
// Unusual text falls back to escaped prose so it cannot break a table or span.
func manifestMarkdownCode(value string) string {
	for _, char := range value {
		if char < 32 || char == 127 || char == '`' || char == '|' {
			return escapeManifestMarkdownText(value)
		}
	}

	if len(value) == 0 || strings.TrimSpace(value) != value {
		return escapeManifestMarkdownText(value)
	}

	return "`" + value + "`"
}
