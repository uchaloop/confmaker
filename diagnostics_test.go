package confmaker

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"
)

type diagnosticConfig struct {
	Default  int    `env:"DEFAULT"`
	Zero     bool   `env:"ZERO"`
	Input    int    `env:"INPUT"`
	Required string `env:"REQUIRED,required"`
	Empty    string `env:"EMPTY,notEmpty"`
}

func (c *diagnosticConfig) SetDefaults() { c.Default = 7 }

func TestDiagnosticSourcesAndErrors(t *testing.T) {
	loader := MakeLoader(WithDiagnostics(), WithEnv(map[string]string{"APP_INPUT": "private-invalid-value", "APP_EMPTY": ""}))
	loader.Register[diagnosticConfig]("app")

	err := loader.Load()
	if err == nil {
		t.Fatal("expected load error")
	}

	report := loader.Report()
	if report.State != LoadFailed || len(report.Configs) != 1 || len(report.Problems) != 3 {
		t.Fatal(report)
	}

	config := report.Configs[0]
	if config.Status != ConfigFailed {
		t.Fatal(config)
	}

	sources := map[string]ValueSource{"APP_DEFAULT": SourceDefault, "APP_ZERO": SourceZero, "APP_INPUT": SourceEnv, "APP_REQUIRED": SourceMissing, "APP_EMPTY": SourceEnv}
	for _, field := range config.Variables {
		if field.Source != sources[field.Name] {
			t.Fatal(field)
		}

		expectedStatus := VariableSucceeded
		if field.Name == "APP_INPUT" || field.Name == "APP_REQUIRED" || field.Name == "APP_EMPTY" {
			expectedStatus = VariableFailed
		}

		if field.Status != expectedStatus {
			t.Fatal(field)
		}
	}

	if strings.Contains(fmt.Sprintf("%+v", report), "private-invalid-value") {
		t.Fatal("value leaked")
	}

	report.Configs[0].Variables[0].Name = "changed"
	report.Problems[0].Kind = "changed"
	if reflect.DeepEqual(report, loader.Report()) {
		t.Fatal("report aliases internal data")
	}

	if loader.Load() != err {
		t.Fatal("load error changed")
	}
}

type diagnosticValidation struct {
	Token string `env:"TOKEN"`
}

func (diagnosticValidation) Validate() error { return errors.New("private-validation-value") }

func TestDiagnosticValidation(t *testing.T) {
	var captured LoadReport
	_, err := Load[diagnosticValidation]("app", WithDiagnosticHandler(func(r LoadReport) { captured = r }), WithEnv(map[string]string{"APP_TOKEN": "private-env-value"}))

	report := captured
	if err == nil || report.Configs[0].Status != ConfigFailed || report.Configs[0].Variables[0].Status != VariableSucceeded || report.Problems[0].Kind != ErrorValidation {
		t.Fatal(report, err)
	}

	if strings.Contains(fmt.Sprintf("%+v", report), "private-") {
		t.Fatal("value leaked")
	}
}

func TestDiagnosticHandlerReadsCompletedLoad(t *testing.T) {
	var loader *Loader
	var handle *Handle[struct{}]
	callbackCalls := 0
	loader = MakeLoader(WithDiagnostics(), WithEnv(nil), WithDiagnosticHandler(func(report LoadReport) {
		callbackCalls++
		if report.State != LoadSucceeded || loader.Report().State != LoadSucceeded {
			t.Fatal(report)
		}

		if err := loader.Load(); err != nil {
			t.Fatal(err)
		}

		if _, err := handle.Value(); err != nil {
			t.Fatal(err)
		}

		report.Configs[0].InstanceName = "changed"
	}))
	handle = loader.Register[struct{}]("app")
	if err := loader.Load(); err != nil {
		t.Fatal(err)
	}

	if err := loader.Load(); err != nil {
		t.Fatal(err)
	}

	if callbackCalls != 1 || loader.Report().Configs[0].InstanceName != "app" {
		t.Fatal(callbackCalls, loader.Report())
	}
}

func TestDiagnosticConflictAndInvalidRegistration(t *testing.T) {
	loader := MakeLoader(WithDiagnostics(), WithEnv(nil))
	loader.Register[diagnosticConfig]("app")
	loader.Register[diagnosticConfig]("app")
	loader.Register[int]("invalid")
	if loader.Load() == nil {
		t.Fatal("expected conflict")
	}

	report := loader.Report()
	if len(report.Configs) != 3 {
		t.Fatal(report)
	}

	for _, c := range report.Configs {
		if c.InstanceName == "invalid" && c.Status != ConfigFailed {
			t.Fatal(c)
		}

		for _, v := range c.Variables {
			if v.Status != VariableNotProcessed || v.Source != SourceUnknown {
				t.Fatal(v)
			}
		}
	}
}

func TestDiagnosticOptions(t *testing.T) {
	for _, opts := range [][]EnvOption{{WithDiagnosticHandler(nil)}, {WithDiagnosticHandler(func(LoadReport) {}), WithDiagnosticHandler(func(LoadReport) {})}} {
		if p := ConfigErrors(MakeLoader(opts...).Load()); len(p) == 0 || p[0].Kind != ErrorDeclaration {
			t.Fatal(p)
		}
	}
}

