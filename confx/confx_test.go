package confx

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/uchaloop/secret/v2"
	"go.uber.org/fx"
)

// widgetConfig is a stand-in for a library's typed config: an ordinary field, a
// field without an env tag that keeps its zero value, and a required secret.
type widgetConfig struct {
	Endpoint string `env:"ENDPOINT"`
	Label    string
	Limit    int32         `env:"LIMIT"`
	Token    secret.Secret `env:"TOKEN,require"`
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
			fx.ParamTags(nameTag("beta")),
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
	t.Setenv("CONFXMAIN_ENDPOINT", "main:9000")
	t.Setenv("CONFXMAIN_TOKEN", "main-secret")
	t.Setenv("CONFXREPLICA_ENDPOINT", "replica:9000")
	t.Setenv("CONFXREPLICA_TOKEN", "replica-secret")

	var (
		main    widgetConfig
		replica widgetConfig
	)
	app := fx.New(
		fx.NopLogger,
		Provide[widgetConfig]("confxmain"),
		ProvideNamed[widgetConfig]("confxreplica"),
		fx.Invoke(fx.Annotate(
			func(m, r widgetConfig) { main, replica = m, r },
			fx.ParamTags("", nameTag("confxreplica")),
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
	t.Setenv("CONFXREPORTING_ENDPOINT", "reporting:9000")
	t.Setenv("CONFXREPORTING_TOKEN", "s3cr3t")

	cfg, err := runProvide(t, "analytics", WithPrefix("CONFXREPORTING_"))
	if err != nil {
		t.Fatalf("build: %v", err)
	}

	if cfg.Endpoint != "reporting:9000" {
		t.Fatalf("endpoint = %q, want the overridden prefix", cfg.Endpoint)
	}
}

func TestProvideRejectsAPrefixThatIsNotOne(t *testing.T) {
	cases := map[string]string{
		"empty":           "",
		"no trailing _":   "REPORTING",
		"lower case":      "reporting_",
		"stray character": "REPORTING-",
	}

	for name, prefix := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := runProvide(t, "alpha", WithPrefix(prefix)); err == nil {
				t.Fatalf("prefix %q was accepted", prefix)
			}
		})
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

	t.Setenv("CONFXPOSTGRES_HOST", "db:5432")
	t.Setenv("CONFXPOSTGRES_POOL_MAX_CONNS", "4")

	var got config
	app := fx.New(
		fx.NopLogger,
		Provide[config]("confxpostgres"),
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
		Endpoint    string        `env:"ENDPOINT,require"`
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

// defaultedConfig establishes its own defaults, so the three branches of the
// rule are visible: unset leaves the default, set overrides it, set-empty
// assigns the empty value.
type defaultedConfig struct {
	Host    string        `env:"HOST"`
	Timeout time.Duration `env:"TIMEOUT"`
	Retries int           `env:"RETRIES"`
}

func (c *defaultedConfig) SetDefaults() {
	c.Host = "localhost:5432"
	c.Timeout = 30 * time.Second
	c.Retries = 3
}

func TestSetDefaultsSurvivesAnUnsetVariable(t *testing.T) {
	// Nothing is set at all.
	var cfg defaultedConfig
	if err := fillEnv(&cfg, "CONFXAPP_", "confxapp"); err != nil {
		t.Fatalf("fill: %v", err)
	}

	if cfg.Host != "localhost:5432" || cfg.Timeout != 30*time.Second || cfg.Retries != 3 {
		t.Fatalf("an unset variable overwrote its default: %+v", cfg)
	}
}

func TestSetVariableOverridesTheDefault(t *testing.T) {
	t.Setenv("CONFXAPP_HOST", "db:5432")
	t.Setenv("CONFXAPP_RETRIES", "5")

	var cfg defaultedConfig
	if err := fillEnv(&cfg, "CONFXAPP_", "confxapp"); err != nil {
		t.Fatalf("fill: %v", err)
	}

	if cfg.Host != "db:5432" || cfg.Retries != 5 {
		t.Fatalf("the environment did not override the default: %+v", cfg)
	}
	if cfg.Timeout != 30*time.Second {
		t.Fatalf("an untouched field lost its default: %v", cfg.Timeout)
	}
}

func TestEmptyVariableAssignsTheEmptyValue(t *testing.T) {
	t.Setenv("CONFXAPP_HOST", "")

	var cfg defaultedConfig
	if err := fillEnv(&cfg, "CONFXAPP_", "confxapp"); err != nil {
		t.Fatalf("fill: %v", err)
	}

	if len(cfg.Host) != 0 {
		t.Fatalf("a variable set to empty kept its default: %q", cfg.Host)
	}
}

func TestNotEmptyRejectsAnEmptyVariable(t *testing.T) {
	type config struct {
		Host string `env:"HOST,notEmpty"`
	}

	t.Setenv("CONFXAPP_HOST", "")

	var cfg config
	err := fillEnv(&cfg, "CONFXAPP_", "confxapp")
	if err == nil {
		t.Fatal("an empty value passed notEmpty")
	}
}

func TestRequiredIsAboutTheVariableNotTheValue(t *testing.T) {
	type config struct {
		Host string `env:"HOST,require"`
	}

	// A default does not satisfy required: the deployment still has to supply it.
	var cfg config
	cfg.Host = "seeded"

	if err := fillEnv(&cfg, "CONFXAPP_", "confxapp"); err == nil {
		t.Fatal("a seeded value satisfied required")
	}
}

func TestFillReportsEveryProblemAtOnce(t *testing.T) {
	type config struct {
		Host    string `env:"HOST,require"`
		Retries int    `env:"RETRIES"`
	}

	t.Setenv("CONFXAPP_RETRIES", "many")

	var cfg config
	err := fillEnv(&cfg, "CONFXAPP_", "confxapp")
	if err == nil {
		t.Fatal("expected both problems to fail the build")
	}
	if !strings.Contains(err.Error(), "CONFXAPP_HOST") || !strings.Contains(err.Error(), "CONFXAPP_RETRIES") {
		t.Fatalf("only one of two problems was reported: %v", err)
	}
}

func TestParseErrorNeverEchoesASecret(t *testing.T) {
	type config struct {
		Count secret.Secret `env:"COUNT"`
	}

	t.Setenv("CONFXAPP_COUNT", "s3cr3t")

	var cfg config
	if err := fillEnv(&cfg, "CONFXAPP_", "confxapp"); err != nil {
		t.Fatalf("a secret decodes any text: %v", err)
	}
	if cfg.Count.Reveal() != "s3cr3t" {
		t.Fatal("the secret was not decoded")
	}
}

// TestConfigErrorNamesEveryLine covers what the label exists for: a stage joins
// its problems, the join renders one per line, and a line read on its own still
// says which config it belongs to.
func TestConfigErrorNamesEveryLine(t *testing.T) {
	err := makeConfigError("store", errors.Join(
		errors.New("first problem"),
		errors.New("second problem"),
	))

	want := `config "store": first problem` + "\n" + `config "store": second problem`
	if err.Error() != want {
		t.Fatalf("got:\n%s\nwant:\n%s", err.Error(), want)
	}
}

// TestConfigErrorKeepsTheStagesOwnContext covers a join a stage wrapped in
// context of its own. Labelling the errors behind the join would report the
// name and drop the "pool:" that says where in the config the problem is.
func TestConfigErrorKeepsTheStagesOwnContext(t *testing.T) {
	err := makeConfigError("store", fmt.Errorf("pool: %w", errors.Join(
		errors.New("max_conns must be positive"),
		errors.New("min_conns exceeds max_conns"),
	)))

	for _, want := range []string{
		`config "store": pool: max_conns must be positive`,
		`config "store": min_conns exceeds max_conns`,
	} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("got:\n%s\nwant a line %q", err.Error(), want)
		}
	}
}

// TestConfigErrorStaysInspectable keeps errors.Is reaching through the label and
// into the join behind it, so a caller can still recognise what a stage
// reported.
func TestConfigErrorStaysInspectable(t *testing.T) {
	sentinel := errors.New("sentinel")

	err := makeConfigError("store", errors.Join(errors.New("other"), sentinel))
	if !errors.Is(err, sentinel) {
		t.Fatal("errors.Is does not reach the error behind the label")
	}
}
