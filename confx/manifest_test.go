package confx

import (
	"reflect"
	"strings"
	"testing"

	"github.com/uchaloop/secret/v2"
	"go.uber.org/fx"
)

// manifestConfig covers what a generator has to render: a plain field, a nested
// struct, a declared default, and a required secret.
type manifestConfig struct {
	Host string `env:"HOST"`
	Pool struct {
		MaxConns int32 `env:"MAX_CONNS" envDefault:"2"`
	} `envPrefix:"POOL_"`
	Password secret.Secret `env:"PASSWORD,required"`
}

func TestManifestListsVariablesInDeclarationOrder(t *testing.T) {
	variables := Manifest[manifestConfig]("postgres")

	want := []string{"POSTGRES_HOST", "POSTGRES_POOL_MAX_CONNS", "POSTGRES_PASSWORD"}
	if len(variables) != len(want) {
		t.Fatalf("got %d variables, want %d: %v", len(variables), len(want), names(variables))
	}

	for i, name := range want {
		if variables[i].Name != name {
			t.Errorf("variable %d = %q, want %q", i, variables[i].Name, name)
		}
	}
}

func TestManifestReportsFieldMetadata(t *testing.T) {
	byName := make(map[string]Variable)
	for _, variable := range Manifest[manifestConfig]("postgres") {
		byName[variable.Name] = variable
	}

	pool := byName["POSTGRES_POOL_MAX_CONNS"]
	if pool.Default != "2" || !pool.HasDefault {
		t.Errorf("envDefault not reported: %+v", pool)
	}
	if pool.Type != "int32" {
		t.Errorf("type = %q, want int32", pool.Type)
	}

	password := byName["POSTGRES_PASSWORD"]
	if !password.Required || !password.Secret {
		t.Errorf("the secret is not reported as required and secret: %+v", password)
	}

	if byName["POSTGRES_HOST"].Required || byName["POSTGRES_HOST"].Secret {
		t.Error("a plain optional field was reported as required or secret")
	}
}

func TestManifestHonoursWithPrefix(t *testing.T) {
	variables := Manifest[manifestConfig]("analytics", WithPrefix("REPORTING_"))

	if variables[0].Name != "REPORTING_HOST" {
		t.Fatalf("first variable = %q, want the overridden prefix", variables[0].Name)
	}
}

// TestManifestMatchesTheStrictCheck pins Manifest to what Module accepts: every
// name a generator emits has to survive the check that runs at startup.
func TestManifestMatchesTheStrictCheck(t *testing.T) {
	for _, variable := range Manifest[manifestConfig]("postgres") {
		t.Setenv(variable.Name, "1")
	}
	t.Setenv("POSTGRES_HOST", "db:5432")

	err := fx.New(
		fx.NopLogger,
		Module(),
		Provide[manifestConfig]("postgres"),
		fx.Invoke(func(manifestConfig) {}),
	).Err()
	if err != nil {
		t.Fatalf("a variable the manifest lists was rejected at startup: %v", err)
	}
}

// TestManifestRendersEnvExample shows the generator case the accessor exists
// for: names and defaults are enough to write a .env.example without running the
// application.
func TestManifestRendersEnvExample(t *testing.T) {
	var out strings.Builder

	for _, variable := range Manifest[manifestConfig]("postgres") {
		out.WriteString(variable.Name)
		out.WriteString("=")
		out.WriteString(variable.Default)
		out.WriteString("\n")
	}

	const want = "POSTGRES_HOST=\nPOSTGRES_POOL_MAX_CONNS=2\nPOSTGRES_PASSWORD=\n"
	if out.String() != want {
		t.Fatalf("rendered:\n%s\nwant:\n%s", out.String(), want)
	}
}

// described is the manifest of a T read under prefix, for the walk tests that do
// not care whether the config came out open.
func described[T any](prefix string) []Variable {
	variables, _ := manifest(reflect.TypeFor[T](), prefix)

	return variables
}