var diagnosticValidationEntered, diagnosticValidationRelease chan struct{}

type diagnosticBlockingConfig struct{}

func (diagnosticBlockingConfig) Validate() error {
	close(diagnosticValidationEntered)
	<-diagnosticValidationRelease

	return nil
}

func TestDiagnosticConcurrentReaders(t *testing.T) {
	loader := MakeLoader(WithDiagnostics(), WithEnv(nil))
	loader.Register[diagnosticBlockingConfig]("app")
	entered, release := make(chan struct{}), make(chan struct{})
	diagnosticValidationEntered, diagnosticValidationRelease = entered, release
	done := make(chan error, 1)
	go func() { done <- loader.Load() }()
	<-entered
	var readers sync.WaitGroup
	for range 8 {
		readers.Go(func() {
			for range 100 {
				if report := loader.Report(); report.State != LoadInProgress || len(report.Configs) != 0 {
					t.Error(report)
				}
			}
		})
	}

	readers.Wait()
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}

	if loader.Report().State != LoadSucceeded {
		t.Fatal(loader.Report())
	}
}

type diagnosticPanics struct{}

func (diagnosticPanics) Validate() error { panic("original panic") }

func TestDiagnosticPanic(t *testing.T) {
	loader := MakeLoader(WithDiagnostics(), WithEnv(nil), WithDiagnosticHandler(func(LoadReport) { t.Error("handler called after panic") }))
	loader.Register[diagnosticPanics]("app")
	func() {
		defer func() {
			if recover() != "original panic" {
				t.Error("panic changed")
			}
		}()
		_ = loader.Load()
	}()
	if loader.Report().State != LoadPanicked || loader.Load() != ErrLoadPanicked {
		t.Fatal(loader.Report())
	}
}

func TestDiagnosticHandlerPanicKeepsResult(t *testing.T) {
	loader := MakeLoader(WithDiagnostics(), WithEnv(nil), WithDiagnosticHandler(func(LoadReport) { panic("handler panic") }))
	func() {
		defer func() {
			if recover() != "handler panic" {
				t.Error("panic changed")
			}
		}()
		_ = loader.Load()
	}()
	if loader.Report().State != LoadSucceeded || loader.Load() != nil {
		t.Fatal(loader.Report())
	}
}

var diagnosticDefaultsCalls int

type diagnosticDefaultsConfig struct{}

func (*diagnosticDefaultsConfig) SetDefaults() { diagnosticDefaultsCalls++ }

func TestDiagnosticsDoNotRepeatProcessing(t *testing.T) {
	loader := MakeLoader(WithDiagnostics(), WithEnv(nil))
	handle := loader.Register[diagnosticDefaultsConfig]("app")

	if _, err := loader.Manifest(); err != nil {
		t.Fatal(err)
	}

	if loader.Report().State != LoadNotStarted {
		t.Fatal("manifest collected load report")
	}

	diagnosticDefaultsCalls = 0
	if err := loader.Load(); err != nil {
		t.Fatal(err)
	}

	if _, err := handle.Value(); err != nil {
		t.Fatal(err)
	}

	if err := loader.Load(); err != nil {
		t.Fatal(err)
	}

	if diagnosticDefaultsCalls != 1 {
		t.Fatal("defaults called", diagnosticDefaultsCalls, "times")
	}
}

func TestDiagnosticsPreserveLoadResult(t *testing.T) {
	env := WithEnv(map[string]string{"APP_REQUIRED": "present", "APP_EMPTY": "present", "APP_INPUT": "42"})
	expected, err := Load[diagnosticConfig]("app", env)
	if err != nil {
		t.Fatal(err)
	}

	var captured LoadReport
	actual, err := Load[diagnosticConfig]("app", env, WithDiagnosticHandler(func(r LoadReport) { captured = r }))
	if err != nil || actual != expected || captured.State != LoadSucceeded {
		t.Fatal(actual, err, captured)
	}
}

func TestDiagnosticPanicPreservesCompletedProblems(t *testing.T) {
	loader := MakeLoader(WithDiagnostics(), WithEnv(nil))
	loader.Register[diagnosticValidation]("first")
	loader.Register[diagnosticPanics]("second")
	func() {
		defer func() { _ = recover() }()

		_ = loader.Load()
	}()

	report := loader.Report()
	if report.State != LoadPanicked || len(report.Problems) != 1 || report.Problems[0].Kind != ErrorValidation {
		t.Fatal(report)
	}
}

func TestDiagnosticHandlerCannotMutatePublishedSlices(t *testing.T) {
	loader := MakeLoader(
		WithEnv(nil),
		WithDiagnostics(),
		WithDiagnosticHandler(func(report LoadReport) {
			report.Configs[0].Variables[0].Name = "changed"
			report.Problems[0].Kind = ErrorConflict
		}),
	)
	loader.Register[diagnosticConfig]("app")

	if err := loader.Load(); err == nil {
		t.Fatal("expected required variable errors")
	}

	report := loader.Report()
	if report.Configs[0].Variables[0].Name != "APP_DEFAULT" || report.Problems[0].Kind != ErrorRequired {
		t.Fatal("handler changed published report", report)
	}
}
