package confx

import (
	"bytes"
	"reflect"
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

// Embedded is embedded by an exported name, so the env parser descends into it.
type Embedded struct {
	Shared string `env:"SHARED"`
}

// unexportedEmbedded is embedded by an unexported name. The env parser cannot set
// such a field, so the manifest must not claim its variables either.
type unexportedEmbedded struct {
	Ignored string `env:"IGNORED"`
}

func TestDescribeWalksNestedAndEmbedded(t *testing.T) {
	type pool struct {
		MaxConns int `env:"MAX_CONNS"`
	}
	type config struct {
		Embedded
		unexportedEmbedded
		Host    string        `env:"HOST"`
		Pool    pool          `envPrefix:"POOL_"`
		Pointer *pool         `envPrefix:"PTR_"`
		Pass    secret.Secret `env:"PASSWORD,notEmpty"`
		Skipped string
	}

	variables := describe(typeOf[config](), "APP_")

	got := make(map[string]Variable, len(variables))
	for _, variable := range variables {
		got[variable.Name] = variable
	}

	for _, want := range []string{
		"APP_HOST", "APP_SHARED", "APP_POOL_MAX_CONNS", "APP_PTR_MAX_CONNS", "APP_PASSWORD",
	} {
		if _, ok := got[want]; !ok {
			t.Errorf("describe did not report %q, got %v", want, keys(got))
		}
	}
	if len(got) != 5 {
		t.Errorf("describe reported extra variables: %v", keys(got))
	}
	if !got["APP_PASSWORD"].Secret || !got["APP_PASSWORD"].Required {
		t.Error("the secret was not reported as secret and required")
	}
}

// TestDescribeMatchesParserOnEmbedding pins the manifest to what the env parser
// actually fills: a variable the parser ignores must not appear in the manifest,
// or the strict check would accept a name that has no effect.
func TestDescribeMatchesParserOnEmbedding(t *testing.T) {
	type config struct {
		Embedded
		unexportedEmbedded
	}

	t.Setenv("APP_SHARED", "shared")
	t.Setenv("APP_IGNORED", "ignored")

	var parsed config
	if err := fillEnv(&parsed, "APP_", "app"); err != nil {
		t.Fatalf("parse: %v", err)
	}

	if parsed.Shared != "shared" {
		t.Fatal("the parser did not fill an exported embedded field")
	}
	if len(parsed.Ignored) != 0 {
		t.Fatal("the parser filled an unexported embedded field; the manifest must follow")
	}

	for _, variable := range describe(typeOf[config](), "APP_") {
		if variable.Name == "APP_IGNORED" {
			t.Fatal("the manifest reports a variable the parser ignores")
		}
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

func TestDescribeStopsAtSelfReference(t *testing.T) {
	type node struct {
		Name string `env:"NAME"`
		Next *node
	}

	variables := describe(typeOf[node](), "")
	if len(variables) != 1 || variables[0].Name != "NAME" {
		t.Fatalf("self-referential config was not handled: %v", variables)
	}
}

func keys(m map[string]Variable) []string {
	out := make([]string, 0, len(m))
	for name := range m {
		out = append(out, name)
	}

	return out
}

func typeOf[T any]() reflect.Type {
	return reflect.TypeFor[T]()
}

func TestManifestFlagsStructCollectionAsOpen(t *testing.T) {
	type shard struct {
		Host string `env:"HOST"`
	}
	type config struct {
		Name   string `env:"NAME"`
		Shards []shard
	}

	variables, open := manifest(typeOf[config](), "CLUSTER_")
	if !open {
		t.Fatal("a slice of structs must mark the config open")
	}
	if len(variables) != 1 || variables[0].Name != "CLUSTER_NAME" {
		t.Fatalf("enumerable variables lost: %v", variables)
	}
}
