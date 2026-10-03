package confmaker

import (
	"reflect"
	"strings"
	"testing"

	"github.com/uchaloop/secret/v2"
)

// strictConfig is the config the strict-check tests register.
type strictConfig struct {
	Host string        `env:"HOST"`
	Port int           `env:"PORT"`
	Pass secret.Secret `env:"PASSWORD,required"`
}

// SetDefaults supplies an optional port.
func (c *strictConfig) SetDefaults() {
	c.Port = 5432
}

// loadStrict loads the strictConfig instance "confxpostgres" from env.
func loadStrict(env map[string]string, opts ...EnvOption) error {
	loadOpts := []LoadOption{WithEnv(env)}
	for _, opt := range opts {
		loadOpts = append(loadOpts, opt)
	}

	if _, err := Load[strictConfig]("confxpostgres", loadOpts...); err != nil {
		return err
	}

	return nil
}

func TestCheckAcceptsDeclaredVariables(t *testing.T) {
	t.Parallel()

	env := map[string]string{
		"CONFXPOSTGRES_HOST":     "db:5432",
		"CONFXPOSTGRES_PASSWORD": "s3cr3t",
	}

	if err := loadStrict(env); err != nil {
		t.Fatalf("declared variables rejected: %v", err)
	}
}

func TestCheckRejectsTypoUnderOwnPrefix(t *testing.T) {
	t.Parallel()

	env := map[string]string{
		"CONFXPOSTGRES_HOST":     "db:5432",
		"CONFXPOSTGRES_PASSWORD": "s3cr3t",
		"CONFXPOSTGRES_HSOT":     "typo",
	}

	err := loadStrict(env)
	if err == nil {
		t.Fatal("expected the typo to fail the load")
	}

	if !strings.Contains(err.Error(), "CONFXPOSTGRES_HSOT") {
		t.Fatalf("error does not name the variable: %v", err)
	}

	if !strings.Contains(err.Error(), `did you mean "CONFXPOSTGRES_HOST"`) {
		t.Fatalf("error does not suggest the intended variable: %v", err)
	}
}

func TestCheckIgnoresForeignPrefixes(t *testing.T) {
	t.Parallel()

	env := map[string]string{
		"CONFXPOSTGRES_HOST":     "db:5432",
		"CONFXPOSTGRES_PASSWORD": "s3cr3t",
		"KAFKA_BROKERS":          "not ours",
	}

	if err := loadStrict(env); err != nil {
		t.Fatalf("a variable outside the application's prefixes was reported: %v", err)
	}
}

func TestAllowUnknownExemptsPrefix(t *testing.T) {
	t.Parallel()

	env := map[string]string{
		"CONFXPOSTGRES_HOST":         "db:5432",
		"CONFXPOSTGRES_PASSWORD":     "s3cr3t",
		"CONFXPOSTGRES_EXPORTER_URL": "sidecar",
	}

	if err := loadStrict(env, AllowUnknown("CONFXPOSTGRES_EXPORTER_")); err != nil {
		t.Fatalf("exempted prefix still reported: %v", err)
	}
}

func TestCheckCoversEveryInstance(t *testing.T) {
	t.Parallel()

	env := map[string]string{
		"CONFXMAIN_HOST":        "db:5432",
		"CONFXMAIN_PASSWORD":    "s3cr3t",
		"CONFXREPLICA_HOST":     "replica:5432",
		"CONFXREPLICA_PASSWORD": "s3cr3t",
		"CONFXREPLICA_PROT":     "typo",
	}

	loader := MakeLoader(WithEnv(env))
	loader.Register[strictConfig]("confxmain")
	loader.Register[strictConfig]("confxreplica")
	err := loader.Load()
	if err == nil || !strings.Contains(err.Error(), "CONFXREPLICA_PROT") {
		t.Fatalf("a named instance was not covered by the check: %v", err)
	}
}

func TestInstanceWithoutPrefixIsRefused(t *testing.T) {
	t.Parallel()

	// Every instance owns a prefix, so the scan has no exception to make: an
	// instance with no prefix would claim the whole environment and is refused
	// where it is declared.
	loader := MakeLoader(WithEnv(nil))
	loader.Register[strictConfig]("confxpostgres", WithPrefix(""))
	err := loader.Load()
	if err == nil {
		t.Fatal("an instance with no prefix was accepted")
	}
}

func TestNestedPrefixesDoNotCollide(t *testing.T) {
	t.Parallel()

	env := map[string]string{
		"CONFXAPI_BASE_URL":        "https://api",
		"CONFXAPI_STATUS_BASE_URL": "https://api/cards",
	}

	type outer struct {
		BaseURL string `env:"BASE_URL"`
	}

	// CONFXAPI_ and CONFXAPI_STATUS_ overlap: a variable of the longer instance
	// must not be reported as unknown for the shorter one.

	loader := MakeLoader(WithEnv(env))
	loader.Register[outer]("confxapi")
	loader.Register[outer]("confxapi_status")
	err := loader.Load()
	if err != nil {
		t.Fatalf("overlapping prefixes reported a false positive: %v", err)
	}
}

