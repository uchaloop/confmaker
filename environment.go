package confmaker

import (
	"maps"
	"os"
	"strings"
)

// environment is the set of variables one load reads. It is taken once, at the
// start of Load, and shared by loading, the unknown-variable check and the dump,
// so all three see the same values whatever changes the process environment
// meanwhile. It is never modified after it is taken.
type environment map[string]string

// osEnvironment copies the process environment. An entry without a name - the
// per-drive "=C:" entries Windows keeps - is not a variable a config can read.
func osEnvironment() environment {
	entries := os.Environ()

	env := make(environment, len(entries))
	for _, entry := range entries {
		name, value, _ := strings.Cut(entry, "=")
		if len(name) != 0 {
			env[name] = value
		}
	}

	return env
}

// WithEnv makes a load read vars instead of the process environment.
//
//   - vars is the complete environment: nothing is added from the process.
//   - nil is an empty environment, not the process environment.
//   - The map is copied when WithEnv is called; later changes to it are not seen.
//   - WithEnv may be given once per load; a second one is reported by
//     [Loader.Load].
//
// Without WithEnv, a load takes a snapshot of the process environment when it
// starts.
func WithEnv(vars map[string]string) EnvOption {
	env := make(environment, len(vars))
	maps.Copy(env, vars)

	return envOption(func(s *envSettings) {
		s.envRepeated = s.env != nil
		s.env = env
	})
}
