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

// TestUnreadableTypeIsRefusedWhenBound is what choosing the parser from the type
// buys: a field that could never be read fails on the first start, whether or
// not anyone happens to set its variable.
func TestUnreadableTypeIsRefusedWhenBound(t *testing.T) {
	type shard struct {
		Host string `env:"HOST"`
	}

	cases := map[string]func(*testing.T) error{
		"a config in a slice": func(t *testing.T) error {
			type config struct {
				Shards []shard `env:"SHARDS"`
			}

			return bindError[config](t)
		},
		"a complex number": func(t *testing.T) error {
			type config struct {
				Ratio complex128 `env:"RATIO"`
			}

			return bindError[config](t)
		},
		"a byte slice": func(t *testing.T) error {
			type config struct {
				Data []byte `env:"DATA"`
			}

			return bindError[config](t)
		},
		"a map keyed by a struct": func(t *testing.T) error {
			type config struct {
				Weights map[shard]int `env:"WEIGHTS"`
			}

			return bindError[config](t)
		},
	}

	for name, run := range cases {
		t.Run(name, func(t *testing.T) {
			// No variable is set: the declaration alone has to fail.
			if err := run(t); !strings.Contains(err.Error(), "field") {
				t.Fatalf("the error does not name the field: %v", err)
			}
		})
	}
}

// TestConfigErrorLabelsEveryLine keeps a multi-problem report readable: a joined
// error renders one problem per line, and a line that scrolls past on its own
// still has to say which config it belongs to.
func TestConfigErrorLabelsEveryLine(t *testing.T) {
	type config struct {
		Host string `env:"HOST,required"`
		User string `env:"USER,required"`
	}

	var cfg config
	err := fillEnv(&cfg, "APP_", "postgres")
	if err == nil {
		t.Fatal("expected both variables to be reported")
	}

	lines := strings.Split(err.Error(), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected one line per problem, got:\n%v", err)
	}

	for _, line := range lines {
		if !strings.HasPrefix(line, `config "postgres":`) {
			t.Errorf("line is not attributed to its config: %q", line)
		}
	}
}

// TestTwoInstancesMayNotShareAPrefix guards the scan: it accepts a variable any
// instance declares, so two instances on one prefix would hide each other's
// typos. Separators normalise, which is how two distinct names get there.
func TestTwoInstancesMayNotShareAPrefix(t *testing.T) {
	t.Setenv("READ_REPLICA_HOST", "db:5432")
	t.Setenv("READ_REPLICA_PASSWORD", "s3cr3t")

	err := fx.New(
		fx.NopLogger,
		Module(),
		Provide[strictConfig]("read-replica"),
		ProvideNamed[strictConfig]("read_replica"),
		fx.Invoke(func(strictConfig) {}),
	).Err()
	if err == nil {
		t.Fatal("two instances read one prefix and the check said nothing")
	}
	if !strings.Contains(err.Error(), "READ_REPLICA_") {
		t.Fatalf("the error does not name the shared prefix: %v", err)
	}
}
