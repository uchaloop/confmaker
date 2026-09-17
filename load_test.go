package confmaker

import (
	"bytes"
	"strings"
	"testing"
)

type loadConfigType struct {
	Host string `env:"HOST,notEmpty"`
	Port int    `env:"PORT"`
}

func (loadConfigType) ConfigName() string { return "confxload" }

func (c *loadConfigType) SetDefaults() { c.Port = 5432 }

func TestLoadTakesBothKindsOfOption(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	cfg, err := Load[loadConfigType](
		WithName("confxreplica"),
		WithEnv(map[string]string{"CONFXREPLICA_HOST": "replica"}),
		WithPrefix("CONFXREPLICA_"),
		WithDump(&out),
	)
	if err != nil {
		t.Fatal(err)
	}

	if cfg.Host != "replica" || cfg.Port != 5432 || !strings.Contains(out.String(), "confxreplica") {
		t.Fatalf("got %+v, dump:\n%s", cfg, out.String())
	}
}

func TestLoadUsesConfigName(t *testing.T) {
	t.Parallel()

	cfg, err := Load[loadConfigType](WithEnv(map[string]string{"CONFXLOAD_HOST": "db", "CONFXLOAD_PORT": "6432"}))
	if err != nil || cfg.Host != "db" || cfg.Port != 6432 {
		t.Fatalf("got %+v, %v", cfg, err)
	}
}

func TestLoadChecksOnlyItsOwnPrefix(t *testing.T) {
	t.Parallel()

	env := map[string]string{
		"CONFXLOAD_HOST":  "db",
		"CONFXLOAD_PROT":  "typo",
		"CONFXOTHER_HSOT": "another config's business",
	}

	_, err := Load[loadConfigType](WithEnv(env))
	if err == nil || !strings.Contains(err.Error(), `unknown configuration variable "CONFXLOAD_PROT" (did you mean "CONFXLOAD_PORT"?)`) {
		t.Fatalf("the typo under the own prefix was missed: %v", err)
	}

	if strings.Contains(err.Error(), "CONFXOTHER_HSOT") {
		t.Fatalf("a foreign prefix was checked: %v", err)
	}

	if _, err := Load[loadConfigType](WithEnv(env), AllowUnknown("CONFXLOAD_PROT")); err != nil {
		t.Fatalf("AllowUnknown was ignored: %v", err)
	}
}

func TestLoadReportsEveryProblem(t *testing.T) {
	t.Parallel()

	_, err := Load[loadConfigType](WithEnv(map[string]string{"CONFXLOAD_HOST": "", "CONFXLOAD_PORT": "x"}))
	for _, want := range []string{
		`config "confxload": variable "CONFXLOAD_HOST" is set but empty`,
		`config "confxload": variable "CONFXLOAD_PORT"`,
	} {
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("report misses %q: %v", want, err)
		}
	}
}

func TestLoadRefusesNilAndInvalidRegistrations(t *testing.T) {
	t.Parallel()

	if _, err := Load[loadConfigType](nil); err == nil || !strings.Contains(err.Error(), "load option must not be nil") {
		t.Fatalf("nil option: %v", err)
	}

	if _, err := Load[envConfig](WithEnv(nil)); err == nil || !strings.Contains(err.Error(), "has no instance name") {
		t.Fatalf("missing name: %v", err)
	}
}
