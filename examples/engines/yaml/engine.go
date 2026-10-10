package main

import (
	"bytes"
	"context"
	"fmt"
	"io"

	"github.com/uchaloop/confmaker/v2"
	"go.yaml.in/yaml/v3"
)

// makeEngine snapshots documents by registration name. A decoder is created per
// call, so mutable parser state is never shared between loaders.
// Decoder errors are returned unchanged and may contain configuration values.
func makeEngine(documents map[string][]byte) confmaker.Engine {
	snapshot := make(map[string][]byte, len(documents))
	for name, document := range documents {
		snapshot[name] = bytes.Clone(document)
	}

	return confmaker.EngineFunc(func(ctx context.Context, req confmaker.LoadRequest) error {
		if err := ctx.Err(); err != nil {
			return err
		}

		document, ok := snapshot[req.Name]
		if !ok {
			return fmt.Errorf("no document for registration %q", req.Name)
		}

		decoder := yaml.NewDecoder(bytes.NewReader(document))
		decoder.KnownFields(true)

		if err := decoder.Decode(req.Target); err != nil {
			return err
		}

		// Reject trailing documents rather than silently ignoring them.
		var extra any
		if err := decoder.Decode(&extra); err != io.EOF {
			if err != nil {
				return err
			}

			return fmt.Errorf("expected one YAML document")
		}

		return nil
	})
}
