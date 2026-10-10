package main

import (
	"context"
	"errors"
	"testing"

	"github.com/uchaloop/confmaker/v2"
)

func TestEngine(t *testing.T) {
	documents := map[string][]byte{
		"primary": append([]byte(nil), primaryDocument...),
		"replica": append([]byte(nil), replicaDocument...),
	}
	engine := makeEngine(documents)

	// Neither the caller's map nor its byte slices remain owned by the engine.
	documents["primary"][0] = '!'
	delete(documents, "replica")

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
		t.Fatal("expected configuration-level report")
	}

	if _, err := loader.Manifest(); !errors.Is(err, confmaker.ErrManifestUnsupported) {
		t.Fatalf("manifest error: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := engine.Load(ctx, confmaker.LoadRequest{Name: "primary", Target: &config{}}); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation: %v", err)
	}
}

func TestFailuresWithholdValues(t *testing.T) {
	for _, raw := range []string{"listen_port: invalid\n", "listen_port: -1\n"} {
		documents := map[string][]byte{"primary": primaryDocument, "replica": []byte(raw)}
		loader := confmaker.MakeLoader(confmaker.WithEngine(makeEngine(documents)))
		good := loader.Register[config]("primary")
		loader.Register[config]("replica")

		if err := loader.Load(); err == nil {
			t.Fatal("expected decoding or validation error")
		}

		if _, err := good.Value(); err == nil {
			t.Fatal("failed load exposed a value")
		}
	}

	documents := map[string][]byte{}
	if _, err := confmaker.Load[config]("missing", confmaker.WithEngine(makeEngine(documents))); err == nil {
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

func TestUnknownField(t *testing.T) {
	engine := makeEngine(map[string][]byte{"primary": []byte("listen_port: 8080\nunexpected: true\n")})
	if _, err := confmaker.Load[config]("primary", confmaker.WithEngine(engine)); err == nil {
		t.Fatal("unknown field accepted")
	}
}
