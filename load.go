// Package confmaker is application-level configuration for Go services. A
// library declares its config as a plain struct with `koanf` (file) and `env`
// (environment) tags; the application loads the file and fills each library's
// struct, so a library never reads files or the environment itself.
//
// Two entry points build on this root package. confmaker/confx wires
// configuration into Uber Fx - it loads the file once and provides each library
// its filled, validated config by name. Load (below) decodes a whole file into a
// struct for services that do not use Fx.
//
// Infrastructure libraries need only the dependency-free secret type
// (github.com/uchaloop/secret); importing this package or confx stays optional,
// for when a library wants more. The `koanf`/`env` tags a library declares are
// inert strings the app-level loader reads.
package confmaker

import (
	"fmt"

	"github.com/go-viper/mapstructure/v2"
	"github.com/knadh/koanf/parsers/toml/v2"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/v2"
)

// defaultConfigPath is read when no paths are given: config.toml in the current
// working directory.
const defaultConfigPath = "config.toml"

// Load reads the given TOML files in order and decodes the merged result into
// dst using the `koanf:"..."` struct tags. Later files override earlier ones, so
// a base + environment-overlay layout works by passing base first. With no
// paths, it reads config.toml from the current working directory.
//
// Decoding is strict: an unknown key in the file (a typo, or a field that does
// not exist on dst) fails instead of being silently ignored. Durations are
// parsed from strings such as "30m"; weak type coercion is disabled, so a
// mistyped value fails rather than being guessed.
func Load(dst any, paths ...string) error {
	if len(paths) == 0 {
		paths = []string{defaultConfigPath}
	}

	k := koanf.New(".")

	for _, path := range paths {
		if err := k.Load(file.Provider(path), toml.Parser()); err != nil {
			return fmt.Errorf("load config %q: %w", path, err)
		}
	}

	return k.UnmarshalWithConf(
		"",
		dst,
		koanf.UnmarshalConf{
			Tag: "koanf",
			DecoderConfig: &mapstructure.DecoderConfig{
				Result:           dst,
				ErrorUnused:      true,
				WeaklyTypedInput: false,
				DecodeHook:       mapstructure.StringToTimeDurationHookFunc(),
			},
		},
	)
}
