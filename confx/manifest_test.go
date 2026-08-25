package confx

import (
	"strings"
	"testing"
	"time"

	"github.com/uchaloop/secret/v2"
	"go.uber.org/fx"
)

// manifestConfig covers what a generator has to render: a plain field, a nested
// struct, a field with a default, and a required secret.
type manifestConfig struct {
	Host string `env:"HOST"`
	Pool struct {
		MaxConns int32 `env:"MAX_CONNS"`
	} `envPrefix:"POOL_"`
	Timeout  time.Duration `env:"TIMEOUT"`
	Labels   map[string]string
	Password secret.Secret `env:"PASSWORD,require"`
}

// SetDefaults is where this config's defaults live, so code and tests see the
// same values the environment starts from.
func (c *manifestConfig) SetDefaults() {
	c.Pool.MaxConns = 2
	c.Timeout = 30 * time.Second
}

// described is the manifest of a T read under prefix, for tests that expect the
// declaration to be valid.
func described[T any](t *testing.T, prefix string) []Variable {
	t.Helper()

	variables, err := manifestOf[T](prefix)
	if err != nil {
		t.Fatalf("bind: %v", err)
	}

	return variables
}

// manifested is Manifest for a declaration the test expects to be valid.
func manifested[T any](t *testing.T, name string, opts ...Option) []Variable {
	t.Helper()

	variables, err := Manifest[T](name, opts...)
	if err != nil {
		t.Fatalf("manifest: %v", err)
	}

	return variables
}

// bindError returns the error a T's declaration produces, failing when it binds
// cleanly.
func bindError[T any](t *testing.T) error {
	t.Helper()

	if _, err := manifestOf[T]("CONFXAPP_"); err != nil {
		return err
	}

	t.Fatal("expected the declaration to be rejected")

	return nil
}

func names(variables []Variable) []string {
	out := make([]string, 0, len(variables))
	for _, variable := range variables {
		out = append(out, variable.Name)
	}

	return out
}

func TestManifestListsVariablesInDeclarationOrder(t *testing.T) {
	got := names(manifested[manifestConfig](t, "confxpostgres"))

	want := []string{
		"CONFXPOSTGRES_HOST", "CONFXPOSTGRES_POOL_MAX_CONNS", "CONFXPOSTGRES_TIMEOUT", "CONFXPOSTGRES_PASSWORD",
	}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}

	for i, name := range want {
		if got[i] != name {
			t.Errorf("variable %d = %q, want %q", i, got[i], name)
		}
	}
}

func TestManifestReportsFieldMetadata(t *testing.T) {
	byName := make(map[string]Variable)
	for _, variable := range manifested[manifestConfig](t, "confxpostgres") {
		byName[variable.Name] = variable
	}

	pool := byName["CONFXPOSTGRES_POOL_MAX_CONNS"]
	if pool.Default != "2" || !pool.HasDefault {
		t.Errorf("the default from SetDefaults was not reported: %+v", pool)
	}
	if pool.Type != "int32" {
		t.Errorf("type = %q, want int32", pool.Type)
	}

	// A duration renders through its own String method, so the default is text
	// the variable could carry back.
	if timeout := byName["CONFXPOSTGRES_TIMEOUT"]; timeout.Default != "30s" {
		t.Errorf("timeout default = %q, want 30s", timeout.Default)
	}

	if host := byName["CONFXPOSTGRES_HOST"]; host.HasDefault || host.Required || host.Secret {
		t.Errorf("a plain field without a default was misreported: %+v", host)
	}

	password := byName["CONFXPOSTGRES_PASSWORD"]
	if !password.Required || !password.Secret {
		t.Errorf("the secret is not reported as required and secret: %+v", password)
	}
}

func TestManifestHonoursWithPrefix(t *testing.T) {
	got := names(manifested[manifestConfig](t, "confxanalytics", WithPrefix("CONFXREPORTING_")))

	if got[0] != "CONFXREPORTING_HOST" {
		t.Fatalf("first variable = %q, want the overridden prefix", got[0])
	}
}

