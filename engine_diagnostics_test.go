package confmaker

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func TestEngineReport(t *testing.T) {
	cause := errors.New("synthetic-sensitive-input")
	var l *Loader
	e := EngineFunc(func(context.Context, LoadRequest) error {
		r := l.Report()
		if r.State != LoadInProgress || r.DetailLevel != "" || len(r.Configs) != 0 {
			t.Fatalf("%+v", r)
		}
		return cause
	})
	l = MakeLoader(WithEngine(e))
	if r := l.Report(); r.State != LoadDisabled || r.DetailLevel != "" {
		t.Fatal(r)
	}
	l = MakeLoader(WithEngine(e), WithDiagnostics())
	l.Register[engineConfig]("service/db")
	if r := l.Report(); r.State != LoadNotStarted || r.DetailLevel != "" {
		t.Fatal(r)
	}
	if err := l.Load(); !errors.Is(err, cause) {
		t.Fatal(err)
	}
	r := l.Report()
	if r.State != LoadFailed || r.DetailLevel != DetailConfig || len(r.Configs) != 1 || r.Configs[0].InstanceName != "service/db" || r.Configs[0].Type == "" || r.Configs[0].Status != ConfigFailed || r.Configs[0].Prefix != "" || len(r.Configs[0].Variables) != 0 {
		t.Fatalf("%+v", r)
	}
	if strings.Contains(fmt.Sprint(r), cause.Error()) {
		t.Fatal("error text leaked")
	}
	r.Configs[0].InstanceName = "mutated"
	if l.Report().Configs[0].InstanceName != "service/db" {
		t.Fatal("shared report")
	}
	builtin := MakeLoader(WithEnv(nil), WithDiagnostics())
	if err := builtin.Load(); err != nil {
		t.Fatal(err)
	}
	if builtin.Report().DetailLevel != DetailField {
		t.Fatal(builtin.Report())
	}
}
func TestEnginePanicReport(t *testing.T) {
	callbacks := 0
	token := new(int)
	e := EngineFunc(func(_ context.Context, r LoadRequest) error {
		if r.Name == "b" {
			panic(token)
		}
		return nil
	})
	l := MakeLoader(WithEngine(e), WithDiagnosticHandler(func(LoadReport) { callbacks++ }))
	for _, n := range []string{"c", "b", "a"} {
		l.Register[engineConfig](n)
	}
	func() {
		defer func() {
			if recover() != token {
				t.Fatal("wrong panic")
			}
		}()
		_ = l.Load()
	}()
	r := l.Report()
	if r.State != LoadPanicked || callbacks != 0 || l.Load() != ErrLoadPanicked {
		t.Fatal(r)
	}
	got := []ConfigStatus{r.Configs[0].Status, r.Configs[1].Status, r.Configs[2].Status}
	if !reflect.DeepEqual(got, []ConfigStatus{ConfigSucceeded, ConfigInterrupted, ConfigNotProcessed}) {
		t.Fatal(got)
	}
}
func TestEngineCallbackPublished(t *testing.T) {
	var l *Loader
	var h *Handle[engineConfig]
	token := new(int)
	l = MakeLoader(WithEngine(EngineFunc(func(context.Context, LoadRequest) error { return nil })), WithDiagnosticHandler(func(r LoadReport) {
		if l.Load() != nil || l.Report().State != LoadSucceeded {
			t.Fatal(r)
		}
		if _, err := h.Value(); err != nil {
			t.Fatal(err)
		}
		panic(token)
	}))
	h = l.Register[engineConfig]("x")
	func() {
		defer func() {
			if recover() != token {
				t.Fatal("wrong panic")
			}
		}()
		_ = l.Load()
	}()
	if l.Load() != nil || l.Report().State != LoadSucceeded {
		t.Fatal(l.Report())
	}
}
func TestEngineManifestUnsupported(t *testing.T) {
	l := MakeLoader(WithEngine(EngineFunc(func(context.Context, LoadRequest) error { t.Fatal("backend called"); return nil })))
	for _, registered := range []bool{false, true} {
		if registered {
			l.Register[cancelDefaults]("x")
		}
		for _, opts := range [][]ManifestOption{nil, {IncludeDefaults()}} {
			r, err := l.Manifest(opts...)
			if !errors.Is(err, ErrManifestUnsupported) || !reflect.DeepEqual(r, ManifestResult{}) {
				t.Fatalf("%+v %v", r, err)
			}
		}
	}
}
