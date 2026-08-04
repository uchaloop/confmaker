package confx

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/uchaloop/secret/v2"
	"github.com/uchaloop/utilfx"
	"go.uber.org/fx"
)

// widgetConfig is a stand-in for a library's typed config: open fields from the
// file, endpoint overridable from the env, and a secret that lives only in env.
type widgetConfig struct {
	Endpoint string        `koanf:"endpoint" env:"ENDPOINT"`
	Label    string        `koanf:"label"`
	Limit    int32         `koanf:"limit"`
	Token    secret.Secret `koanf:"-" env:"TOKEN,required"`
}

func (c widgetConfig) Validate() error {
	if len(c.Endpoint) == 0 {
		return errors.New("endpoint is required")
	}

	return nil
}

const sampleTOML = `
[widgets.alpha]
endpoint = "file-endpoint:9000"
label = "primary"
limit = 20

[widgets.beta]
endpoint = "beta:9000"
label = "secondary"
`

func writeTOML(t *testing.T) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte(sampleTOML), 0o600); err != nil {
		t.Fatalf("write toml: %v", err)
	}

	return path
}

// runProvide runs a single Provide against the file and returns the config.
func runProvide(t *testing.T, path, section, name string, opts ...Option) (widgetConfig, error) {
	t.Helper()

	var (
		got    widgetConfig
		gotErr error
	)
	app := fx.New(
		fx.NopLogger,
		LoadModule(path),
		Provide[widgetConfig](section, name, opts...),
		fx.Invoke(fx.Annotate(
			func(cfg widgetConfig) { got = cfg },
			fx.ParamTags(utilfx.NameTag(name)),
		)),
	)
	gotErr = app.Err()

	return got, gotErr
}

func TestProvideDecodesSectionAndSecret(t *testing.T) {
	path := writeTOML(t)
	t.Setenv("ALPHA_TOKEN", "s3cr3t")

	cfg, err := runProvide(t, path, "widgets.alpha", "alpha")
	if err != nil {
		t.Fatalf("build: %v", err)
	}

	if cfg.Endpoint != "file-endpoint:9000" {
		t.Errorf("endpoint = %q, want file value", cfg.Endpoint)
	}
	if cfg.Label != "primary" || cfg.Limit != 20 {
		t.Errorf("label/limit = %q/%d", cfg.Label, cfg.Limit)
	}
	if cfg.Token.Reveal() != "s3cr3t" {
		t.Errorf("token not filled from ALPHA_TOKEN")
	}
}

func TestProvideDefaultIsUntaggedWithSectionPrefix(t *testing.T) {
	path := writeTOML(t)
	t.Setenv("ALPHA_TOKEN", "s3cr3t")

	var got widgetConfig
	app := fx.New(
		fx.NopLogger,
		LoadModule(path),
		ProvideDefault[widgetConfig]("widgets.alpha"), // untagged, prefix from "alpha"
		fx.Invoke(func(cfg widgetConfig) { got = cfg }),
	)
	if app.Err() != nil {
		t.Fatalf("app: %v", app.Err())
	}

	if got.Endpoint != "file-endpoint:9000" || got.Token.Reveal() != "s3cr3t" {
		t.Fatalf("default provide did not fill config: endpoint=%q token set=%v",
			got.Endpoint, got.Token.Reveal() != "")
	}
}

func TestProvideEnvOverridesFileEndpoint(t *testing.T) {
	path := writeTOML(t)
	t.Setenv("ALPHA_TOKEN", "s3cr3t")
	t.Setenv("ALPHA_ENDPOINT", "env-endpoint:6543")

	cfg, err := runProvide(t, path, "widgets.alpha", "alpha")
	if err != nil {
		t.Fatalf("build: %v", err)
	}

	if cfg.Endpoint != "env-endpoint:6543" {
		t.Fatalf("endpoint = %q, want env override", cfg.Endpoint)
	}
}

func TestProvidePerInstancePrefix(t *testing.T) {
	path := writeTOML(t)
	t.Setenv("BETA_TOKEN", "beta-secret")

	cfg, err := runProvide(t, path, "widgets.beta", "beta")
	if err != nil {
		t.Fatalf("build: %v", err)
	}

	if cfg.Token.Reveal() != "beta-secret" {
		t.Fatalf("token not filled from BETA_TOKEN")
	}
}

