package confmaker

import (
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"io"
)

// manifestJSON is a versioned wire format independent of Go metadata fields.
type manifestJSON struct {
	Version int                  `json:"version"`
	Configs []configManifestJSON `json:"configs"`
}

type configManifestJSON struct {
	InstanceName string         `json:"instanceName"`
	Prefix       string         `json:"prefix"`
	Variables    []variableJSON `json:"variables"`
}

type variableJSON struct {
	Name        string  `json:"name"`
	Description string  `json:"description"`
	Type        string  `json:"type"`
	Required    bool    `json:"required"`
	NotEmpty    bool    `json:"notEmpty"`
	Secret      bool    `json:"secret"`
	Default     *string `json:"default,omitzero"`
	HasDefault  bool    `json:"hasDefault"`
}

// WriteManifestJSON writes a version 1 manifest with fixed camelCase keys,
// two-space indentation and a trailing newline. Empty lists are JSON arrays.
// All variable metadata is included except secret defaults, whose default key
// is omitted. hasDefault retains Manifest's non-zero-value semantics.
//
// It uses Loader.Manifest's snapshot and lifecycle: no ENV is read or config
// loaded. Manifest and encoding errors leave writer untouched. Writer errors
// are returned and may leave partial output. The caller owns file handling.
func (l *Loader) WriteManifestJSON(writer io.Writer) error {
	configs, err := l.Manifest()
	if err != nil {
		return err
	}

	document := manifestJSON{Version: 1, Configs: make([]configManifestJSON, 0, len(configs))}
	for _, config := range configs {
		entry := configManifestJSON{
			InstanceName: config.InstanceName, Prefix: config.Prefix,
			Variables: make([]variableJSON, 0, len(config.Variables)),
		}

		for _, variable := range config.Variables {
			field := variableJSON{
				Name: variable.Name, Description: variable.Description, Type: variable.Type,
				Required: variable.Required, NotEmpty: variable.NotEmpty, Secret: variable.Secret,
				HasDefault: variable.HasDefault,
			}
			if !variable.Secret {
				field.Default = &variable.Default
			}

			entry.Variables = append(entry.Variables, field)
		}

		document.Configs = append(document.Configs, entry)
	}

	data, err := json.Marshal(document, jsontext.WithIndent("  "))
	if err != nil {
		return err
	}

	data = append(data, '\n')
	n, err := writer.Write(data)
	if err == nil && n != len(data) {
		return io.ErrShortWrite
	}

	return err
}
