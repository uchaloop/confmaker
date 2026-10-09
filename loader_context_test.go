package confmaker

import (
	"context"
	"errors"
	"fmt"
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
	defer func() {
		select {
		case <-release:
		default:
			close(release)
		}
	}()
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
	close(release)
	if err := <-result; err != nil {
		t.Fatal(err)
	}
	if _, err := h.Value(); err != nil {
		t.Fatal(err)
	}
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

func TestLoadContextRetainsEarlierErrors(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cause := errors.New("earlier backend error")
	l := MakeLoader(WithEngine(EngineFunc(func(_ context.Context, r LoadRequest) error {
		if r.Name == "a" {
			return cause
		}
		cancel()
		return nil
	})), WithDiagnostics())
	for _, n := range []string{"a", "b", "c"} {
		l.Register[engineConfig](n)
	}
	err := l.LoadContext(ctx)
	if !errors.Is(err, cause) || !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	r := l.Report()
	if r.State != LoadFailed || r.Configs[0].Status != ConfigFailed || r.Configs[1].Status != ConfigInterrupted || r.Configs[2].Status != ConfigNotProcessed {
		t.Fatalf("%+v", r)
	}
}
func TestLoadContextBuiltinCanceledBeforeValidate(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stageCancel = cancel
	l := MakeLoader(WithEnv(nil), WithDiagnostics())
	h := l.Register[cancelDefaults]("a")
	l.Register[struct{}]("b")
	if err := l.LoadContext(ctx); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if _, err := h.Value(); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if l.Report().State != LoadFailed {
		t.Fatal(l.Report())
	}
}

func TestLoadContextCancellationReportBoundaries(t *testing.T) {
	for _, builtin := range []bool{false, true} {
		for _, stage := range []string{"defaults", "validation"} {
			t.Run(fmt.Sprintf("builtin=%v/%s", builtin, stage), func(t *testing.T) {
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				stageCancel = cancel
				opts := []LoaderOption{WithDiagnostics()}
				if builtin {
					opts = append(opts, WithEnv(nil))
				} else {
					opts = append(opts, WithEngine(EngineFunc(func(context.Context, LoadRequest) error { return nil })))
				}
				l := MakeLoader(opts...)
				if stage == "defaults" {
					l.Register[cancelDefaults]("a")
				} else {
					l.Register[cancelValidation]("a")
				}
				l.Register[struct{}]("b")
				if err := l.LoadContext(ctx); !errors.Is(err, context.Canceled) {
					t.Fatal(err)
				}
				want := ConfigInterrupted
				if stage == "validation" {
					want = ConfigSucceeded
				}
				r := l.Report()
				if r.Configs[0].Status != want || r.Configs[1].Status != ConfigNotProcessed {
					t.Fatalf("%+v", r)
				}
			})
		}
	}
}
