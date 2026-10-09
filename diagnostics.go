package confmaker

import (
	"errors"
	"slices"
)

// LoadState describes the progress of one load.
type LoadState string

const (
	LoadDisabled   LoadState = "disabled"
	LoadNotStarted LoadState = "not_started"
	LoadInProgress LoadState = "in_progress"
	LoadSucceeded  LoadState = "succeeded"
	LoadFailed     LoadState = "failed"
	LoadPanicked   LoadState = "panicked"
)

// ConfigStatus describes processing of a config, independently of other configs.
type ConfigStatus string

const (
	ConfigInterrupted  ConfigStatus = "interrupted"
	ConfigNotProcessed ConfigStatus = "not_processed"
	ConfigSucceeded    ConfigStatus = "succeeded"
	ConfigFailed       ConfigStatus = "failed"
)

// VariableStatus describes ENV application to a field, not config validation.
type VariableStatus string

const (
	VariableInterrupted  VariableStatus = "interrupted"
	VariableNotProcessed VariableStatus = "not_processed"
	VariableSucceeded    VariableStatus = "succeeded"
	VariableFailed       VariableStatus = "failed"
)

// ValueSource identifies the input used or required by a field. A zero value
// explicitly assigned in SetDefaults cannot be distinguished from an untouched zero.
type ValueSource string

const (
	SourceUnknown ValueSource = "unknown"
	SourceEnv     ValueSource = "env"
	SourceDefault ValueSource = "default"
	SourceZero    ValueSource = "zero"
	SourceMissing ValueSource = "missing"
)

// LoadReport is a value-free snapshot. Problems never contain error messages or
// causes. Configs include invalid registrations, whose fields may be unavailable.
// Successful fields do not imply successful config validation or loading.
// After a panic, completed stages retain their problems; the interrupted stage
// may be incomplete.
type LoadReport struct {
	State    LoadState
	Configs  []ConfigReport
	Problems []LoadProblem
}

// ConfigReport describes one registration, including its explicit name and Go type.
type ConfigReport struct {
	InstanceName string
	Type         string
	Prefix       string
	Status       ConfigStatus
	Variables    []VariableReport
}

// VariableReport describes one compiled field without its value or default.
type VariableReport struct {
	Name      string
	FieldPath string
	Source    ValueSource
	Status    VariableStatus
}

// LoadProblem contains only the structural context of a library error.
// Context is empty when the problem cannot be attributed to one config or field.
type LoadProblem struct {
	Kind         ErrorKind
	InstanceName string
	VariableName string
	FieldPath    string
}

// Report returns an independent snapshot without starting or waiting for a load.
func (l *Loader) Report() LoadReport {
	l.mu.Lock()
	defer l.mu.Unlock()
	if !l.env.diagnostics {
		return LoadReport{State: LoadDisabled}
	}
	if l.state == loading {
		return LoadReport{State: LoadInProgress}
	}
	if l.report.State == "" {
		return LoadReport{State: LoadNotStarted}
	}
	return cloneLoadReport(l.report)
}

func cloneLoadReport(report LoadReport) LoadReport {
	report.Configs = slices.Clone(report.Configs)
	for i := range report.Configs {
		report.Configs[i].Variables = slices.Clone(report.Configs[i].Variables)
	}

	report.Problems = slices.Clone(report.Problems)

	return report
}

// WithDiagnostics enables collection of value-free reports on the loader.
// Repeated use is harmless.
func WithDiagnostics() EnvOption {
	return envOption(func(s *envSettings) { s.diagnostics = true })
}

// WithDiagnosticHandler enables collection and calls handler synchronously once
// after load completion is published, on success or error, before Load returns.
// The handler receives an independent report and may read handles or call Load.
// It is not called when loading panics. Handler panics propagate without changing
// the completed load result. A nil or repeated handler is a declaration error.
// Concurrent Load callers may return before the handler finishes.
// No logging, process termination or error recovery is performed by this option.
func WithDiagnosticHandler(handler func(LoadReport)) EnvOption {
	return envOption(func(s *envSettings) {
		if handler == nil || s.diagnosticHandler != nil {
			s.diagnosticErr = makeConfigError(ErrorDeclaration, "", "", errors.New("WithDiagnosticHandler requires one non-nil handler"))

			return
		}

		s.diagnosticHandler = handler
		s.diagnostics = true
	})
}

func makeLoadReport(registrations []*registration) *LoadReport {
	report := &LoadReport{
		State:   LoadInProgress,
		Configs: make([]ConfigReport, len(registrations)),
	}
	for i, registration := range registrations {
		configReport := ConfigReport{
			InstanceName: registration.name,
			Type:         registration.typeName,
			Prefix:       registration.prefix,
			Status:       ConfigNotProcessed,
		}
		if registration.err != nil {
			configReport.Status = ConfigFailed
		}

		if len(registration.fields) != 0 {
			configReport.Variables = make([]VariableReport, len(registration.fields))
		}

		for index, field := range registration.fields {
			configReport.Variables[index] = VariableReport{
				Name:      field.Name,
				FieldPath: field.field,
				Source:    SourceUnknown,
				Status:    VariableNotProcessed,
			}
		}

		report.Configs[i] = configReport
	}

	return report
}

func setLoadReportState(report *LoadReport, err error) {
	if err != ErrLoadPanicked {
		for _, p := range report.Problems {
			if p.InstanceName != "" {
				for i := range report.Configs {
					if report.Configs[i].InstanceName == p.InstanceName {
						report.Configs[i].Status = ConfigFailed
					}
				}
			}
		}
	}
	switch {
	case err == ErrLoadPanicked:
		report.State = LoadPanicked
	case err != nil:
		report.State = LoadFailed
	default:
		report.State = LoadSucceeded
	}
}

// recordLoadProblems collects library problems without inspecting user causes.
func recordLoadProblems(report *LoadReport, err error) {
	switch problem := err.(type) {
	case *ConfigError:
		if problem == nil {
			return
		}

		report.Problems = append(report.Problems, LoadProblem{
			Kind:         problem.Kind,
			InstanceName: problem.InstanceName,
			VariableName: problem.VariableName,
			FieldPath:    problem.FieldPath,
		})
	case configError:
		recordLoadProblems(report, problem.err)
	case interface{ Unwrap() []error }:
		for _, child := range problem.Unwrap() {
			recordLoadProblems(report, child)
		}
	}
}
