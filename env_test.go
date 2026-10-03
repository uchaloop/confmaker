package confmaker

import (
	"os"
	"strings"
	"testing"
)

type envConfig struct {
	Host string `env:"HOST,required"`
	Port int    `env:"PORT"`
}

func TestWithEnvIsTheWholeEnvironment(t *testing.T) {
	// The process sets a value the map does not: it must not be read.
	t.Setenv("CONFXENV_PORT", "9999")

	cfg, err := Load[envConfig]("confxenv", WithEnv(map[string]string{"CONFXENV_HOST": "db"}))
	if err != nil {
		t.Fatal(err)
	}

	if cfg.Host != "db" || cfg.Port != 0 {
		t.Fatalf("got %+v, want only the map's values", cfg)
	}
}

func TestWithEnvCopiesTheMap(t *testing.T) {
	t.Parallel()

	vars := map[string]string{"CONFXENV_HOST": "db"}
	option := WithEnv(vars)
	vars["CONFXENV_HOST"] = "changed"
	vars["CONFXENV_PORT"] = "not a number"

	cfg, err := Load[envConfig]("confxenv", option)
	if err != nil || cfg.Host != "db" {
		t.Fatalf("a change after WithEnv reached the load: %+v, %v", cfg, err)
	}
}

func TestWithEnvNilIsEmpty(t *testing.T) {
	t.Setenv("CONFXENV_HOST", "from the process")

	_, err := Load[envConfig]("confxenv", WithEnv(nil))
	if err == nil || !strings.Contains(err.Error(), `required variable "CONFXENV_HOST" is not set`) {
		t.Fatalf("nil did not load an empty environment: %v", err)
	}
}

func TestWithEnvGivenTwiceIsReported(t *testing.T) {
	t.Parallel()

	env := WithEnv(map[string]string{"CONFXENV_HOST": "db"})
	_, err := Load[envConfig]("confxenv", env, env)
	if err == nil || !strings.Contains(err.Error(), "WithEnv is given more than once") {
		t.Fatalf("got %v", err)
	}
}

// snapshotConfig changes the process environment while it is being loaded.
type snapshotConfig struct {
	Host string `env:"HOST"`
}

func (*snapshotConfig) SetDefaults() {
	_ = os.Setenv("CONFXSNAPSHOT_HOST", "set during load")
}

func TestProcessEnvironmentIsTakenOnce(t *testing.T) {
	t.Setenv("CONFXSNAPSHOT_HOST", "before load")

	cfg, err := Load[snapshotConfig]("confxsnapshot")
	if err != nil {
		t.Fatal(err)
	}

	if cfg.Host != "before load" {
		t.Fatalf("host = %q, want the value from before the load started", cfg.Host)
	}
}

func TestProcessEnvironmentPreservesEqualsInValues(t *testing.T) {
	t.Setenv("CONFXENV_EQUALS", "a=b")

	loaded, err := Load[struct {
		Value string `env:"EQUALS"`
	}]("confxenv")
	if err != nil {
		t.Fatal(err)
	}

	if loaded.Value != "a=b" {
		t.Fatalf("value = %q, want a=b", loaded.Value)
	}
}
