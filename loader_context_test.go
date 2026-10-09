package confmaker

import (
	"context"
	"errors"
	"testing"
)

func TestLoadContextPreCanceled(t *testing.T) {
	calls := 0
	l := MakeLoader(WithEngine(EngineFunc(func(context.Context, LoadRequest) error { calls++; return nil })))
	l.Register[engineConfig]("x")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := l.LoadContext(ctx); !errors.Is(err, context.Canceled) || calls != 0 {
		t.Fatalf("%v %d", err, calls)
	}
	if err := l.Load(); err != nil || calls != 1 {
		t.Fatalf("%v %d", err, calls)
	}
}
func TestLoadContextWaiterCanceled(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	defer close(release)
	l := MakeLoader(WithEngine(EngineFunc(func(context.Context, LoadRequest) error { close(entered); <-release; return nil })))
	h := l.Register[engineConfig]("x")
	result := make(chan error, 1)
	go func() { result <- l.Load() }()
	<-entered
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := l.LoadContext(ctx); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	select {
	case err := <-result:
		t.Fatalf("owner stopped: %v", err)
	default:
	}
	_ = h
}
func TestLoadContextOwnerCanceled(t *testing.T) {
	entered := make(chan struct{})
	calls, callbacks := 0, 0
	l := MakeLoader(WithEngine(EngineFunc(func(ctx context.Context, _ LoadRequest) error {
		calls++
		close(entered)
		<-ctx.Done()
		return ctx.Err()
	})), WithDiagnosticHandler(func(LoadReport) { callbacks++ }))
	h := l.Register[engineConfig]("x")
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() { result <- l.LoadContext(ctx) }()
	<-entered
	cancel()
	if err := <-result; !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if err := l.Load(); !errors.Is(err, context.Canceled) || calls != 1 || callbacks != 1 {
		t.Fatalf("%v %d %d", err, calls, callbacks)
	}
	if _, err := h.Value(); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

type cancelDefaults struct{}

var stageCancel context.CancelFunc

func (*cancelDefaults) SetDefaults() { stageCancel() }

type cancelValidation struct{}

func (*cancelValidation) Validate() error { stageCancel(); return nil }
func TestLoadContextStageBoundaries(t *testing.T) {
	for _, stage := range []string{"defaults", "engine", "validation"} {
		t.Run(stage, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			stageCancel = cancel
			calls := 0
			l := MakeLoader(WithEngine(EngineFunc(func(context.Context, LoadRequest) error {
				calls++
				if stage == "engine" {
					cancel()
				}
				return nil
			})))
			switch stage {
			case "defaults":
				l.Register[cancelDefaults]("a")
			case "validation":
				l.Register[cancelValidation]("a")
			default:
				l.Register[engineConfig]("a")
			}
			h := l.Register[engineConfig]("b")
			if err := l.LoadContext(ctx); !errors.Is(err, context.Canceled) {
				t.Fatal(err)
			}
			if _, err := h.Value(); !errors.Is(err, context.Canceled) {
				t.Fatal(err)
			}
			if stage == "defaults" && calls != 0 || stage != "defaults" && calls != 1 {
				t.Fatal(calls)
			}
		})
	}
}
func TestLoadContextPublishedResult(t *testing.T) {
	l := MakeLoader(WithEngine(EngineFunc(func(context.Context, LoadRequest) error { return nil })))
	l.Register[engineConfig]("x")
	if err := l.Load(); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := l.LoadContext(ctx); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadContext[engineConfig](ctx, "x")
	if !errors.Is(err, context.Canceled) || cfg.Port != 0 {
		t.Fatalf("%+v %v", cfg, err)
	}
}
