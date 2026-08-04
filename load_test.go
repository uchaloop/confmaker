package confmaker

import (
	"os"
	"path/filepath"
	"testing"
	"time"
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
