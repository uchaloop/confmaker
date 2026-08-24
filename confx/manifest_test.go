package confx

import (
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

func names(variables []Variable) []string {
	out := make([]string, 0, len(variables))
	for _, variable := range variables {
		out = append(out, variable.Name)
	}

	return out
}
