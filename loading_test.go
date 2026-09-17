package confmaker

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/uchaloop/secret/v2"
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

// loadWidget loads a widgetConfig under the instance name from env.
func loadWidget(env map[string]string, name string, opts ...ConfigOption) (widgetConfig, error) {
	loadOpts := []LoadOption{WithName(name), WithEnv(env)}
	for _, opt := range opts {
		loadOpts = append(loadOpts, opt)
	}

	return Load[widgetConfig](loadOpts...)
}

func TestLoadReadsPrefixedVariables(t *testing.T) {
	t.Parallel()

	env := map[string]string{
		"ALPHA_ENDPOINT": "alpha:9000",
		"ALPHA_LIMIT":    "20",
		"ALPHA_TOKEN":    "s3cr3t",
	}

	cfg, err := loadWidget(env, "alpha")
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

func TestLoadLeavesUntaggedFieldsZero(t *testing.T) {
	t.Parallel()

	env := map[string]string{
		"ALPHA_ENDPOINT": "alpha:9000",
		"ALPHA_TOKEN":    "s3cr3t",
		"ALPHA_LABEL":    "primary",
	}

	// ALPHA_LABEL is reported as unknown, which is the point: nothing reads it.
	cfg, err := Load[widgetConfig](AllowUnknown("ALPHA_LABEL"), WithEnv(env), WithName("alpha"))
	if err != nil {
		t.Fatalf("build: %v", err)
	}

	if len(cfg.Label) != 0 {
		t.Fatalf("label = %q, want zero: a field without an env tag is not filled", cfg.Label)
	}
}

func TestInstancesReadTheirOwnPrefix(t *testing.T) {
	t.Parallel()

	env := map[string]string{
		"ALPHA_ENDPOINT": "alpha:9000",
		"ALPHA_TOKEN":    "alpha-secret",
		"BETA_ENDPOINT":  "beta:9000",
		"BETA_TOKEN":     "beta-secret",
	}

	alpha, err := loadWidget(env, "alpha")
	if err != nil {
		t.Fatalf("build alpha: %v", err)
	}

	beta, err := loadWidget(env, "beta")
	if err != nil {
		t.Fatalf("build beta: %v", err)
	}

	if alpha.Token.Reveal() != "alpha-secret" || beta.Token.Reveal() != "beta-secret" {
		t.Fatalf("instances read each other's variables: alpha=%q beta=%q",
			alpha.Endpoint, beta.Endpoint)
	}
}

func TestPrefixFromDashedName(t *testing.T) {
	t.Parallel()

	env := map[string]string{
		"READ_REPLICA_ENDPOINT": "read:9000",
		"READ_REPLICA_TOKEN":    "s3cr3t",
	}

	cfg, err := loadWidget(env, "read-replica")
	if err != nil {
		t.Fatalf("build: %v", err)
	}

	if cfg.Endpoint != "read:9000" {
		t.Fatalf("endpoint = %q, want the dashed name mapped to READ_REPLICA_", cfg.Endpoint)
	}
}

func TestWithPrefixOverridesName(t *testing.T) {
	t.Parallel()

	env := map[string]string{
		"CONFXREPORTING_ENDPOINT": "reporting:9000",
		"CONFXREPORTING_TOKEN":    "s3cr3t",
	}

	cfg, err := loadWidget(env, "analytics", WithPrefix("CONFXREPORTING_"))
	if err != nil {
		t.Fatalf("build: %v", err)
	}

	if cfg.Endpoint != "reporting:9000" {
		t.Fatalf("endpoint = %q, want the overridden prefix", cfg.Endpoint)
	}
}

func TestInvalidPrefixIsRefused(t *testing.T) {
	t.Parallel()

	cases := map[string]string{
		"empty":           "",
		"no trailing _":   "REPORTING",
		"lower case":      "reporting_",
		"stray character": "REPORTING-",
	}

	for name, prefix := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := loadWidget(nil, "alpha", WithPrefix(prefix)); err == nil {
				t.Fatalf("prefix %q was accepted", prefix)
			}
		})
	}
}

func TestRequiredSecretMustBeSet(t *testing.T) {
	t.Parallel()

	env := map[string]string{
		"ALPHA_ENDPOINT": "alpha:9000",
	}

	// ALPHA_TOKEN deliberately unset.

	_, err := loadWidget(env, "alpha")
	if err == nil {
		t.Fatal("expected an error when the required secret is unset")
	}
}

