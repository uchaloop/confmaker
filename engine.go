package confmaker

import (
	"context"
	"errors"
	"fmt"
	"reflect"
)

// Engine fills registered configurations using its own sources, tags and rules.
// Calls are sequential within a Loader. Sharing an Engine across loaders requires
// safe concurrent use. The caller owns its resources; Loader never closes it.
type Engine interface {
	Load(context.Context, LoadRequest) error
}

// LoadRequest identifies a configuration and its non-nil pointer to a struct.
// The engine must not retain Target or modify it after Load returns.
type LoadRequest struct {
	Name   string
	Target any
}

// EngineFunc adapts a function to Engine.
type EngineFunc func(context.Context, LoadRequest) error

// Load invokes f synchronously.
func (f EngineFunc) Load(ctx context.Context, req LoadRequest) error { return f(ctx, req) }

// WithEngine selects one backend for every registration. Without it Loader uses
// the built-in ENV engine. Nil, repeated selection and built-in ENV options with
// a custom engine are configuration errors. Backend errors may contain values;
// their sanitization is the backend's responsibility.
func WithEngine(engine Engine) LoaderOption {
	return envOption(func(s *envSettings) {
		if s.engineSet || nilEngineValue(engine) {
			s.engineErr = errors.New("WithEngine requires one non-nil engine")
		}
		s.engineSet = true
		s.engine = engine
	})
}

func nilEngineValue(engine Engine) bool {
	if engine == nil {
		return true
	}

	v := reflect.ValueOf(engine)
	switch v.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return v.IsNil()
	}

	return false
}

func makeEngineDescriptor[T any](name string, opts []ConfigOption) (descriptor, error) {
	d := descriptor{
		instanceName: name,
		defaults:     makeDefaults[T],
	}

	if name == "" {
		return d, makeConfigError(ErrorDeclaration, "", "", errors.New("an instance name is required"))
	}

	if reflect.TypeFor[T]().Kind() != reflect.Struct {
		return d, makeConfigError(ErrorDeclaration, "", "", errors.New("configuration must be a struct"))
	}

	if len(opts) != 0 {
		return d, makeConfigError(ErrorDeclaration, "", "", errors.New("configuration options require the built-in engine"))
	}

	return d, nil
}

func (l *Loader) loadEngine(ctx context.Context, regs []*registration, report *LoadReport) error {
	var errs []error
	if report != nil {
		defer func() {
			for _, err := range errs {
				recordLoadProblems(report, err)
			}
		}()
	}

	if l.envErr != nil {
		errs = append(errs, l.envErr)
	}

	if l.env.engineErr != nil {
		errs = append(errs, l.env.engineErr)
	}

	if l.env.builtinOptions {
		errs = append(errs, errors.New("ENV options require the built-in engine"))
	}

	for _, r := range regs {
		if r.err != nil {
			errs = append(errs, wrapConfigError(r.name, r.err))
		}
	}

	if err := checkIdentityConflicts(regs); err != nil {
		errs = append(errs, err)
	}

	if len(errs) != 0 {
		return errors.Join(errs...)
	}

	configs := make([]any, len(regs))
	for i, r := range regs {
		if ctx.Err() != nil {
			return errors.Join(append(errs, ctx.Err())...)
		}

		if report != nil {
			report.Configs[i].Status = ConfigInterrupted
		}
		configs[i] = r.defaults()
		if ctx.Err() != nil {
			return errors.Join(append(errs, ctx.Err())...)
		}

		if report != nil {
			report.Configs[i].Status = ConfigNotProcessed
		}
	}

	for i, r := range regs {
		if ctx.Err() != nil {
			return errors.Join(append(errs, ctx.Err())...)
		}

		if report != nil {
			report.Configs[i].Status = ConfigInterrupted
		}

		err := r.fillEngine(ctx, l.env.engine, configs[i])
		if report != nil {
			setConfigReportStatus(&report.Configs[i], ctx, err)
		}

		if err != nil {
			errs = append(errs, err)
		}
	}

	return errors.Join(errs...)
}

// Identity is checked before schema filtering so even invalid declarations reserve
// their names for the common preflight check.
func checkIdentityConflicts(regs []*registration) error {
	seen := make(map[string]bool)
	var errs []error
	for _, r := range regs {
		if seen[r.name] {
			errs = append(errs, wrapConfigError(r.name, makeConfigError(ErrorConflict, "", "", fmt.Errorf("instance name %q is registered more than once", r.name))))
		}
		seen[r.name] = true
	}

	return errors.Join(errs...)
}
