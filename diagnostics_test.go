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
	diagnostics := MakeDiagnostics()
	if diagnostics.Report().State != LoadNotStarted {
		t.Fatal(diagnostics.Report())
	}

	loader := MakeLoader(WithDiagnostics(diagnostics), WithEnv(map[string]string{"APP_INPUT": "private-invalid-value", "APP_EMPTY": ""}))
	loader.Register[diagnosticConfig]("app")

	err := loader.Load()
	if err == nil {
		t.Fatal("expected load error")
	}

	report := diagnostics.Report()
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
	if reflect.DeepEqual(report, diagnostics.Report()) {
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
	diagnostics := MakeDiagnostics()
	_, err := Load[diagnosticValidation]("app", WithDiagnostics(diagnostics), WithEnv(map[string]string{"APP_TOKEN": "private-env-value"}))

	report := diagnostics.Report()
	if err == nil || report.Configs[0].Status != ConfigFailed || report.Configs[0].Variables[0].Status != VariableSucceeded || report.Problems[0].Kind != ErrorValidation {
		t.Fatal(report, err)
	}

	if strings.Contains(fmt.Sprintf("%+v", report), "private-") {
		t.Fatal("value leaked")
	}
}

func TestDiagnosticHandlerReadsCompletedLoad(t *testing.T) {
	diagnostics := MakeDiagnostics()
	var loader *Loader
	var handle *Handle[struct{}]
	callbackCalls := 0
	loader = MakeLoader(WithDiagnostics(diagnostics), WithEnv(nil), WithDiagnosticHandler(func(report LoadReport) {
		callbackCalls++
		if report.State != LoadSucceeded || diagnostics.Report().State != LoadSucceeded {
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

	if callbackCalls != 1 || diagnostics.Report().Configs[0].InstanceName != "app" {
		t.Fatal(callbackCalls, diagnostics.Report())
	}
}

func TestDiagnosticConflictAndInvalidRegistration(t *testing.T) {
	diagnostics := MakeDiagnostics()
	loader := MakeLoader(WithDiagnostics(diagnostics), WithEnv(nil))
	loader.Register[diagnosticConfig]("app")
	loader.Register[diagnosticConfig]("app")
	loader.Register[int]("invalid")
	if loader.Load() == nil {
		t.Fatal("expected conflict")
	}

	report := diagnostics.Report()
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
	diagnostics := MakeDiagnostics()
	owner := MakeLoader(WithDiagnostics(diagnostics))
	for _, opts := range [][]EnvOption{
		{WithDiagnostics(nil)},
		{WithDiagnostics(diagnostics)},
		{WithDiagnostics(MakeDiagnostics()), WithDiagnostics(MakeDiagnostics())},
		{WithDiagnosticHandler(nil)},
		{WithDiagnosticHandler(func(LoadReport) {}), WithDiagnosticHandler(func(LoadReport) {})},
	} {
		loader := MakeLoader(opts...)
		problems := ConfigErrors(loader.Load())
		if len(problems) == 0 || problems[0].Kind != ErrorDeclaration {
			t.Fatal(problems)
		}
	}

	if diagnostics.Report().State != LoadNotStarted {
		t.Fatal("foreign load changed report")
	}

	if err := owner.Load(); err != nil {
		t.Fatal(err)
	}
}

type diagnosticBlockingConfig struct {
	Entered chan struct{}
	Release chan struct{}
}

// Channels are provided through a test-only registration factory below.
func (c diagnosticBlockingConfig) Validate() error {
	close(c.Entered)
	<-c.Release

	return nil
}

func TestDiagnosticConcurrentReaders(t *testing.T) {
	diagnostics := MakeDiagnostics()
	loader := MakeLoader(WithDiagnostics(diagnostics), WithEnv(nil))
	loader.Register[diagnosticBlockingConfig]("app")
	entered, release := make(chan struct{}), make(chan struct{})
	loader.registrations[0].defaults = func() any { return &diagnosticBlockingConfig{entered, release} }
	done := make(chan error, 1)
	go func() { done <- loader.Load() }()
	<-entered
	var readers sync.WaitGroup
	for range 8 {
		readers.Go(func() {
			for range 100 {
				if report := diagnostics.Report(); report.State != LoadInProgress || len(report.Configs) != 0 {
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

	if diagnostics.Report().State != LoadSucceeded {
		t.Fatal(diagnostics.Report())
	}
}

type diagnosticPanics struct{}

func (diagnosticPanics) Validate() error { panic("original panic") }

func TestDiagnosticPanic(t *testing.T) {
	diagnostics := MakeDiagnostics()
	loader := MakeLoader(WithDiagnostics(diagnostics), WithEnv(nil), WithDiagnosticHandler(func(LoadReport) { t.Error("handler called after panic") }))
	loader.Register[diagnosticPanics]("app")
	func() {
		defer func() {
			if recover() != "original panic" {
				t.Error("panic changed")
			}
		}()
		_ = loader.Load()
	}()
	if diagnostics.Report().State != LoadPanicked || loader.Load() != errLoadPanicked {
		t.Fatal(diagnostics.Report())
	}
}

func TestDiagnosticHandlerPanicKeepsResult(t *testing.T) {
	diagnostics := MakeDiagnostics()
	loader := MakeLoader(WithDiagnostics(diagnostics), WithEnv(nil), WithDiagnosticHandler(func(LoadReport) { panic("handler panic") }))
	func() {
		defer func() {
			if recover() != "handler panic" {
				t.Error("panic changed")
			}
		}()
		_ = loader.Load()
	}()
	if diagnostics.Report().State != LoadSucceeded || loader.Load() != nil {
		t.Fatal(diagnostics.Report())
	}
}

func TestDiagnosticsDoNotRepeatProcessing(t *testing.T) {
	diagnostics := MakeDiagnostics()
	loader := MakeLoader(WithDiagnostics(diagnostics), WithEnv(map[string]string{"APP_TOKEN": "private-secret", "APP_ENDPOINT": "localhost"}))
	handle := loader.Register[widgetConfig]("app")
	defaults := loader.registrations[0].defaults
	defaultsCalls := 0
	loader.registrations[0].defaults = func() any {
		defaultsCalls++

		return defaults()
	}
	if _, err := loader.Manifest(); err != nil {
		t.Fatal(err)
	}

	if diagnostics.Report().State != LoadNotStarted {
		t.Fatal("manifest collected load report")
	}

	defaultsCalls = 0
	if err := loader.Load(); err != nil {
		t.Fatal(err)
	}

	if _, err := handle.Value(); err != nil {
		t.Fatal(err)
	}

	if err := loader.Load(); err != nil {
		t.Fatal(err)
	}

	if defaultsCalls != 1 {
		t.Fatal("defaults called", defaultsCalls, "times")
	}

	report := diagnostics.Report()
	if strings.Contains(fmt.Sprintf("%+v", report), "private-secret") {
		t.Fatal("secret leaked")
	}
}

func TestDiagnosticsPreserveLoadResult(t *testing.T) {
	env := WithEnv(map[string]string{"APP_REQUIRED": "present", "APP_EMPTY": "present", "APP_INPUT": "42"})
	expected, err := Load[diagnosticConfig]("app", env)
	if err != nil {
		t.Fatal(err)
	}

	diagnostics := MakeDiagnostics()
	actual, err := Load[diagnosticConfig]("app", env, WithDiagnostics(diagnostics))
	if err != nil || actual != expected || diagnostics.Report().State != LoadSucceeded {
		t.Fatal(actual, err, diagnostics.Report())
	}
}

type diagnosticErrorWriter struct{ err error }

func (w diagnosticErrorWriter) Write([]byte) (int, error) { return 0, w.err }

type diagnosticUnwrapPanic struct{}

func (diagnosticUnwrapPanic) Error() string { return "writer failed" }
func (diagnosticUnwrapPanic) Unwrap() error { panic("must not inspect writer cause") }

func TestDiagnosticDumpErrorDoesNotInspectUserCause(t *testing.T) {
	diagnostics := MakeDiagnostics()
	loader := MakeLoader(WithDiagnostics(diagnostics), WithEnv(nil), WithDump(diagnosticErrorWriter{diagnosticUnwrapPanic{}}))
	loader.Register[struct{}]("app")

	err := loader.Load()
	if err == nil {
		t.Fatal("expected writer error")
	}

	if loader.Load() != err {
		t.Fatal("completed load result not preserved")
	}

	if diagnostics.Report().State != LoadFailed {
		t.Fatal(diagnostics.Report())
	}
}

func TestDiagnosticPanicPreservesCompletedProblems(t *testing.T) {
	diagnostics := MakeDiagnostics()
	loader := MakeLoader(WithDiagnostics(diagnostics), WithEnv(nil))
	loader.Register[diagnosticValidation]("first")
	loader.Register[diagnosticPanics]("second")
	func() {
		defer func() { _ = recover() }()

		_ = loader.Load()
	}()

	report := diagnostics.Report()
	if report.State != LoadPanicked || len(report.Problems) != 1 || report.Problems[0].Kind != ErrorValidation {
		t.Fatal(report)
	}
}

func TestDiagnosticDumpErrorCannotSupplyConfigMetadata(t *testing.T) {
	diagnostics := MakeDiagnostics()
	writerError := &ConfigError{Kind: ErrorParse, InstanceName: "private-writer-data"}
	loader := MakeLoader(WithDiagnostics(diagnostics), WithEnv(nil), WithDump(diagnosticErrorWriter{writerError}))
	loader.Register[struct{}]("app")
	if err := loader.Load(); !errors.Is(err, writerError) {
		t.Fatal("writer cause not preserved", err)
	}

	report := diagnostics.Report()
	if report.State != LoadFailed || len(report.Problems) != 0 || report.Configs[0].Status != ConfigSucceeded {
		t.Fatal(report)
	}
}