func TestValidateFailureFailsLoad(t *testing.T) {
	t.Parallel()

	env := map[string]string{
		"ALPHA_TOKEN": "s3cr3t",
	}

	// ALPHA_ENDPOINT deliberately unset, so Validate fails.

	_, err := loadWidget(env, "alpha")
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

func TestPointerReceiverValidateRuns(t *testing.T) {
	t.Parallel()

	env := map[string]string{
		"SERVICE_ENDPOINT": "e",
	}

	if _, err := Load[ptrValidatedConfig](WithEnv(env), WithName("service")); err == nil {
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

func TestValidateRunsAfterEnvironment(t *testing.T) {
	t.Parallel()

	env := map[string]string{
		"SERVICE_ENDPOINT": "valid",
	}

	if _, err := Load[validatedAfterEnv](WithEnv(env), WithName("service")); err != nil {
		t.Fatalf("load: %v", err)
	}
}

func TestNestedStructPrefix(t *testing.T) {
	t.Parallel()

	env := map[string]string{
		"CONFXPOSTGRES_HOST":           "db:5432",
		"CONFXPOSTGRES_POOL_MAX_CONNS": "4",
	}

	type poolConfig struct {
		MaxConns int32 `env:"MAX_CONNS"`
	}

	type config struct {
		Host string     `env:"HOST"`
		Pool poolConfig `envPrefix:"POOL_"`
	}

	got, err := Load[config](WithEnv(env), WithName("confxpostgres"))
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	if got.Pool.MaxConns != 4 {
		t.Fatalf("pool.max_conns = %d, want 4 from POSTGRES_POOL_MAX_CONNS", got.Pool.MaxConns)
	}
}

func TestSecretWithoutEnvTagStaysZero(t *testing.T) {
	t.Parallel()

	env := map[string]string{
		"CUSTOM_ENDPOINT": "localhost:9000",
		"CUSTOM_PASSWORD": "from-env",
	}

	type config struct {
		Endpoint    string        `env:"ENDPOINT,required"`
		FromEnv     secret.Secret `env:"PASSWORD"`
		WithoutEnv  secret.Secret `json:"password" yaml:"password"`
		WithoutTags secret.Secret
	}

	got, err := Load[config](WithEnv(env), WithName("ignored"), WithPrefix("CUSTOM_"))
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	if got.FromEnv.Reveal() != "from-env" {
		t.Fatal("Secret with an env tag was not populated")
	}

	if !got.WithoutEnv.IsZero() || !got.WithoutTags.IsZero() {
		t.Fatal("Secret without an env tag was populated")
	}
}

// defaultedConfig establishes its own defaults, so the three branches of the
// rule are visible: unset leaves the default, set overrides it, and an empty
// string field set empty becomes "".
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
	t.Parallel()

	// Nothing is set at all.
	cfg, err := Load[defaultedConfig](WithName("confxapp"), WithEnv(nil))
	if err != nil {
		t.Fatalf("fill: %v", err)
	}

	if cfg.Host != "localhost:5432" || cfg.Timeout != 30*time.Second || cfg.Retries != 3 {
		t.Fatalf("an unset variable overwrote its default: %+v", cfg)
	}
}

func TestSetVariableOverridesTheDefault(t *testing.T) {
	t.Parallel()

	env := map[string]string{
		"CONFXAPP_HOST":    "db:5432",
		"CONFXAPP_RETRIES": "5",
	}

	cfg, err := Load[defaultedConfig](WithName("confxapp"), WithEnv(env))
	if err != nil {
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
	t.Parallel()

	env := map[string]string{
		"CONFXAPP_HOST": "",
	}

	cfg, err := Load[defaultedConfig](WithName("confxapp"), WithEnv(env))
	if err != nil {
		t.Fatalf("fill: %v", err)
	}

	if len(cfg.Host) != 0 {
		t.Fatalf("a variable set to empty kept its default: %q", cfg.Host)
	}
}

func TestNotEmptyRejectsAnEmptyVariable(t *testing.T) {
	t.Parallel()

	env := map[string]string{
		"CONFXAPP_HOST": "",
	}

	type config struct {
		Host string `env:"HOST,notEmpty"`
	}

	_, err := Load[config](WithName("confxapp"), WithEnv(env))
	if err == nil {
		t.Fatal("an empty value passed notEmpty")
	}
}

func TestRequiredIsAboutTheVariableNotTheValue(t *testing.T) {
	t.Parallel()

	type config struct {
		Host string `env:"HOST,required"`
	}

	fields, err := compileSchema(reflect.TypeFor[config](), "CONFXAPP_")
	if err != nil {
		t.Fatal(err)
	}

	// A default does not satisfy required: the deployment still has to supply it.
	cfg := config{Host: "seeded"}
	if err := applyAndValidate(&cfg, fields, "confxapp", nil); err == nil {
		t.Fatal("a seeded value satisfied required")
	}
}

func TestLoadReportsEveryFieldProblemAtOnce(t *testing.T) {
	t.Parallel()

	env := map[string]string{
		"CONFXAPP_RETRIES": "many",
	}

	type config struct {
		Host    string `env:"HOST,required"`
		Retries int    `env:"RETRIES"`
	}

	_, err := Load[config](WithName("confxapp"), WithEnv(env))
	if err == nil {
		t.Fatal("expected both problems to fail the build")
	}

	if !strings.Contains(err.Error(), "CONFXAPP_HOST") || !strings.Contains(err.Error(), "CONFXAPP_RETRIES") {
		t.Fatalf("only one of two problems was reported: %v", err)
	}
}

func TestSecretReadsAnyText(t *testing.T) {
	t.Parallel()

	env := map[string]string{
		"CONFXAPP_COUNT": "s3cr3t",
	}

	type config struct {
		Count secret.Secret `env:"COUNT"`
	}

	cfg, err := Load[config](WithName("confxapp"), WithEnv(env))
	if err != nil {
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
