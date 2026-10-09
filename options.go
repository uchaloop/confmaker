package confmaker

import (
	"errors"
	"fmt"
	"reflect"
)

// LoadOption is an option [Load] takes: every [ConfigOption] and every
// [LoaderOption].
type LoadOption interface {
	loadOption()
}

// ConfigOption configures one config with [WithPrefix]. [Loader.Register],
// [Manifest] and [Load] take it.
type ConfigOption interface {
	DescribeOption
	LoadOption
	applyConfig(*configSettings)
}

// LoaderOption configures a whole load: [WithEnv], [AllowUnknown],
// [WithDiagnostics] and [WithDiagnosticHandler].
// [MakeLoader] and [Load] take it.
type LoaderOption interface {
	LoadOption
	applyEnv(*envSettings)
}

// configSettings holds the resolved options of one config.
type configSettings struct {
	name, prefix string
	prefixSet    bool
}

// envSettings holds the resolved options of a load.
type envSettings struct {
	engine            Engine
	engineSet         bool
	engineErr         error
	builtinOptions    bool
	allowed           []string
	env               environment
	envRepeated       bool
	diagnostics       bool
	diagnosticHandler func(LoadReport)
	diagnosticErr     error
}

type configOption func(*configSettings)

func (o configOption) applyConfig(s *configSettings) { o(s) }
func (configOption) describeOption()                 {}
func (configOption) loadOption()                     {}

type envOption func(*envSettings)

func (o envOption) applyEnv(s *envSettings) { o(s) }
func (envOption) loadOption()               {}

// WithPrefix overrides the environment prefix without changing the instance
// name. It must be non-empty, contain only upper-case letters, digits or
// underscores, and end with
// an underscore, for example "REPLICA_POSTGRES_".
func WithPrefix(prefix string) ConfigOption {
	return configOption(func(s *configSettings) {
		s.prefix, s.prefixSet = prefix, true
	})
}

// AllowUnknown exempts variables starting with the given prefixes from the check
// for unknown variables, for an environment shared with other programs. The
// prefixes are copied. An empty prefix is ignored: it would match every variable
// and turn the check off.
func AllowUnknown(prefixes ...string) LoaderOption {
	// Copied here, like WithEnv's map: changing the caller's slice afterwards
	// does not change what the option allows.
	allowed := make([]string, 0, len(prefixes))
	for _, prefix := range prefixes {
		if len(prefix) != 0 {
			allowed = append(allowed, prefix)
		}
	}

	return envOption(func(s *envSettings) {
		s.builtinOptions = true
		s.allowed = append(s.allowed, allowed...)
	})
}

// resolveSettings validates the explicit name and options of one config and
// derives its prefix when none is supplied.
func resolveSettings[T any](name string, opts []ConfigOption) (configSettings, error) {
	set := configSettings{name: name}
	for _, opt := range opts {
		if opt == nil {
			return set, errors.New("a config option must not be nil")
		}

		opt.applyConfig(&set)
	}

	if reflect.TypeFor[T]().Kind() != reflect.Struct {
		return set, fmt.Errorf("a config must be a struct, got %s", reflect.TypeFor[T]())
	}

	if err := checkName(set.name); err != nil {
		return set, err
	}

	if !set.prefixSet {
		set.prefix = defaultPrefix(set.name)
	}

	if err := checkPrefix(set.prefix); err != nil {
		return set, err
	}

	return set, nil
}
