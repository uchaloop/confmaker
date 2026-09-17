package confmaker

import (
	"bytes"
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

	cfg, err := Load[envConfig](WithEnv(map[string]string{"CONFXENV_HOST": "db"}), WithName("confxenv"))
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

	cfg, err := Load[envConfig](option, WithName("confxenv"))
	if err != nil || cfg.Host != "db" {
		t.Fatalf("a change after WithEnv reached the load: %+v, %v", cfg, err)
	}
}

func TestWithEnvNilIsEmpty(t *testing.T) {
	t.Setenv("CONFXENV_HOST", "from the process")

	_, err := Load[envConfig](WithEnv(nil), WithName("confxenv"))
	if err == nil || !strings.Contains(err.Error(), `required variable "CONFXENV_HOST" is not set`) {
		t.Fatalf("nil did not load an empty environment: %v", err)
	}
}

func TestWithEnvGivenTwiceIsReported(t *testing.T) {
	t.Parallel()

	env := WithEnv(map[string]string{"CONFXENV_HOST": "db"})
	_, err := Load[envConfig](env, env, WithName("confxenv"))
	if err == nil || !strings.Contains(err.Error(), "WithEnv is given more than once") {
		t.Fatalf("got %v", err)
	}
}

func TestWithEnvFeedsTheCheckAndTheDump(t *testing.T) {
	// The process holds a typo and a host the map does not: neither may show.
	t.Setenv("CONFXENV_HSOT", "process typo")
	t.Setenv("CONFXENV_HOST", "process host")

	var out bytes.Buffer
	_, err := Load[envConfig](
		WithName("confxenv"),
		WithDump(&out),
		WithEnv(map[string]string{"CONFXENV_HOST": "map host", "CONFXENV_PROT": "1"}),
	)

	if err == nil || !strings.Contains(err.Error(), `unknown configuration variable "CONFXENV_PROT"`) || strings.Contains(err.Error(), "CONFXENV_HSOT") {
		t.Fatalf("the check did not read the map: %v", err)
	}

	if !strings.Contains(out.String(), "map host") || strings.Contains(out.String(), "process host") {
		t.Fatalf("the dump did not read the map:\n%s", out.String())
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

	cfg, err := Load[snapshotConfig](WithName("confxsnapshot"))
	if err != nil {
		t.Fatal(err)
	}

	if cfg.Host != "before load" {
		t.Fatalf("host = %q, want the value from before the load started", cfg.Host)
	}
}

func TestOSEnvironmentSkipsNamelessEntries(t *testing.T) {
	t.Setenv("CONFXENV_EQUALS", "a=b")

	env := osEnvironment()
	if value, ok := env["CONFXENV_EQUALS"]; !ok || value != "a=b" {
		t.Fatalf("value with = split wrongly: %q, %v", value, ok)
	}

	if _, ok := env[""]; ok {
		t.Fatal("a nameless entry was kept")
	}
}
