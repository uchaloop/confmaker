package confmaker

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

type sensitiveText string

func (*sensitiveText) IsSensitive() {}
func (s *sensitiveText) UnmarshalText(b []byte) error {
	return fmt.Errorf("do-not-disclose: %w", errSensitiveInput)
}

var errSensitiveInput = errors.New("sensitive cause")

func TestSensitiveTagAndMarker(t *testing.T) {
	type config struct {
		Password sensitiveText `env:"PASSWORD,secret"`
	}
	_, err := Load[config]("app", WithEnv(map[string]string{"APP_PASSWORD": "do-not-disclose"}))
	if err == nil || strings.Contains(err.Error(), "do-not-disclose") || errors.Is(err, errSensitiveInput) {
		t.Fatalf("unsafe error: %v", err)
	}
	problems := ConfigErrors(err)
	if len(problems) != 1 || problems[0].Kind != ErrorParse {
		t.Fatalf("expected sanitized parse error: %v", err)
	}
}
func TestPointerMarkerInMap(t *testing.T) {
	type config struct {
		Tokens map[string]sensitiveText `env:"TOKENS" envFormat:"json"`
	}
	_, err := Load[config]("app", WithEnv(map[string]string{"APP_TOKENS": `{"key":"do-not-disclose"}`}))
	if err == nil || strings.Contains(err.Error(), "do-not-disclose") || errors.Is(err, errSensitiveInput) {
		t.Fatalf("unsafe error: %v", err)
	}
	if p := ConfigErrors(err); len(p) != 1 || p[0].Kind != ErrorParse {
		t.Fatalf("expected parse error: %v", err)
	}
}
func TestSupportedPointerBoundary(t *testing.T) {
	type config struct {
		Value **int `env:"VALUE"`
	}
	_, err := Load[config]("app", WithEnv(nil))
	if err == nil {
		t.Fatal("pointer chains must be rejected")
	}
}

func TestLoaderReportStates(t *testing.T) {
	disabled := MakeLoader(WithEnv(nil))
	if disabled.Report().State != LoadDisabled {
		t.Fatal(disabled.Report())
	}
	_ = disabled.Load()
	if disabled.Report().State != LoadDisabled {
		t.Fatal(disabled.Report())
	}
	enabled := MakeLoader(WithEnv(nil), WithDiagnostics(), WithDiagnostics())
	if enabled.Report().State != LoadNotStarted {
		t.Fatal(enabled.Report())
	}
	if err := enabled.Load(); err != nil {
		t.Fatal(err)
	}
	if enabled.Report().State != LoadSucceeded {
		t.Fatal(enabled.Report())
	}
}
func TestLoaderPanicReport(t *testing.T) {
	l := MakeLoader(WithEnv(nil), WithDiagnostics(), WithDiagnosticHandler(func(LoadReport) { t.Error("callback on panic") }))
	l.Register[diagnosticPanics]("app")
	func() {
		defer func() {
			if recover() == nil {
				t.Error("panic swallowed")
			}
		}()
		_ = l.Load()
	}()
	r := l.Report()
	if r.State != LoadPanicked || r.Configs[0].Status != ConfigInterrupted || !errors.Is(l.Load(), ErrLoadPanicked) {
		t.Fatalf("panic result: %+v", r)
	}
}

func TestDiagnosticCallbackPublication(t *testing.T) {
	entered, release, done := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	var l *Loader
	l = MakeLoader(WithEnv(nil), WithDiagnosticHandler(func(r LoadReport) {
		if r.State != LoadSucceeded || l.Report().State != LoadSucceeded || l.Load() != nil {
			t.Error("result not published")
		}
		close(entered)
		<-release
	}))
	go func() { done <- l.Load() }()
	<-entered
	if err := l.Load(); err != nil {
		t.Fatal(err)
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

type panicField string

func (*panicField) UnmarshalText([]byte) error { panic("decode") }
func TestPanicMarksActiveField(t *testing.T) {
	type config struct {
		A string     `env:"A"`
		B panicField `env:"B"`
		C string     `env:"C"`
	}
	l := MakeLoader(WithEnv(map[string]string{"APP_A": "a", "APP_B": "b"}), WithDiagnostics())
	l.Register[config]("app")
	func() {
		defer func() {
			if recover() != "decode" {
				t.Error("wrong panic")
			}
		}()
		_ = l.Load()
	}()
	r := l.Report()
	v := r.Configs[0].Variables
	if r.Configs[0].Status != ConfigInterrupted || v[0].Status != VariableSucceeded || v[1].Status != VariableInterrupted || v[2].Status != VariableNotProcessed {
		t.Fatal(r)
	}
}