// TestManifestMatchesTheStrictCheck pins Manifest to what Module accepts: every
// name a generator emits has to survive the check that runs at startup.
func TestManifestMatchesTheStrictCheck(t *testing.T) {
	for _, variable := range manifested[manifestConfig](t, "confxpostgres") {
		t.Setenv(variable.Name, "1")
	}
	t.Setenv("CONFXPOSTGRES_HOST", "db:5432")
	t.Setenv("CONFXPOSTGRES_TIMEOUT", "1s")

	err := fx.New(
		fx.NopLogger,
		Module(),
		Provide[manifestConfig]("confxpostgres"),
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

	for _, variable := range manifested[manifestConfig](t, "confxpostgres") {
		out.WriteString(variable.Name)
		out.WriteString("=")
		out.WriteString(variable.Default)
		out.WriteString("\n")
	}

	const want = "CONFXPOSTGRES_HOST=\n" +
		"CONFXPOSTGRES_POOL_MAX_CONNS=2\n" +
		"CONFXPOSTGRES_TIMEOUT=30s\n" +
		"CONFXPOSTGRES_PASSWORD=\n"

	if out.String() != want {
		t.Fatalf("rendered:\n%s\nwant:\n%s", out.String(), want)
	}
}

func TestManifestOmitsSecretValueFromDefault(t *testing.T) {
	type config struct {
		Password secret.Secret `env:"PASSWORD"`
	}

	// A default that is a secret still renders through the type's own text form,
	// which is a mask.
	variables := described[config](t, "CONFXAPP_")
	if strings.Contains(variables[0].Default, "s3cr3t") {
		t.Fatalf("a secret leaked into the manifest: %q", variables[0].Default)
	}
}

// Embedded is embedded by an exported name, so the walk descends into it.
type Embedded struct {
	Shared string `env:"SHARED"`
}

// unexportedEmbedded is embedded by an unexported name. Its fields cannot be
// assigned, so the config does not read them.
type unexportedEmbedded struct {
	Ignored string `env:"IGNORED"`
}

func TestBindWalksNestedAndEmbedded(t *testing.T) {
	type pool struct {
		MaxConns int `env:"MAX_CONNS"`
	}
	type config struct {
		Embedded
		unexportedEmbedded
		Host    string        `env:"HOST"`
		Pool    pool          `envPrefix:"POOL_"`
		Pass    secret.Secret `env:"PASSWORD,notEmpty"`
		Skipped string
	}

	got := make(map[string]Variable)
	for _, variable := range described[config](t, "CONFXAPP_") {
		got[variable.Name] = variable
	}

	for _, want := range []string{"CONFXAPP_SHARED", "CONFXAPP_HOST", "CONFXAPP_POOL_MAX_CONNS", "CONFXAPP_PASSWORD"} {
		if _, ok := got[want]; !ok {
			t.Errorf("the walk did not report %q, got %v", want, keys(got))
		}
	}
	if len(got) != 4 {
		t.Errorf("the walk reported extra variables: %v", keys(got))
	}
	if !got["CONFXAPP_PASSWORD"].Secret || !got["CONFXAPP_PASSWORD"].Required {
		t.Error("the secret was not reported as secret and required")
	}
}

// TestBindFillsWhatItDescribes is the property the single traversal exists for:
// the variables a config reports are exactly the variables that fill it.
func TestBindFillsWhatItDescribes(t *testing.T) {
	type pool struct {
		MaxConns int `env:"MAX_CONNS"`
	}
	type config struct {
		Embedded
		unexportedEmbedded
		Host string `env:"HOST"`
		Pool pool   `envPrefix:"POOL_"`
	}

	for _, variable := range described[config](t, "CONFXAPP_") {
		t.Setenv(variable.Name, "7")
	}
	t.Setenv("CONFXAPP_HOST", "db:5432")
	t.Setenv("CONFXAPP_SHARED", "shared")
	t.Setenv("CONFXAPP_IGNORED", "ignored")

	var parsed config
	if err := fillEnv(&parsed, "CONFXAPP_", "confxapp"); err != nil {
		t.Fatalf("fill: %v", err)
	}

	if parsed.Host != "db:5432" || parsed.Shared != "shared" || parsed.Pool.MaxConns != 7 {
		t.Fatalf("a described variable did not fill its field: %+v", parsed)
	}
	if len(parsed.Ignored) != 0 {
		t.Fatal("a field the manifest omits was filled anyway")
	}
}

func TestBindSkipsIgnoredField(t *testing.T) {
	type pool struct {
		MaxConns int `env:"MAX_CONNS"`
	}
	type config struct {
		Host   string `env:"HOST"`
		Hidden pool   `env:"-" envPrefix:"HIDDEN_"`
	}

	t.Setenv("CONFXAPP_HOST", "db:5432")
	t.Setenv("CONFXAPP_HIDDEN_MAX_CONNS", "2")

	var parsed config
	if err := fillEnv(&parsed, "CONFXAPP_", "confxapp"); err != nil {
		t.Fatalf("fill: %v", err)
	}

	if parsed.Hidden.MaxConns != 0 {
		t.Fatal(`a field marked env:"-" was filled`)
	}
	if got := names(described[config](t, "CONFXAPP_")); len(got) != 1 || got[0] != "CONFXAPP_HOST" {
		t.Fatalf("manifest = %v, want only APP_HOST", got)
	}
}

func TestBindRejectsEnvDefault(t *testing.T) {
	type config struct {
		Timeout time.Duration `env:"TIMEOUT" envDefault:"30s"`
	}

	err := bindError[config](t)
	if !strings.Contains(err.Error(), "SetDefaults") {
		t.Fatalf("the error does not point at the replacement: %v", err)
	}
}

func TestBindRejectsNonStruct(t *testing.T) {
	if err := bindError[*manifestConfig](t); !strings.Contains(err.Error(), "must be a struct") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestBindReportsEveryProblemAtOnce(t *testing.T) {
	type pool struct {
		MaxConns int `env:"MAX_CONNS"`
	}
	type config struct {
		Timeout time.Duration `env:"TIMEOUT" envDefault:"30s"`
		Pool    *pool         `envPrefix:"POOL_"`
	}

	err := bindError[config](t)
	if !strings.Contains(err.Error(), "SetDefaults") || !strings.Contains(err.Error(), "nest by value") {
		t.Fatalf("only one of two problems was reported: %v", err)
	}
}

func keys(m map[string]Variable) []string {
	out := make([]string, 0, len(m))
	for name := range m {
		out = append(out, name)
	}

	return out
}
