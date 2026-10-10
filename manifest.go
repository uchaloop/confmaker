package confmaker

import (
	"errors"
	"slices"
)

// ErrManifestUnsupported indicates that a custom engine does not provide manifests.
var ErrManifestUnsupported = errors.New("manifest requires the built-in engine")

// Variable describes a field using the same schema that loads it.
type Variable struct {
	Name, FieldPath, Description, Type string
	Required, NotEmpty, Secret         bool
	Format, Separator, KeyValSeparator string
	Default                            DefaultInfo
}

// ConfigManifest describes one registration without loaded values.
type ConfigManifest struct {
	InstanceName, Prefix string
	Variables            []Variable
}

// ManifestResult is an independent description with optional default-render problems.
type ManifestResult struct {
	Configs  []ConfigManifest
	Problems []ManifestProblem
}

// ManifestProblem deliberately excludes error messages and causes.
type ManifestProblem struct {
	Kind                                  ErrorKind
	InstanceName, VariableName, FieldPath string
}

// DescribeOption configures the single-configuration Manifest operation.
type DescribeOption interface{ describeOption() }

// ManifestOption configures optional manifest evaluation, never loading.
type ManifestOption interface {
	DescribeOption
	applyManifest(*manifestSettings)
}

type manifestSettings struct{ defaults bool }
type manifestOption func(*manifestSettings)

func (manifestOption) describeOption()                     {}
func (o manifestOption) applyManifest(s *manifestSettings) { o(s) }

// IncludeDefaults evaluates root defaults once per configuration on fresh instances
// and renders nonzero values. Sensitive fields remain redacted and are never
// marshaled. User panics propagate without changing the loader's load state.
func IncludeDefaults() ManifestOption {
	return manifestOption(func(s *manifestSettings) { s.defaults = true })
}

// Manifest returns ErrManifestUnsupported for custom engines without calling user code.
// With the built-in engine it describes a registration snapshot. Without IncludeDefaults it runs
// no user methods and reads no ENV. Configurations are sorted by instance name;
// variables follow declaration order. Returned slices are independent snapshots.
// Schema errors return no result; render problems stay in the result with a nil
// error. Loaded values are never used. Concurrent calls with IncludeDefaults
// require user defaults and marshalers to support concurrent invocation.
func (l *Loader) Manifest(opts ...ManifestOption) (ManifestResult, error) {
	if l.env.engineSet {
		return ManifestResult{}, ErrManifestUnsupported
	}
	var settings manifestSettings
	for _, o := range opts {
		if o == nil {
			return ManifestResult{}, makeConfigError(ErrorDeclaration, "", "", errors.New("nil manifest option"))
		}
		o.applyManifest(&settings)
	}
	l.mu.Lock()
	regs := slices.Clone(l.registrations)
	l.mu.Unlock()
	sortRegistrations(regs)
	var errs []error
	descriptors := make([]descriptor, 0, len(regs))
	for _, r := range regs {
		if r.err != nil {
			errs = append(errs, r.err)
		} else {
			descriptors = append(descriptors, r.descriptor)
		}
	}

	if _, err := checkRegistrations(descriptors); err != nil {
		errs = append(errs, err)
	}

	if err := errors.Join(errs...); err != nil {
		return ManifestResult{}, err
	}

	result := ManifestResult{Configs: make([]ConfigManifest, 0, len(descriptors))}
	for _, d := range descriptors {
		config := ConfigManifest{InstanceName: d.instanceName, Prefix: d.prefix, Variables: make([]Variable, 0, len(d.fields))}
		for _, f := range d.fields {
			state := DefaultNotEvaluated
			if f.Secret {
				state = DefaultRedacted
			}
			config.Variables = append(config.Variables, Variable{Name: f.Name, FieldPath: f.field, Description: f.Description, Type: f.Type, Required: f.Required, NotEmpty: f.NotEmpty, Secret: f.Secret, Format: f.format, Separator: f.separator, KeyValSeparator: f.keyValSeparator, Default: DefaultInfo{State: state}})
		}

		if settings.defaults {
			result.Problems = append(result.Problems, evaluateDefaults(d, config.Variables)...)
		}
		result.Configs = append(result.Configs, config)
	}

	return result, nil
}

// Manifest describes one configuration with the same result model as Loader.Manifest.
// Options may include ConfigOption values such as WithPrefix and ManifestOption
// values such as IncludeDefaults. It does not load the configuration.
func Manifest[T any](name string, opts ...DescribeOption) (ManifestResult, error) {
	var configs []ConfigOption
	var manifests []ManifestOption
	for _, o := range opts {
		switch o := o.(type) {
		case ConfigOption:
			configs = append(configs, o)
		case ManifestOption:
			manifests = append(manifests, o)
		default:
			return ManifestResult{}, makeConfigError(ErrorDeclaration, "", "", errors.New("nil description option"))
		}
	}

	l := MakeLoader()
	l.Register[T](name, configs...)
	return l.Manifest(manifests...)
}