// ptrValidatedConfig declares Validate on a pointer receiver; build must still
// call it (regression test for checking &cfg, not cfg).
type ptrValidatedConfig struct {
	Endpoint string `koanf:"endpoint"`
}

func (c *ptrValidatedConfig) Validate() error {
	return errors.New("always invalid")
}

type noFileValidatedConfig struct {
	Endpoint string `env:"ENDPOINT"`
}

func (c noFileValidatedConfig) Validate() error {
	if c.Endpoint != "valid" {
		return errors.New("endpoint was not populated before validation")
	}

	return nil
}

func TestProvideCallsPointerReceiverValidate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte("[widgets.alpha]\nendpoint = \"e\"\n"), 0o600); err != nil {
		t.Fatalf("write toml: %v", err)
	}

	app := fx.New(
		fx.NopLogger,
		LoadModule(path),
		ProvideDefault[ptrValidatedConfig]("widgets.alpha"),
		fx.Invoke(func(ptrValidatedConfig) {}),
	)
	if app.Err() == nil {
		t.Fatal("expected pointer-receiver Validate to run and fail the build")
	}
}

func TestProvideMissingRequiredSecret(t *testing.T) {
	path := writeTOML(t)
	// ALPHA_TOKEN deliberately unset.

	_, err := runProvide(t, path, "widgets.alpha", "alpha")
	if err == nil {
		t.Fatal("expected an error when the required secret is unset")
	}
}

func TestProvideMissingSection(t *testing.T) {
	path := writeTOML(t)
	t.Setenv("GAMMA_TOKEN", "x")

	_, err := runProvide(t, path, "widgets.gamma", "gamma")
	if err == nil {
		t.Fatal("expected an error for a missing section")
	}
}

func TestProvideUnknownKeyRejected(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	body := "[widgets.alpha]\nendpoint = \"e\"\nlabel = \"l\"\ntypo = true\n"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write toml: %v", err)
	}
	t.Setenv("ALPHA_TOKEN", "s3cr3t")

	_, err := runProvide(t, path, "widgets.alpha", "alpha")
	if err == nil {
		t.Fatal("expected an error for an unknown key")
	}
}

func TestProvideRejectsSecretKeyExcludedByTag(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	body := "[widgets.alpha]\nendpoint = \"e\"\nlabel = \"l\"\ntoken = \"in-file\"\n"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write toml: %v", err)
	}
	t.Setenv("ALPHA_TOKEN", "s3cr3t")

	// token maps to a koanf:"-" field, so strict decoding still rejects it as an
	// unknown key before type-based secret ignoring applies.
	_, err := runProvide(t, path, "widgets.alpha", "alpha")
	if err == nil {
		t.Fatal("expected an error for a key excluded with koanf:\"-\"")
	}
}

func TestProvideIgnoresSecretFromFileThenReadsEnv(t *testing.T) {
	type config struct {
		Endpoint string        `koanf:"endpoint"`
		Password secret.Secret `koanf:"password" env:"PASSWORD,required"`
	}

	path := filepath.Join(t.TempDir(), "config.toml")
	body := "[service]\nendpoint = \"localhost:9000\"\npassword = \"from-file\"\n"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write toml: %v", err)
	}
	t.Setenv("SERVICE_PASSWORD", "from-env")

	var got config
	app := fx.New(
		fx.NopLogger,
		LoadModule(path),
		ProvideDefault[config]("service"),
		fx.Invoke(func(cfg config) { got = cfg }),
	)
	if app.Err() != nil {
		t.Fatalf("app: %v", app.Err())
	}
	if got.Password.Reveal() != "from-env" {
		t.Fatalf("password = %q, want env value", got.Password.Reveal())
	}
}

