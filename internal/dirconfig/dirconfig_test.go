package dirconfig

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveSelectsCanonicalEnvironmentFile(t *testing.T) {
	tests := []struct {
		name        string
		environment string
		wantFile    string
	}{
		{name: "dev", environment: "dev", wantFile: "dev.toml"},
		{name: "stage", environment: "stage", wantFile: "stage.toml"},
		{name: "prod", environment: "prod", wantFile: "prod.toml"},
		{name: "prd alias", environment: "prd", wantFile: "prod.toml"},
		{name: "case and whitespace", environment: " PROD ", wantFile: "prod.toml"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			writeFile(t, filepath.Join(dir, tt.wantFile))
			t.Setenv(environmentVariable, tt.environment)

			paths, err := Resolve(dir)
			if err != nil {
				t.Fatalf("Resolve: %v", err)
			}
			want := filepath.Join(dir, tt.wantFile)
			if len(paths) != 1 || paths[0] != want {
				t.Fatalf("paths = %v, want [%s]", paths, want)
			}
		})
	}
}

func TestResolveLoadsCommonBeforeEnvironment(t *testing.T) {
	dir := t.TempDir()
	common := filepath.Join(dir, "common.toml")
	prod := filepath.Join(dir, "prod.toml")
	writeFile(t, common)
	writeFile(t, prod)
	t.Setenv(environmentVariable, "prod")

	paths, err := Resolve(dir)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if len(paths) != 2 || paths[0] != common || paths[1] != prod {
		t.Fatalf("paths = %v, want [%s %s]", paths, common, prod)
	}
}

func TestResolveDoesNotRequireCommon(t *testing.T) {
	dir := t.TempDir()
	dev := filepath.Join(dir, "dev.toml")
	writeFile(t, dev)
	t.Setenv(environmentVariable, "dev")

	paths, err := Resolve(dir)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if len(paths) != 1 || paths[0] != dev {
		t.Fatalf("paths = %v, want [%s]", paths, dev)
	}
}

func TestResolveRejectsInvalidEnvironment(t *testing.T) {
	tests := []struct {
		name        string
		environment string
	}{
		{name: "empty", environment: ""},
		{name: "unknown", environment: "production"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(environmentVariable, tt.environment)

			_, err := Resolve(t.TempDir())
			if err == nil {
				t.Fatal("expected an error")
			}
			if !strings.Contains(err.Error(), environmentVariable) {
				t.Fatalf("error = %q, want %s context", err, environmentVariable)
			}
		})
	}
}

func TestResolveRequiresEnvironmentVariable(t *testing.T) {
	unsetEnvironment(t, environmentVariable)

	_, err := Resolve(t.TempDir())
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), environmentVariable+" is not set") {
		t.Fatalf("error = %q, want missing-variable context", err)
	}
}

func TestResolveRequiresEnvironmentFile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(environmentVariable, "stage")

	_, err := Resolve(dir)
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), filepath.Join(dir, "stage.toml")) {
		t.Fatalf("error = %q, want stage.toml path", err)
	}
}

func TestResolveRequiresDirectory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config")
	writeFile(t, path)
	t.Setenv(environmentVariable, "dev")

	_, err := Resolve(path)
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "not a directory") {
		t.Fatalf("error = %q, want not-a-directory context", err)
	}
}

func TestResolveRejectsNonRegularConfigFiles(t *testing.T) {
	tests := []struct {
		name        string
		environment string
		path        func(string) string
	}{
		{
			name:        "common is directory",
			environment: "dev",
			path:        func(dir string) string { return filepath.Join(dir, "common.toml") },
		},
		{
			name:        "environment is directory",
			environment: "stage",
			path:        func(dir string) string { return filepath.Join(dir, "stage.toml") },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.Mkdir(tt.path(dir), 0o700); err != nil {
				t.Fatal(err)
			}
			if tt.environment == "dev" {
				writeFile(t, filepath.Join(dir, "dev.toml"))
			}
			t.Setenv(environmentVariable, tt.environment)

			_, err := Resolve(dir)
			if err == nil {
				t.Fatal("expected an error")
			}
			if !strings.Contains(err.Error(), "not a regular file") {
				t.Fatalf("error = %q, want non-regular-file context", err)
			}
		})
	}
}

func writeFile(t *testing.T, path string) {
	t.Helper()

	if err := os.WriteFile(path, []byte("[service]\n"), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func unsetEnvironment(t *testing.T, name string) {
	t.Helper()

	value, existed := os.LookupEnv(name)
	if err := os.Unsetenv(name); err != nil {
		t.Fatalf("unset %s: %v", name, err)
	}
	t.Cleanup(func() {
		if existed {
			_ = os.Setenv(name, value)
		} else {
			_ = os.Unsetenv(name)
		}
	})
}