func TestAllowUnknownIgnoresEmptyPrefix(t *testing.T) {
	t.Parallel()

	env := map[string]string{
		"CONFXPOSTGRES_HOST":     "db:5432",
		"CONFXPOSTGRES_PASSWORD": "s3cr3t",
		"CONFXPOSTGRES_HSOT":     "typo",
	}

	// An empty prefix matches everything; honouring it would turn the check off
	// without saying so.
	err := loadStrict(env, AllowUnknown(""))
	if err == nil || !strings.Contains(err.Error(), "CONFXPOSTGRES_HSOT") {
		t.Fatalf("an empty prefix silently disabled the check: %v", err)
	}
}

func TestNonStructConfigIsNotBlamedOnTheEnvironment(t *testing.T) {
	t.Parallel()

	env := map[string]string{
		"CONFXTHING_HOST": "db:5432",
	}

	// A pointer T is a mistake in the code, not in the environment. The check
	// must not bury the parser's own report under a list of "unknown" variables.
	loader := MakeLoader(WithEnv(env))
	loader.Register[*strictConfig]("confxthing")
	err := loader.Load()
	if err == nil {
		t.Fatal("expected a pointer config to fail")
	}

	if strings.Contains(err.Error(), "unknown configuration variable") {
		t.Fatalf("the environment was blamed for a mistake in the code: %v", err)
	}
}

func TestUnreadableConfigIsReported(t *testing.T) {
	t.Parallel()

	type shard struct {
		Host string `env:"HOST"`
	}

	// A collection of structs would read variables no type can enumerate, so the
	// declaration is refused instead of quietly leaving a hole in the check.
	loader := MakeLoader(WithEnv(nil))
	loader.Register[struct {
		Name   string `env:"NAME"`
		Shards []shard
	}]("confxcluster")
	err := loader.Load()
	if err == nil || !strings.Contains(err.Error(), "nest by value") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestTwoInstancesMayNotClaimOneVariable covers the collision a single config
// cannot see: two prefixes that run into each other, so a name declared under
// one instance is the name another instance reads. One value would fill two
// configs that nothing keeps in step.
func TestTwoInstancesMayNotClaimOneVariable(t *testing.T) {
	_, err := checkRegistrations([]descriptor{
		{instanceName: "db", prefix: "CONFXDB_", fields: mustDescribe[struct {
			Host string `env:"MAIN_HOST"`
		}](t, "CONFXDB_")},
		{instanceName: "db_main", prefix: "CONFXDB_MAIN_", fields: mustDescribe[struct {
			Host string `env:"HOST"`
		}](t, "CONFXDB_MAIN_")},
	})

	if err == nil {
		t.Fatal("the shared variable was accepted")
	}

	if !strings.Contains(err.Error(), "CONFXDB_MAIN_HOST") {
		t.Fatalf("the error does not name the variable: %v", err)
	}

	if !strings.Contains(err.Error(), `"db"`) || !strings.Contains(err.Error(), `"db_main"`) {
		t.Fatalf("the error does not name both instances: %v", err)
	}
}

func TestDistinctInstancesAreNotACollision(t *testing.T) {
	type config struct {
		Host string `env:"HOST"`
	}

	_, err := checkRegistrations([]descriptor{
		{instanceName: "primary", prefix: "CONFXPRIMARY_", fields: mustDescribe[config](t, "CONFXPRIMARY_")},
		{instanceName: "replica", prefix: "CONFXREPLICA_", fields: mustDescribe[config](t, "CONFXREPLICA_")},
	})
	if err != nil {
		t.Fatalf("two instances of one type were refused: %v", err)
	}
}

func mustDescribe[T any](t *testing.T, prefix string) []fieldSpec {
	t.Helper()

	fields, err := compileSchema(reflect.TypeFor[T](), prefix)
	if err != nil {
		t.Fatal(err)
	}

	return fields
}

func TestUnknownVariableSuggestions(t *testing.T) {
	for _, scenario := range []struct{ name, suggestion string }{
		{"APP_HSOT", "APP_HOST"},
		{"APP_XOST", "APP_COST"},
		{"APP_A_VERY_LONG_VARIABLE_NAMF", "APP_A_VERY_LONG_VARIABLE_NAME"},
		{"APP_SOMETHING_ENTIRELY_ELSE", ""},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			for range 20 {
				_, err := Load[struct {
					Host     string `env:"HOST"`
					Cost     string `env:"COST"`
					Most     string `env:"MOST"`
					LongName string `env:"A_VERY_LONG_VARIABLE_NAME"`
				}]("app", WithEnv(map[string]string{scenario.name: "value"}))
				if err == nil {
					t.Fatal("unknown variable accepted")
				}

				if len(scenario.suggestion) == 0 {
					if strings.Contains(err.Error(), "did you mean") {
						t.Fatal(err)
					}
				} else if !strings.Contains(err.Error(), "did you mean \""+scenario.suggestion+"\"") {
					t.Fatal(err)
				}
			}
		})
	}
}
