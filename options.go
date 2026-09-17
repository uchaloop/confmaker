package confmaker

import (
	"errors"
	"fmt"
	"io"
	"reflect"
)

// LoadOption is an option [Load] takes: every [ConfigOption] and every
// [EnvOption].
type LoadOption interface {
	loadOption()
}

// ConfigOption configures one config: [WithName] and [WithPrefix]. [Loader.Add],
// [Manifest] and [Load] take it.
type ConfigOption interface {
	LoadOption
	applyConfig(*configSettings)
}

// EnvOption configures a whole load: [WithEnv], [WithDump] and [AllowUnknown].
// [MakeLoader] and [Load] take it.
type EnvOption interface {
	LoadOption
	applyEnv(*envSettings)
}

// configSettings holds the resolved options of one config.
type configSettings struct {
	name, prefix                     string
	nameSet, prefixSet, nameRepeated bool
}

// envSettings holds the resolved options of a load.
type envSettings struct {
	allowed     []string
	dump        io.Writer
	env         environment
	envRepeated bool
}

type configOption func(*configSettings)

func (o configOption) applyConfig(s *configSettings) { o(s) }
func (configOption) loadOption()                     {}

type envOption func(*envSettings)

func (o envOption) applyEnv(s *envSettings) { o(s) }
func (envOption) loadOption()               {}

// WithName sets the instance name, instead of the config's ConfigName. The name
// gives the default prefix ("read-replica" reads READ_REPLICA_*) and labels the
// config in errors. It may be given once per config.
func WithName(name string) ConfigOption {
	return configOption(func(s *configSettings) {
		s.nameRepeated = s.nameSet
		s.name, s.nameSet = name, true
	})
}

// WithPrefix overrides the environment prefix without changing the instance
// name. It must contain upper-case letters, digits or underscores and end with
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
func AllowUnknown(prefixes ...string) EnvOption {
	// Copied here, like WithEnv's map: changing the caller's slice afterwards
	// does not change what the option allows.
	allowed := make([]string, 0, len(prefixes))
	for _, prefix := range prefixes {
		if len(prefix) != 0 {
			allowed = append(allowed, prefix)
		}
	}

	return envOption(func(s *envSettings) {
		s.allowed = append(s.allowed, allowed...)
	})
}

// WithDump writes a table to w while loading: each variable the configs declare,
// its type, its environment value or rendered default, and where the value comes
// from. It is written even when loading fails, except for conflicting
// registrations, so it is not proof that loading succeeded. Secrets are shown as
// set or unset, never printed.
func WithDump(w io.Writer) EnvOption {
	return envOption(func(s *envSettings) {
		s.dump = w
	})
}

// resolveSettings applies the options of one config and completes its name and
// prefix from ConfigName and the name.
func resolveSettings[T any](opts []ConfigOption) (configSettings, error) {
	var set configSettings
	for _, opt := range opts {
		if opt == nil {
			return set, errors.New("a config option must not be nil")
		}

		opt.applyConfig(&set)
	}

	if set.nameRepeated {
		return set, fmt.Errorf("config %s is given an instance name more than once", reflect.TypeFor[T]())
	}

	if !set.nameSet {
		// Only structs are configs. Checking before calling a user method also
		// avoids invoking a promoted method through a nil *T.
		if reflect.TypeFor[T]().Kind() != reflect.Struct {
			return set, fmt.Errorf("a config must be a struct, got %s", reflect.TypeFor[T]())
		}

		var cfg T
		namer, ok := any(&cfg).(ConfigNamer)
		if !ok {
			return set, fmt.Errorf("config %s has no instance name; use WithName or implement ConfigName() string", reflect.TypeFor[T]())
		}

		set.name = namer.ConfigName()
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
