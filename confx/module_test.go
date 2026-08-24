package confx

import (
	"bytes"
	"strings"
	"testing"

	"github.com/uchaloop/secret/v2"
	"go.uber.org/fx"
)

// strictConfig is the config the strict-check tests register.
type strictConfig struct {
	Host string        `env:"HOST"`
	Port int           `env:"PORT" envDefault:"5432"`
	Pass secret.Secret `env:"PASSWORD,required"`
}

// runModule builds an app with Module and one postgres instance, and returns the
// resulting error.
func runModule(t *testing.T, opts ...ModuleOption) error {
	t.Helper()

	return fx.New(
		fx.NopLogger,
		Module(opts...),
		Provide[strictConfig]("postgres"),
		fx.Invoke(func(strictConfig) {}),
	).Err()
}

func TestModuleAcceptsDeclaredVariables(t *testing.T) {
	t.Setenv("POSTGRES_HOST", "db:5432")
	t.Setenv("POSTGRES_PASSWORD", "s3cr3t")

	if err := runModule(t); err != nil {
		t.Fatalf("declared variables rejected: %v", err)
	}
}

func TestModuleRejectsTypoUnderOwnPrefix(t *testing.T) {
	t.Setenv("POSTGRES_HOST", "db:5432")
	t.Setenv("POSTGRES_PASSWORD", "s3cr3t")
	t.Setenv("POSTGRES_HSOT", "typo")

	err := runModule(t)
	if err == nil {
		t.Fatal("expected the typo to fail the start")
	}
	if !strings.Contains(err.Error(), "POSTGRES_HSOT") {
		t.Fatalf("error does not name the variable: %v", err)
	}
	if !strings.Contains(err.Error(), `did you mean "POSTGRES_HOST"`) {
		t.Fatalf("error does not suggest the intended variable: %v", err)
	}
}

func TestModuleIgnoresForeignPrefixes(t *testing.T) {
	t.Setenv("POSTGRES_HOST", "db:5432")
	t.Setenv("POSTGRES_PASSWORD", "s3cr3t")
	t.Setenv("KAFKA_BROKERS", "not ours")

	if err := runModule(t); err != nil {
		t.Fatalf("a variable outside the application's prefixes was reported: %v", err)
	}
}

func TestModuleAllowUnknownExemptsPrefix(t *testing.T) {
	t.Setenv("POSTGRES_HOST", "db:5432")
	t.Setenv("POSTGRES_PASSWORD", "s3cr3t")
	t.Setenv("POSTGRES_EXPORTER_URL", "sidecar")

	if err := runModule(t, AllowUnknown("POSTGRES_EXPORTER_")); err != nil {
		t.Fatalf("exempted prefix still reported: %v", err)
	}
}

func TestModuleChecksEveryInstance(t *testing.T) {
	t.Setenv("MAIN_HOST", "db:5432")
	t.Setenv("MAIN_PASSWORD", "s3cr3t")
	t.Setenv("REPLICA_HOST", "replica:5432")
	t.Setenv("REPLICA_PASSWORD", "s3cr3t")
	t.Setenv("REPLICA_PROT", "typo")

	err := fx.New(
		fx.NopLogger,
		Module(),
		Provide[strictConfig]("main"),
		ProvideNamed[strictConfig]("replica"),
		fx.Invoke(func(strictConfig) {}),
	).Err()
	if err == nil || !strings.Contains(err.Error(), "REPLICA_PROT") {
		t.Fatalf("a named instance was not covered by the check: %v", err)
	}
}

func TestModuleIgnoresInstanceWithEmptyPrefix(t *testing.T) {
	t.Setenv("HOST", "db:5432")
	t.Setenv("PASSWORD", "s3cr3t")
	t.Setenv("SOMETHING_ELSE", "unrelated")

	err := fx.New(
		fx.NopLogger,
		Module(),
		Provide[strictConfig]("postgres", WithPrefix("")),
		fx.Invoke(func(strictConfig) {}),
	).Err()
	if err != nil {
		t.Fatalf("an instance with no prefix must not claim the whole environment: %v", err)
	}
}

