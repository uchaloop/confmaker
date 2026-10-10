package main

import (
	"context"
	"fmt"
	"maps"

	"github.com/caarlos0/env/v11"
	"github.com/uchaloop/confmaker/v2"
)

// makeEngine copies complete environments per registration. No process ENV or
// built-in confmaker prefixes are used. Backend errors may contain values.
func makeEngine(environments map[string]map[string]string) confmaker.Engine {
	snapshot := make(map[string]map[string]string, len(environments))
	for name, values := range environments {
		snapshot[name] = make(map[string]string, len(values))
		maps.Copy(snapshot[name], values)
	}

	return confmaker.EngineFunc(func(ctx context.Context, req confmaker.LoadRequest) error {
		if err := ctx.Err(); err != nil {
			return err
		}

		values, ok := snapshot[req.Name]
		if !ok {
			return fmt.Errorf("no environment for registration %q", req.Name)
		}

		return env.ParseWithOptions(req.Target, env.Options{
			Environment: maps.Clone(values),
		})
	})
}
