package main

import (
	"bytes"
	"context"
	"fmt"

	"github.com/knadh/koanf/parsers/toml/v2"
	"github.com/knadh/koanf/providers/rawbytes"
	"github.com/knadh/koanf/v2"
	"github.com/uchaloop/confmaker/v2"
)

// makeEngine snapshots documents by registration name. A decoder is created per
// call, so mutable parser state is never shared between loaders.
// Decoder errors are returned unchanged and may contain configuration values.
func makeEngine(documents map[string][]byte, overrides []byte) confmaker.Engine {
	snapshot := make(map[string][]byte, len(documents))
	for name, document := range documents {
		snapshot[name] = bytes.Clone(document)
	}

	overlay := bytes.Clone(overrides)

	return confmaker.EngineFunc(func(ctx context.Context, req confmaker.LoadRequest) error {
		if err := ctx.Err(); err != nil {
			return err
		}

		document, ok := snapshot[req.Name]
		if !ok {
			return fmt.Errorf("no document for registration %q", req.Name)
		}

		settings := koanf.New(".")
		if err := settings.Load(rawbytes.Provider(document), toml.Parser()); err != nil {
			return err
		}

		// Later sources override earlier keys according to koanf's merge rules.
		if len(overlay) != 0 {
			if err := settings.Load(rawbytes.Provider(overlay), toml.Parser()); err != nil {
				return err
			}
		}

		return settings.Unmarshal("", req.Target)
	})
}
