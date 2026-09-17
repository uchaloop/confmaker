package confx_test

import (
	"fmt"
	"time"

	"github.com/uchaloop/confmaker"
	"github.com/uchaloop/confmaker/confx"
	"go.uber.org/fx"
)

// StoreConfig is the kind of config a library declares: plain fields, env tags,
// and no knowledge of where the values come from.
type StoreConfig struct {
	Host    string        `env:"HOST,notEmpty"`
	Timeout time.Duration `env:"TIMEOUT"`
}

func (StoreConfig) ConfigName() string { return "store" }

func (c *StoreConfig) SetDefaults() { c.Timeout = 30 * time.Second }

// Provide makes the config available to constructors. WithEnv stands in for the
// process environment here; an application calls confx.Module() without it.
func Example() {
	env := confmaker.WithEnv(map[string]string{"STORE_HOST": "db:5432"})

	app := fx.New(
		fx.NopLogger,
		confx.Module(env),
		confx.Provide[StoreConfig](),
		fx.Invoke(func(cfg StoreConfig) {
			fmt.Println(cfg.Host, cfg.Timeout)
		}),
	)
	if err := app.Err(); err != nil {
		fmt.Println(err)
	}

	// Output:
	// db:5432 30s
}

// ProvideNamed provides a second instance of the same type under an Fx tag, with
// its own prefix.
func ExampleProvideNamed() {
	env := confmaker.WithEnv(map[string]string{
		"STORE_HOST":         "db:5432",
		"REPLICA_STORE_HOST": "replica:5432",
	})

	type params struct {
		fx.In

		Primary StoreConfig
		Replica StoreConfig `name:"replica"`
	}

	app := fx.New(
		fx.NopLogger,
		confx.Module(env),
		confx.Provide[StoreConfig](),
		confx.ProvideNamed[StoreConfig]("replica", confmaker.WithPrefix("REPLICA_STORE_")),
		fx.Invoke(func(p params) {
			fmt.Println(p.Primary.Host, p.Replica.Host)
		}),
	)
	if err := app.Err(); err != nil {
		fmt.Println(err)
	}

	// Output:
	// db:5432 replica:5432
}
