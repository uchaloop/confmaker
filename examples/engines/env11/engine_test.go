package main

import (
	"context"
	"errors"
	"testing"

	"github.com/uchaloop/confmaker/v2"
)

func TestNativeDefaultsAndSnapshot(t *testing.T) {
	t.Setenv("PORT", "9999")
	values := map[string]map[string]string{"primary": nil, "replica": {"PORT": "9090"}}
	engine := makeEngine(values)
	values["replica"]["PORT"] = "invalid"
	delete(values, "primary")

	loader := confmaker.MakeLoader(confmaker.WithEngine(engine), confmaker.WithDiagnostics())
	primary := loader.Register[config]("primary")
	replica := loader.Register[config]("replica")

	if err := loader.Load(); err != nil {
		t.Fatal(err)
	}

	for i, handle := range []*confmaker.Handle[config]{primary, replica} {
		cfg, err := handle.Value()
		if err != nil || cfg.Port != []int{8080, 9090}[i] || cfg.Label != "local" {
			t.Fatalf("unexpected configuration: %+v, %v", cfg, err)
		}
	}

	if loader.Report().DetailLevel != confmaker.DetailConfig {
		t.Fatal("expected config-level report")
	}

	if _, err := loader.Manifest(); !errors.Is(err, confmaker.ErrManifestUnsupported) {
		t.Fatalf("manifest: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := engine.Load(ctx, confmaker.LoadRequest{Name: "primary", Target: &config{}}); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation: %v", err)
	}
}

func TestFailuresWithholdValues(t *testing.T) {
	for _, raw := range []string{"invalid", "-1"} {
		engine := makeEngine(map[string]map[string]string{
			"primary": {},
			"replica": {"PORT": raw},
		})
		loader := confmaker.MakeLoader(confmaker.WithEngine(engine))
		good := loader.Register[config]("primary")
		loader.Register[config]("replica")

		if err := loader.Load(); err == nil {
			t.Fatal("expected decoding or validation error")
		}

		if _, err := good.Value(); err == nil {
			t.Fatal("failed load exposed a value")
		}
	}

	if _, err := confmaker.Load[config]("missing", confmaker.WithEngine(makeEngine(nil))); err == nil {
		t.Fatal("unknown registration accepted")
	}
}

func Example() {
	main()

	// Output:
	// primary 8080 local
	// replica 9090 local
	// config
}
