package confmaker

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/uchaloop/secret/v2"
)

type serviceSection struct {
	Name            string        `koanf:"name"`
	ShutdownTimeout time.Duration `koanf:"shutdown_timeout"`
}

type limitsSection struct {
	Max int32 `koanf:"max"`
}

type widgetSection struct {
	Endpoint string        `koanf:"endpoint"`
	Aliases  []string      `koanf:"aliases"`
	Limits   limitsSection `koanf:"limits"`
}

type testConfig struct {
	Service serviceSection           `koanf:"service"`
	Widgets map[string]widgetSection `koanf:"widgets"`
}

func writeTOML(t *testing.T, body string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write temp toml: %v", err)
	}

	return path
}

func writeNamedTOML(t *testing.T, dir, name, body string) string {
	t.Helper()

	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}

	return path
}

func TestLoadDecodesTypesAndSections(t *testing.T) {
	path := writeTOML(t, `
[service]
name = "app"
shutdown_timeout = "15s"

[widgets.alpha]
endpoint = "localhost:9000"
aliases = ["a1", "a2"]

[widgets.alpha.limits]
max = 20

[widgets.beta]
endpoint = "localhost:9001"
aliases = []
`)

	var cfg testConfig
	if err := Load(&cfg, path); err != nil {
		t.Fatalf("Load: %v", err)
	}

	if cfg.Service.Name != "app" {
		t.Errorf("service.name = %q, want app", cfg.Service.Name)
	}
	if cfg.Service.ShutdownTimeout != 15*time.Second {
		t.Errorf("service.shutdown_timeout = %v, want 15s", cfg.Service.ShutdownTimeout)
	}

	alpha, ok := cfg.Widgets["alpha"]
	if !ok {
		t.Fatal("widgets.alpha missing")
	}
	if alpha.Endpoint != "localhost:9000" {
		t.Errorf("widgets.alpha.endpoint = %q", alpha.Endpoint)
	}
	if len(alpha.Aliases) != 2 || alpha.Aliases[0] != "a1" {
		t.Errorf("widgets.alpha.aliases = %v", alpha.Aliases)
	}
	if alpha.Limits.Max != 20 {
		t.Errorf("widgets.alpha.limits.max = %d, want 20", alpha.Limits.Max)
	}
	if _, ok := cfg.Widgets["beta"]; !ok {
		t.Error("widgets.beta missing")
	}
}

func TestLoadRejectsUnknownKey(t *testing.T) {
	path := writeTOML(t, `
[service]
name = "app"
shutdown_timout = "15s"
`) // typo: shutdown_timout

	var cfg testConfig
	if err := Load(&cfg, path); err == nil {
		t.Fatal("expected an error for the unknown key, got nil")
	}
}

func TestLoadOverlayOverrides(t *testing.T) {
	base := writeTOML(t, `
[service]
name = "base"
shutdown_timeout = "5s"
`)
	overlay := writeTOML(t, `
[service]
name = "prod"
`)

	var cfg testConfig
	if err := Load(&cfg, base, overlay); err != nil {
		t.Fatalf("Load: %v", err)
	}

	if cfg.Service.Name != "prod" {
		t.Errorf("service.name = %q, want prod (overlay wins)", cfg.Service.Name)
	}
	if cfg.Service.ShutdownTimeout != 5*time.Second {
		t.Errorf("service.shutdown_timeout = %v, want 5s (kept from base)", cfg.Service.ShutdownTimeout)
	}
}

func TestLoadMissingFile(t *testing.T) {
	var cfg testConfig
	if err := Load(&cfg, filepath.Join(t.TempDir(), "nope.toml")); err == nil {
		t.Fatal("expected an error for a missing file, got nil")
	}
}

func TestLoadDefaultsToCurrentDirConfig(t *testing.T) {
	dir := t.TempDir()
	body := "[service]\nname = \"app\"\n"
	if err := os.WriteFile(filepath.Join(dir, "config.toml"), []byte(body), 0o600); err != nil {
		t.Fatalf("write config.toml: %v", err)
	}
	t.Chdir(dir)

	var cfg testConfig
	if err := Load(&cfg); err != nil {
		t.Fatalf("Load with no paths: %v", err)
	}
	if cfg.Service.Name != "app" {
		t.Errorf("service.name = %q, want app (from ./config.toml)", cfg.Service.Name)
	}
}

