package main

import (
	"bytes"
	"context"
	"fmt"

	"github.com/pelletier/go-toml/v2"
	"github.com/uchaloop/confmaker/v2"
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

		decoder := toml.NewDecoder(bytes.NewReader(document))
		decoder.DisallowUnknownFields()

		return decoder.Decode(req.Target)
	})
}