func TestProvideIgnoresMappedSecretWithoutEnvTag(t *testing.T) {
	type config struct {
		Endpoint string        `koanf:"endpoint"`
		Password secret.Secret `koanf:"password"`
	}

	path := filepath.Join(t.TempDir(), "config.toml")
	body := "[service]\nendpoint = \"localhost:9000\"\npassword = \"from-file\"\n"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write toml: %v", err)
	}

	var got config
	app := fx.New(
		fx.NopLogger,
		LoadModule(path),
		ProvideDefault[config]("service"),
		fx.Invoke(func(cfg config) { got = cfg }),
	)
	if app.Err() != nil {
		t.Fatalf("app: %v", app.Err())
	}
	if !got.Password.IsZero() {
		t.Fatal("file populated a Secret without an env tag")
	}
}

func TestProvideNoFileDefaultReadsEnvWithoutFile(t *testing.T) {
	// No LoadModule or file: env-tagged fields are filled from the environment.
	t.Setenv("ALPHA_ENDPOINT", "env-only:9000")
	t.Setenv("ALPHA_TOKEN", "s3cr3t")

	var got widgetConfig
	app := fx.New(
		fx.NopLogger,
		ProvideNoFileDefault[widgetConfig]("alpha"),
		fx.Invoke(func(cfg widgetConfig) { got = cfg }),
	)
	if app.Err() != nil {
		t.Fatalf("app: %v", app.Err())
	}
	if got.Endpoint != "env-only:9000" || got.Token.Reveal() != "s3cr3t" {
		t.Fatalf("no-file config not filled: endpoint=%q token set=%v",
			got.Endpoint, got.Token.Reveal() != "")
	}
	if got.Label != "" || got.Limit != 0 {
		t.Fatalf("fields without env tags must remain zero-valued: label=%q limit=%d",
			got.Label, got.Limit)
	}
}

func TestProvideNoFileTagsInstance(t *testing.T) {
	t.Setenv("BETA_ENDPOINT", "beta:9000")
	t.Setenv("BETA_TOKEN", "beta-secret")

	var got widgetConfig
	app := fx.New(
		fx.NopLogger,
		ProvideNoFile[widgetConfig]("beta"),
		fx.Invoke(fx.Annotate(
			func(cfg widgetConfig) { got = cfg },
			fx.ParamTags(utilfx.NameTag("beta")),
		)),
	)
	if app.Err() != nil {
		t.Fatalf("app: %v", app.Err())
	}
	if got.Endpoint != "beta:9000" {
		t.Fatalf("tagged no-file config not filled: %q", got.Endpoint)
	}
}

func TestProvideNoFileMissingRequiredSecret(t *testing.T) {
	t.Setenv("ALPHA_ENDPOINT", "env-only:9000")
	// ALPHA_TOKEN deliberately unset.

	app := fx.New(
		fx.NopLogger,
		ProvideNoFileDefault[widgetConfig]("alpha"),
		fx.Invoke(func(widgetConfig) {}),
	)
	if app.Err() == nil {
		t.Fatal("expected an error when the required secret is unset")
	}
}

func TestProvideNoFileSecretWithoutEnvTagRemainsZero(t *testing.T) {
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
		ProvideNoFileDefault[config]("ignored", WithEnvPrefix("CUSTOM_")),
		fx.Invoke(func(cfg config) { got = cfg }),
	)
	if app.Err() != nil {
		t.Fatalf("app: %v", app.Err())
	}
	if got.Endpoint != "localhost:9000" {
		t.Fatalf("custom prefix was not applied: %q", got.Endpoint)
	}
	if got.FromEnv.Reveal() != "from-env" {
		t.Fatal("Secret with only an env tag was not populated")
	}
	if !got.WithoutEnv.IsZero() || !got.WithoutTags.IsZero() {
		t.Fatal("Secret without an env tag was populated")
	}
}

func TestProvideNoFileValidatesAfterEnv(t *testing.T) {
	t.Setenv("SERVICE_ENDPOINT", "valid")

	var got noFileValidatedConfig
	app := fx.New(
		fx.NopLogger,
		ProvideNoFileDefault[noFileValidatedConfig]("service"),
		fx.Invoke(func(cfg noFileValidatedConfig) { got = cfg }),
	)
	if app.Err() != nil {
		t.Fatalf("app: %v", app.Err())
	}
	if got.Endpoint != "valid" {
		t.Fatalf("endpoint = %q, want valid", got.Endpoint)
	}
}
