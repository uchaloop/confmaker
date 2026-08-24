package confx

import (
	"errors"
	"testing"

	"github.com/uchaloop/secret/v2"
	"github.com/uchaloop/utilfx"
	"go.uber.org/fx"
)

// widgetConfig is a stand-in for a library's typed config: an ordinary field, a
// field without an env tag that keeps its zero value, and a required secret.
type widgetConfig struct {
	Endpoint string `env:"ENDPOINT"`
	Label    string
	Limit    int32         `env:"LIMIT"`
	Token    secret.Secret `env:"TOKEN,required"`
}

func (c widgetConfig) Validate() error {
	if len(c.Endpoint) == 0 {
		return errors.New("endpoint is required")
	}

	return nil
}

// runProvide builds a default (untagged) widgetConfig and returns it.
func runProvide(t *testing.T, name string, opts ...Option) (widgetConfig, error) {
	t.Helper()

	var got widgetConfig
	app := fx.New(
		fx.NopLogger,
		Provide[widgetConfig](name, opts...),
		fx.Invoke(func(cfg widgetConfig) { got = cfg }),
	)

	return got, app.Err()
}

func TestProvideReadsPrefixedEnv(t *testing.T) {
	t.Setenv("ALPHA_ENDPOINT", "alpha:9000")
	t.Setenv("ALPHA_LIMIT", "20")
	t.Setenv("ALPHA_TOKEN", "s3cr3t")

	cfg, err := runProvide(t, "alpha")
	if err != nil {
		t.Fatalf("build: %v", err)
	}

	if cfg.Endpoint != "alpha:9000" {
		t.Errorf("endpoint = %q, want alpha:9000", cfg.Endpoint)
	}
	if cfg.Limit != 20 {
		t.Errorf("limit = %d, want 20", cfg.Limit)
	}
	if cfg.Token.Reveal() != "s3cr3t" {
		t.Error("token not filled from ALPHA_TOKEN")
	}
}

func TestProvideLeavesUntaggedFieldsZero(t *testing.T) {
	t.Setenv("ALPHA_ENDPOINT", "alpha:9000")
	t.Setenv("ALPHA_TOKEN", "s3cr3t")
	t.Setenv("ALPHA_LABEL", "primary")

	cfg, err := runProvide(t, "alpha")
	if err != nil {
		t.Fatalf("build: %v", err)
	}

	if len(cfg.Label) != 0 {
		t.Fatalf("label = %q, want zero: a field without an env tag is not filled", cfg.Label)
	}
}

func TestProvidePerInstancePrefix(t *testing.T) {
	t.Setenv("ALPHA_ENDPOINT", "alpha:9000")
	t.Setenv("ALPHA_TOKEN", "alpha-secret")
	t.Setenv("BETA_ENDPOINT", "beta:9000")
	t.Setenv("BETA_TOKEN", "beta-secret")

	alpha, err := runProvide(t, "alpha")
	if err != nil {
		t.Fatalf("build alpha: %v", err)
	}
	beta, err := runProvide(t, "beta")
	if err != nil {
		t.Fatalf("build beta: %v", err)
	}

	if alpha.Token.Reveal() != "alpha-secret" || beta.Token.Reveal() != "beta-secret" {
		t.Fatalf("instances read each other's variables: alpha=%q beta=%q",
			alpha.Endpoint, beta.Endpoint)
	}
}

func TestProvideNamedTagsInstance(t *testing.T) {
	t.Setenv("BETA_ENDPOINT", "beta:9000")
	t.Setenv("BETA_TOKEN", "beta-secret")

	var got widgetConfig
	app := fx.New(
		fx.NopLogger,
		ProvideNamed[widgetConfig]("beta"),
		fx.Invoke(fx.Annotate(
			func(cfg widgetConfig) { got = cfg },
			fx.ParamTags(utilfx.NameTag("beta")),
		)),
	)
	if app.Err() != nil {
		t.Fatalf("app: %v", app.Err())
	}

	if got.Endpoint != "beta:9000" {
		t.Fatalf("tagged config not filled: %q", got.Endpoint)
	}
}

func TestProvideNamedAndDefaultCoexist(t *testing.T) {
	t.Setenv("MAIN_ENDPOINT", "main:9000")
	t.Setenv("MAIN_TOKEN", "main-secret")
	t.Setenv("REPLICA_ENDPOINT", "replica:9000")
	t.Setenv("REPLICA_TOKEN", "replica-secret")

	var (
		main    widgetConfig
		replica widgetConfig
	)
	app := fx.New(
		fx.NopLogger,
		Provide[widgetConfig]("main"),
		ProvideNamed[widgetConfig]("replica"),
		fx.Invoke(fx.Annotate(
			func(m, r widgetConfig) { main, replica = m, r },
			fx.ParamTags("", utilfx.NameTag("replica")),
		)),
	)
	if app.Err() != nil {
		t.Fatalf("app: %v", app.Err())
	}

	if main.Endpoint != "main:9000" || replica.Endpoint != "replica:9000" {
		t.Fatalf("instances crossed: main=%q replica=%q", main.Endpoint, replica.Endpoint)
	}
}

