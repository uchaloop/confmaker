package confmaker

import (
	"context"
	"errors"
	"testing"
)

type engineConfig struct {
	Port      int `env:"PORT,strange"`
	Validated bool
}

func (c *engineConfig) SetDefaults() { c.Port = 9000 }
func (c *engineConfig) Validate() error {
	c.Validated = true
	if c.Port < 0 {
		return errors.New("invalid port")
	}
	return nil
}

type nilEngine struct{}

func (*nilEngine) Load(context.Context, LoadRequest) error { panic("nil engine called") }
func TestEngineLifecycle(t *testing.T) {
	cause := errors.New("backend failed")
	calls := 0
	e := EngineFunc(func(_ context.Context, r LoadRequest) error {
		calls++
		c := r.Target.(*engineConfig)
		if c.Port != 9000 {
			t.Fatal("missing defaults")
		}
		c.Port = 42
		if r.Name == "bad" {
			return cause
		}
		return nil
	})
	l := MakeLoader(WithEngine(e))
	h := l.Register[engineConfig]("service/db primary")
	if err := l.Load(); err != nil {
		t.Fatal(err)
	}
	c, err := h.Value()
	if err != nil || c.Port != 42 || !c.Validated {
		t.Fatalf("%+v %v", c, err)
	}
	if err := l.Load(); err != nil || calls != 1 {
		t.Fatalf("calls=%d err=%v", calls, err)
	}
	l = MakeLoader(WithEngine(e))
	h = l.Register[engineConfig]("good")
	l.Register[engineConfig]("bad")
	if err := l.Load(); !errors.Is(err, cause) {
		t.Fatal(err)
	}
	if _, err := h.Value(); !errors.Is(err, cause) {
		t.Fatal(err)
	}
}
func TestEngineRegistration(t *testing.T) {
	calls := 0
	e := EngineFunc(func(context.Context, LoadRequest) error { calls++; return nil })
	for _, kind := range []string{"empty", "duplicate", "scalar"} {
		t.Run(kind, func(t *testing.T) {
			l := MakeLoader(WithEngine(e))
			switch kind {
			case "empty":
				l.Register[engineConfig]("")
			case "duplicate":
				l.Register[engineConfig]("same")
				l.Register[engineConfig]("same")
			case "scalar":
				l.Register[int]("scalar")
			}
			if l.Load() == nil {
				t.Fatal("expected error")
			}
		})
	}
	if calls != 0 {
		t.Fatal(calls)
	}
}
func TestEngineOptions(t *testing.T) {
	e := EngineFunc(func(context.Context, LoadRequest) error { t.Fatal("backend called"); return nil })
	var p *nilEngine
	var f EngineFunc
	cases := [][]LoaderOption{{WithEngine(nil)}, {WithEngine(p)}, {WithEngine(f)}, {WithEngine(e), WithEngine(e)}, {WithEngine(e), WithEnv(nil)}, {WithEnv(nil), WithEngine(e)}, {WithEngine(e), AllowUnknown()}, {AllowUnknown(), WithEngine(e)}}
	for i, opts := range cases {
		l := MakeLoader(opts...)
		if l.Load() == nil {
			t.Fatalf("case %d", i)
		}
	}
	l := MakeLoader(WithEngine(e))
	l.Register[engineConfig]("x", WithPrefix("X_"))
	if l.Load() == nil {
		t.Fatal("prefix accepted")
	}
}
