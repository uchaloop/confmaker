package confx

import (
	"strings"
	"testing"

	"github.com/uchaloop/secret/v2"
	"go.uber.org/fx"
)

// TestBindRejectsACollision covers the mistake the rule exists for: a second
// nested struct of the same type without an envPrefix of its own. Both fields
// would read one variable, and the manifest would list its name twice.
func TestBindRejectsACollision(t *testing.T) {
	type pool struct {
		MaxConns int `env:"MAX_CONNS"`
	}
	type config struct {
		Primary pool `envPrefix:"PRIMARY_"`
		Replica pool
		Spare   pool
	}

	err := bindError[config](t)
	if !strings.Contains(err.Error(), "APP_MAX_CONNS") {
		t.Fatalf("the error does not name the variable: %v", err)
	}
	if !strings.Contains(err.Error(), "Replica.MaxConns") || !strings.Contains(err.Error(), "Spare.MaxConns") {
		t.Fatalf("the error does not name both declarations: %v", err)
	}
}

func TestBindAllowsTheSameNameUnderDifferentPrefixes(t *testing.T) {
	type pool struct {
		MaxConns int `env:"MAX_CONNS"`
	}
	type config struct {
		Primary pool `envPrefix:"PRIMARY_"`
		Replica pool `envPrefix:"REPLICA_"`
	}

	got := names(described[config](t, "APP_"))
	if len(got) != 2 || got[0] != "APP_PRIMARY_MAX_CONNS" || got[1] != "APP_REPLICA_MAX_CONNS" {
		t.Fatalf("manifest = %v", got)
	}
}

// TestSecretPublishesNoDefault keeps a mask out of anything a deployment might
// paste back: a rendered secret reads as a value but is not one.
func TestSecretPublishesNoDefault(t *testing.T) {
	type config struct {
		Password secret.Secret `env:"PASSWORD"`
	}

	variable := described[config](t, "APP_")[0]
	if len(variable.Default) != 0 || variable.HasDefault {
		t.Fatalf("a secret published a default: %+v", variable)
	}
	if !variable.Secret {
		t.Fatal("the field was not reported as a secret")
	}
}

func TestManifestReportsAnInvalidDeclaration(t *testing.T) {
	type pool struct {
		MaxConns int `env:"MAX_CONNS"`
	}
	type config struct {
		Shards []pool
	}

	variables, err := Manifest[config]("app")
	if err == nil {
		t.Fatal("an invalid declaration produced a manifest instead of an error")
	}
	if len(variables) != 0 {
		t.Fatalf("variables returned alongside the error: %v", names(variables))
	}
}

func TestInstanceNameIsChecked(t *testing.T) {
	rejected := []string{"", "my service", "Postgres", "_postgres", "postgres-", "pg/main"}

	for _, name := range rejected {
		t.Run(name, func(t *testing.T) {
			if _, err := Manifest[strictConfig](name); err == nil {
				t.Errorf("Manifest accepted %q", name)
			}

			app := fx.New(
				fx.NopLogger,
				Provide[strictConfig](name),
				fx.Invoke(func(strictConfig) {}),
			)
			if app.Err() == nil {
				t.Errorf("Provide accepted %q", name)
			}
		})
	}

	for _, name := range []string{"postgres", "read-replica", "daemon_ozon_card_publish", "db.main", "pg2"} {
		if err := checkName(name); err != nil {
			t.Errorf("%q rejected: %v", name, err)
		}
	}
}