func names(variables []Variable) []string {
	out := make([]string, 0, len(variables))
	for _, variable := range variables {
		out = append(out, variable.Name)
	}

	return out
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
		Host      string        `env:"HOST"`
		Pool      pool          `envPrefix:"POOL_"`
		Allocated *pool         `env:",init" envPrefix:"ALLOC_"`
		Pass      secret.Secret `env:"PASSWORD,notEmpty"`
		Skipped   string
	}

	variables := described[config]("APP_")

	got := make(map[string]Variable, len(variables))
	for _, variable := range variables {
		got[variable.Name] = variable
	}

	for _, want := range []string{
		"APP_HOST", "APP_SHARED", "APP_POOL_MAX_CONNS", "APP_ALLOC_MAX_CONNS", "APP_PASSWORD",
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

	for _, variable := range described[config]("APP_") {
		if variable.Name == "APP_IGNORED" {
			t.Fatal("the manifest reports a variable the parser ignores")
		}
	}
}

// TestDescribeMatchesParserOnPointers pins the manifest to what the parser does
// with a pointer field: a plain one stays nil in a config built from its zero
// value, and only init makes the parser allocate and read through it.
func TestDescribeMatchesParserOnPointers(t *testing.T) {
	type pool struct {
		MaxConns int `env:"MAX_CONNS"`
	}
	type config struct {
		Plain     *pool `envPrefix:"PLAIN_"`
		Allocated *pool `env:",init" envPrefix:"ALLOC_"`
	}

	t.Setenv("APP_PLAIN_MAX_CONNS", "2")
	t.Setenv("APP_ALLOC_MAX_CONNS", "4")

	var parsed config
	if err := fillEnv(&parsed, "APP_", "app"); err != nil {
		t.Fatalf("parse: %v", err)
	}

	if parsed.Plain != nil {
		t.Fatal("the parser allocated a pointer declared without init")
	}
	if parsed.Allocated == nil || parsed.Allocated.MaxConns != 4 {
		t.Fatalf("the parser did not read through an init pointer: %+v", parsed.Allocated)
	}

	got := names(described[config]("APP_"))
	if len(got) != 1 || got[0] != "APP_ALLOC_MAX_CONNS" {
		t.Fatalf("manifest = %v, want only the variable the parser reads", got)
	}
}

// TestDescribeMatchesParserOnIgnoredField pins the manifest to env:"-", which
// takes a field out of the parse together with anything nested in it.
func TestDescribeMatchesParserOnIgnoredField(t *testing.T) {
	type pool struct {
		MaxConns int `env:"MAX_CONNS"`
	}
	type config struct {
		Host   string `env:"HOST"`
		Hidden pool   `env:"-" envPrefix:"HIDDEN_"`
	}

	t.Setenv("APP_HOST", "db:5432")
	t.Setenv("APP_HIDDEN_MAX_CONNS", "2")

	var parsed config
	if err := fillEnv(&parsed, "APP_", "app"); err != nil {
		t.Fatalf("parse: %v", err)
	}

	if parsed.Hidden.MaxConns != 0 {
		t.Fatal(`the parser read through a field marked env:"-"`)
	}

	got := names(described[config]("APP_"))
	if len(got) != 1 || got[0] != "APP_HOST" {
		t.Fatalf("manifest = %v, want only APP_HOST", got)
	}
}

func TestDescribeStopsAtSelfReference(t *testing.T) {
	// init is what makes the walk follow the pointer, so this is the shape that
	// would recurse forever without the guard.
	type node struct {
		Name string `env:"NAME"`
		Next *node  `env:",init" envPrefix:"NEXT_"`
	}

	variables := described[node]("")
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

func TestManifestFlagsStructCollectionAsOpen(t *testing.T) {
	type shard struct {
		Host string `env:"HOST"`
	}
	type config struct {
		Name   string `env:"NAME"`
		Shards []shard
	}

	variables, open := manifest(reflect.TypeFor[config](), "CLUSTER_")
	if !open {
		t.Fatal("a slice of structs must mark the config open")
	}
	if len(variables) != 1 || variables[0].Name != "CLUSTER_NAME" {
		t.Fatalf("enumerable variables lost: %v", variables)
	}
}
