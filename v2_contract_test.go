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
	_, err := Load[struct {
		Password sensitiveText `env:"PASSWORD,secret"`
	}]("app", WithEnv(map[string]string{"APP_PASSWORD": "do-not-disclose"}))
	if err == nil || strings.Contains(err.Error(), "do-not-disclose") || errors.Is(err, errSensitiveInput) {
		t.Fatalf("unsafe error: %v", err)
	}
	problems := ConfigErrors(err)
	if len(problems) != 1 || problems[0].Kind != ErrorParse {
		t.Fatalf("expected sanitized parse error: %v", err)
	}
}
func TestPointerMarkerInMap(t *testing.T) {
	_, err := Load[struct {
		Tokens map[string]sensitiveText `env:"TOKENS" envFormat:"json"`
	}]("app", WithEnv(map[string]string{"APP_TOKENS": `{"key":"do-not-disclose"}`}))
	if err == nil || strings.Contains(err.Error(), "do-not-disclose") || errors.Is(err, errSensitiveInput) {
		t.Fatalf("unsafe error: %v", err)
	}
	if p := ConfigErrors(err); len(p) != 1 || p[0].Kind != ErrorParse {
		t.Fatalf("expected parse error: %v", err)
	}
}
func TestSupportedPointerBoundary(t *testing.T) {
	_, err := Load[struct {
		Value **int `env:"VALUE"`
	}]("app", WithEnv(nil))
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

func TestManifestSchemaRunsNoUserCode(t *testing.T) {
	l := MakeLoader()
	l.Register[manifestPanicDefaults]("app")
	r, err := l.Manifest()
	if err != nil || len(r.Configs) != 1 || r.Configs[0].Variables[0].Default.State != DefaultNotEvaluated {
		t.Fatalf("schema: %+v %v", r, err)
	}
}

type manifestV2Config struct {
	Broken   brokenText `env:"BROKEN"`
	Zero     int        `env:"ZERO"`
	Password string     `env:"PASSWORD,secret"`
	Good     string     `env:"GOOD"`
}

func (c *manifestV2Config) SetDefaults() {
	c.Broken = 1
	c.Password = "do-not-disclose"
	c.Good = "hello"
}
func TestManifestDefaultStates(t *testing.T) {
	r, err := Manifest[manifestV2Config]("app", IncludeDefaults())
	if err != nil || len(r.Problems) != 1 {
		t.Fatalf("result: %+v %v", r, err)
	}
	v := r.Configs[0].Variables
	if v[0].Default.State != DefaultUnrenderable || v[1].Default.State != DefaultZero || v[2].Default.State != DefaultRedacted || v[3].Default.Text != "hello" {
		t.Fatal(v)
	}
	if strings.Contains(fmt.Sprint(r), "do-not-disclose") {
		t.Fatal("leaked secret")
	}
}

type emptyRendered int

func (*emptyRendered) UnmarshalText([]byte) error  { return nil }
func (emptyRendered) MarshalText() ([]byte, error) { return []byte{}, nil }

type emptyRenderedConfig struct {
	Value emptyRendered `env:"VALUE"`
}

func (c *emptyRenderedConfig) SetDefaults() { c.Value = 1 }
func TestManifestRenderedEmptyText(t *testing.T) {
	r, err := Manifest[emptyRenderedConfig]("app", IncludeDefaults())
	if err != nil || r.Configs[0].Variables[0].Default.State != DefaultRendered || r.Configs[0].Variables[0].Default.Text != "" {
		t.Fatalf("%+v %v", r, err)
	}
}
func TestManifestPanicDoesNotChangeLoad(t *testing.T) {
	l := MakeLoader(WithDiagnostics())
	l.Register[manifestPanicDefaults]("app")
	func() {
		defer func() {
			if recover() == nil {
				t.Error("panic swallowed")
			}
		}()
		_, _ = l.Manifest(IncludeDefaults())
	}()
	if l.Report().State != LoadNotStarted {
		t.Fatal(l.Report())
	}
}

func TestPanicReportPreservesEarlierFieldProblems(t *testing.T) {
	type config struct {
		A int        `env:"A"`
		B panicField `env:"B"`
	}
	l := MakeLoader(WithDiagnostics(), WithEnv(map[string]string{"APP_A": "bad", "APP_B": "panic"}))
	l.Register[config]("app")
	func() { defer func() { _ = recover() }(); _ = l.Load() }()
	r := l.Report()
	if len(r.Problems) != 1 || r.Problems[0].Kind != ErrorParse || r.Problems[0].VariableName != "APP_A" {
		t.Fatalf("lost earlier problem: %+v", r)
	}
	if r.Configs[0].Status != ConfigInterrupted {
		t.Fatalf("interrupted status lost: %+v", r)
	}
}
func TestPanicReportDoesNotOverwriteExecutionStatus(t *testing.T) {
	type untouched struct {
		Value int `env:"VALUE"`
	}
	l := MakeLoader(WithDiagnostics(), WithEnv(map[string]string{"APP_TYPO": "x", "LATER_TYPO": "y"}))
	l.Register[diagnosticPanics]("app")
	l.Register[untouched]("later")
	func() { defer func() { _ = recover() }(); _ = l.Load() }()
	r := l.Report()
	if len(r.Problems) != 2 || r.Configs[0].Status != ConfigInterrupted || r.Configs[1].Status != ConfigNotProcessed {
		t.Fatalf("incorrect panic report: %+v", r)
	}
}

func TestReportReflectsPublishedLoadingState(t *testing.T) {
	// Load publishes its lifecycle under the mutex before allocating detail slices.
	l := MakeLoader(WithDiagnostics())
	l.mu.Lock()
	l.state = loading
	l.mu.Unlock()
	if l.Report().State != LoadInProgress {
		t.Fatal(l.Report())
	}
}