func TestLoadDirMergesCommonThenEnvironment(t *testing.T) {
	dir := t.TempDir()
	writeNamedTOML(t, dir, "common.toml", `
[service]
name = "common"
shutdown_timeout = "5s"
`)
	writeNamedTOML(t, dir, "prod.toml", `
[service]
name = "production"
`)
	t.Setenv("ENVIRONMENT", "prd")

	var cfg testConfig
	if err := LoadDir(&cfg, dir); err != nil {
		t.Fatalf("LoadDir: %v", err)
	}
	if cfg.Service.Name != "production" {
		t.Fatalf("service.name = %q, want production", cfg.Service.Name)
	}
	if cfg.Service.ShutdownTimeout != 5*time.Second {
		t.Fatalf("service.shutdown_timeout = %v, want common value 5s", cfg.Service.ShutdownTimeout)
	}
}

func TestLoadDirWorksWithoutCommon(t *testing.T) {
	dir := t.TempDir()
	writeNamedTOML(t, dir, "dev.toml", `
[service]
name = "development"
`)
	t.Setenv("ENVIRONMENT", " DEV ")

	var cfg testConfig
	if err := LoadDir(&cfg, dir); err != nil {
		t.Fatalf("LoadDir: %v", err)
	}
	if cfg.Service.Name != "development" {
		t.Fatalf("service.name = %q, want development", cfg.Service.Name)
	}
}

func TestLoadDirRequiresEnvironment(t *testing.T) {
	previous, existed := os.LookupEnv("ENVIRONMENT")
	if err := os.Unsetenv("ENVIRONMENT"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if existed {
			_ = os.Setenv("ENVIRONMENT", previous)
		} else {
			_ = os.Unsetenv("ENVIRONMENT")
		}
	})

	var cfg testConfig
	if err := LoadDir(&cfg, t.TempDir()); err == nil {
		t.Fatal("expected an error when ENVIRONMENT is not set")
	}
}

func TestLoadDirRejectsUnknownEnvironment(t *testing.T) {
	t.Setenv("ENVIRONMENT", "production")

	var cfg testConfig
	if err := LoadDir(&cfg, t.TempDir()); err == nil {
		t.Fatal("expected an error for an unsupported ENVIRONMENT")
	}
}

func TestLoadDirRequiresEnvironmentFile(t *testing.T) {
	t.Setenv("ENVIRONMENT", "stage")

	var cfg testConfig
	if err := LoadDir(&cfg, t.TempDir()); err == nil {
		t.Fatal("expected an error when stage.toml is missing")
	}
}

func TestLoadIgnoresSecretFromFile(t *testing.T) {
	type config struct {
		Name     string        `koanf:"name"`
		Password secret.Secret `koanf:"password"`
	}

	path := writeTOML(t, `
name = "app"
password = "must-not-be-loaded"
`)

	var cfg config
	if err := Load(&cfg, path); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Name != "app" {
		t.Fatalf("name = %q, want app", cfg.Name)
	}
	if cfg.Password.Reveal() != "" {
		t.Fatal("secret was populated from the configuration file")
	}
}

func TestLoadClearsExistingSecretsEvenWhenAbsentFromFile(t *testing.T) {
	type credentials struct {
		Password secret.Secret `env:"PASSWORD"`
	}
	type service struct {
		Credentials credentials `koanf:"credentials"`
	}
	type config struct {
		Services map[string]service `koanf:"services"`
	}

	path := writeTOML(t, `
[services.api]
`)
	cfg := config{
		Services: map[string]service{
			"api": {
				Credentials: credentials{Password: secret.New("previous-value")},
			},
		},
	}

	if err := Load(&cfg, path); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := cfg.Services["api"].Credentials.Password.Reveal(); got != "" {
		t.Fatal("previously initialized secret was not cleared")
	}
}
