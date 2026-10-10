package confexport

import (
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"io"

	c "github.com/uchaloop/confmaker/v2"
)

type configManifestJSON struct {
	InstanceName string         `json:"instanceName"`
	Prefix       string         `json:"prefix"`
	Variables    []variableJSON `json:"variables"`
}

type variableJSON struct {
	Name            string      `json:"name"`
	Description     string      `json:"description"`
	Type            string      `json:"type"`
	Required        bool        `json:"required"`
	NotEmpty        bool        `json:"notEmpty"`
	Secret          bool        `json:"secret"`
	Default         defaultJSON `json:"default"`
	FieldPath       string      `json:"fieldPath"`
	Format          string      `json:"format"`
	Separator       string      `json:"separator,omitzero"`
	KeyValSeparator string      `json:"keyValSeparator,omitzero"`
}

// WriteJSON writes version 2 JSON from a prepared manifest. It preserves default
// states and render problems without running user methods. Encoding failures
// leave the writer untouched; writer errors may leave partial output.
func WriteJSON(writer io.Writer, configs c.ManifestResult) error {
	// Keep the one-use document envelope local to its writer.
	document := struct {
		Version  int                  `json:"version"`
		Configs  []configManifestJSON `json:"configs"`
		Problems []problemJSON        `json:"problems"`
	}{
		Version: 2,
		Configs: make([]configManifestJSON, 0, len(configs.Configs)),
	}

	for _, config := range configs.Configs {
		entry := configManifestJSON{
			InstanceName: config.InstanceName,
			Prefix:       config.Prefix,
			Variables:    make([]variableJSON, 0, len(config.Variables)),
		}

		for _, variable := range config.Variables {
			field := variableJSON{
				Name:            variable.Name,
				Description:     variable.Description,
				Type:            variable.Type,
				Required:        variable.Required,
				NotEmpty:        variable.NotEmpty,
				Secret:          variable.Secret,
				Default:         defaultJSON{State: variable.Default.State},
				FieldPath:       variable.FieldPath,
				Format:          variable.Format,
				Separator:       variable.Separator,
				KeyValSeparator: variable.KeyValSeparator,
			}

			if !variable.Secret && variable.Default.State == c.DefaultRendered {
				field.Default.Text = &variable.Default.Text
			}

			entry.Variables = append(entry.Variables, field)
		}

		document.Configs = append(document.Configs, entry)
	}

	for _, p := range configs.Problems {
		document.Problems = append(document.Problems, problemJSON{p.Kind, p.InstanceName, p.VariableName, p.FieldPath})
	}

	data, err := json.Marshal(document, jsontext.WithIndent("  "))
	if err != nil {
		return err
	}

	data = append(data, '\n')
	n, err := writer.Write(data)
	if err != nil {
		return err
	}

	if n != len(data) {
		return io.ErrShortWrite
	}

	return nil
}

type defaultJSON struct {
	State c.DefaultState `json:"state"`
	Text  *string        `json:"text,omitzero"`
}

type problemJSON struct {
	Kind         c.ErrorKind `json:"kind"`
	InstanceName string      `json:"instanceName"`
	VariableName string      `json:"variableName"`
	FieldPath    string      `json:"fieldPath"`
}