func TestProvideDerivesPrefixFromDashedName(t *testing.T) {
	t.Setenv("READ_REPLICA_ENDPOINT", "read:9000")
	t.Setenv("READ_REPLICA_TOKEN", "s3cr3t")

	cfg, err := runProvide(t, "read-replica")
	if err != nil {
		t.Fatalf("build: %v", err)
	}

	if cfg.Endpoint != "read:9000" {
		t.Fatalf("endpoint = %q, want the dashed name mapped to READ_REPLICA_", cfg.Endpoint)
	}
}

func TestProvideWithPrefixOverridesName(t *testing.T) {
	t.Setenv("REPORTING_ENDPOINT", "reporting:9000")
	t.Setenv("REPORTING_TOKEN", "s3cr3t")

	cfg, err := runProvide(t, "analytics", WithPrefix("REPORTING_"))
	if err != nil {
		t.Fatalf("build: %v", err)
	}

	if cfg.Endpoint != "reporting:9000" {
		t.Fatalf("endpoint = %q, want the overridden prefix", cfg.Endpoint)
	}
}

func TestProvideWithEmptyPrefixReadsBareNames(t *testing.T) {
	t.Setenv("ENDPOINT", "bare:9000")
	t.Setenv("TOKEN", "s3cr3t")

	cfg, err := runProvide(t, "alpha", WithPrefix(""))
	if err != nil {
		t.Fatalf("build: %v", err)
	}

	if cfg.Endpoint != "bare:9000" {
		t.Fatalf("endpoint = %q, want the unprefixed variable", cfg.Endpoint)
	}
}

func TestProvideMissingRequiredSecret(t *testing.T) {
	t.Setenv("ALPHA_ENDPOINT", "alpha:9000")
	// ALPHA_TOKEN deliberately unset.

	_, err := runProvide(t, "alpha")
	if err == nil {
		t.Fatal("expected an error when the required secret is unset")
	}
}

func TestProvideReportsValidationFailure(t *testing.T) {
	t.Setenv("ALPHA_TOKEN", "s3cr3t")
	// ALPHA_ENDPOINT deliberately unset, so Validate fails.

	_, err := runProvide(t, "alpha")
	if err == nil {
		t.Fatal("expected Validate to fail the build")
	}
}

// ptrValidatedConfig declares Validate on a pointer receiver; build must still
// call it (regression test for checking &cfg, not cfg).
type ptrValidatedConfig struct {
	Endpoint string `env:"ENDPOINT"`
}

func (c *ptrValidatedConfig) Validate() error {
	return errors.New("always invalid")
}

func TestProvideCallsPointerReceiverValidate(t *testing.T) {
	t.Setenv("SERVICE_ENDPOINT", "e")

	app := fx.New(
		fx.NopLogger,
		Provide[ptrValidatedConfig]("service"),
		fx.Invoke(func(ptrValidatedConfig) {}),
	)
	if app.Err() == nil {
		t.Fatal("expected pointer-receiver Validate to run and fail the build")
	}
}

// validatedAfterEnv fails validation unless the environment was applied first.
type validatedAfterEnv struct {
	Endpoint string `env:"ENDPOINT"`
}

func (c validatedAfterEnv) Validate() error {
	if c.Endpoint != "valid" {
		return errors.New("endpoint was not populated before validation")
	}

	return nil
}

func TestProvideValidatesAfterEnv(t *testing.T) {
	t.Setenv("SERVICE_ENDPOINT", "valid")

	app := fx.New(
		fx.NopLogger,
		Provide[validatedAfterEnv]("service"),
		fx.Invoke(func(validatedAfterEnv) {}),
	)
	if app.Err() != nil {
		t.Fatalf("app: %v", app.Err())
	}
}

func TestProvideNestedStructPrefix(t *testing.T) {
	type poolConfig struct {
		MaxConns int32 `env:"MAX_CONNS"`
	}
	type config struct {
		Host string     `env:"HOST"`
		Pool poolConfig `envPrefix:"POOL_"`
	}

	t.Setenv("POSTGRES_HOST", "db:5432")
	t.Setenv("POSTGRES_POOL_MAX_CONNS", "4")

	var got config
	app := fx.New(
		fx.NopLogger,
		Provide[config]("postgres"),
		fx.Invoke(func(cfg config) { got = cfg }),
	)
	if app.Err() != nil {
		t.Fatalf("app: %v", app.Err())
	}

	if got.Pool.MaxConns != 4 {
		t.Fatalf("pool.max_conns = %d, want 4 from POSTGRES_POOL_MAX_CONNS", got.Pool.MaxConns)
	}
}

func TestProvideSecretWithoutEnvTagRemainsZero(t *testing.T) {
	type config struct {
		Endpoint    string        `env:"ENDPOINT,required"`
		FromEnv     secret.Secret `env:"PASSWORD"`
		WithoutEnv  secret.Secret `json:"password" yaml:"password"`
		WithoutTags secret.Secret
	}

	t.Setenv("CUSTOM_ENDPOINT", "localhost:9000")
	t.Setenv("CUSTOM_PASSWORD", "from-env")

	var got config
	app := fx.New(
		fx.NopLogger,
		Provide[config]("ignored", WithPrefix("CUSTOM_")),
		fx.Invoke(func(cfg config) { got = cfg }),
	)
	if app.Err() != nil {
		t.Fatalf("app: %v", app.Err())
	}

	if got.FromEnv.Reveal() != "from-env" {
		t.Fatal("Secret with an env tag was not populated")
	}
	if !got.WithoutEnv.IsZero() || !got.WithoutTags.IsZero() {
		t.Fatal("Secret without an env tag was populated")
	}
}