func TestModuleNestedPrefixesDoNotCollide(t *testing.T) {
	type outer struct {
		BaseURL string `env:"BASE_URL"`
	}

	// OZON_ and OZON_CARD_STATUS_ overlap: a variable of the longer instance must
	// not be reported as unknown for the shorter one.
	t.Setenv("OZON_BASE_URL", "https://api")
	t.Setenv("OZON_CARD_STATUS_BASE_URL", "https://api/cards")

	err := fx.New(
		fx.NopLogger,
		Module(),
		Provide[outer]("ozon"),
		ProvideNamed[outer]("ozon_card_status"),
		fx.Invoke(func(outer) {}),
	).Err()
	if err != nil {
		t.Fatalf("overlapping prefixes reported a false positive: %v", err)
	}
}

func TestWithDumpListsVariablesAndMasksSecrets(t *testing.T) {
	t.Setenv("POSTGRES_HOST", "db:5432")
	t.Setenv("POSTGRES_PASSWORD", "s3cr3t")

	var out bytes.Buffer

	err := fx.New(
		fx.NopLogger,
		Module(WithDump(&out)),
		Provide[strictConfig]("postgres"),
		fx.Invoke(func(strictConfig) {}),
	).Err()
	if err != nil {
		t.Fatalf("app: %v", err)
	}

	dump := out.String()
	for _, want := range []string{
		"POSTGRES_HOST", "db:5432",
		"POSTGRES_PORT", "5432", "envDefault",
		"POSTGRES_PASSWORD", "(set)",
	} {
		if !strings.Contains(dump, want) {
			t.Errorf("dump is missing %q:\n%s", want, dump)
		}
	}
	if strings.Contains(dump, "s3cr3t") {
		t.Fatalf("dump printed a secret value:\n%s", dump)
	}
}

func TestModuleHintIsDeterministic(t *testing.T) {
	// Three candidates sit at the same edit distance from the typo, so an
	// unordered scan of the known names would report a different one per run.
	known := map[string]bool{"APP_HOST": true, "APP_MOST": true, "APP_COST": true}

	first := hint("APP_XOST", known)
	for range 100 {
		if got := hint("APP_XOST", known); got != first {
			t.Fatalf("hint changed between runs: %q then %q", first, got)
		}
	}
	if !strings.Contains(first, "APP_COST") {
		t.Fatalf("hint = %q, want the first candidate by name among the ties", first)
	}
}

func TestAllowUnknownIgnoresEmptyPrefix(t *testing.T) {
	t.Setenv("POSTGRES_HOST", "db:5432")
	t.Setenv("POSTGRES_PASSWORD", "s3cr3t")
	t.Setenv("POSTGRES_HSOT", "typo")

	// An empty prefix matches everything; honouring it would turn the check off
	// without saying so.
	err := runModule(t, AllowUnknown(""))
	if err == nil || !strings.Contains(err.Error(), "POSTGRES_HSOT") {
		t.Fatalf("an empty prefix silently disabled the check: %v", err)
	}
}

func TestModuleLeavesNonStructConfigToItsConstructor(t *testing.T) {
	t.Setenv("THING_HOST", "db:5432")

	// A pointer T is a mistake in the code, not in the environment. The check
	// must not bury the parser's own report under a list of "unknown" variables.
	err := fx.New(
		fx.NopLogger,
		Module(),
		Provide[*strictConfig]("thing"),
		fx.Invoke(func(*strictConfig) {}),
	).Err()
	if err == nil {
		t.Fatal("expected a pointer config to fail")
	}
	if strings.Contains(err.Error(), "unknown configuration variable") {
		t.Fatalf("the environment was blamed for a mistake in the code: %v", err)
	}
}

func TestModuleSkipsOpenConfig(t *testing.T) {
	type shard struct {
		Host string `env:"HOST"`
	}
	type config struct {
		Name   string `env:"NAME"`
		Shards []shard
	}

	t.Setenv("CLUSTER_NAME", "main")
	t.Setenv("CLUSTER_SHARDS_0_HOST", "shard-0:5432")

	err := fx.New(
		fx.NopLogger,
		Module(),
		Provide[config]("cluster"),
		fx.Invoke(func(config) {}),
	).Err()
	if err != nil {
		t.Fatalf("a config with per-element variables must not be scanned: %v", err)
	}
}
