package confx_test

import (
	"fmt"
	"os"
	"time"

	"github.com/uchaloop/confmaker/confx"
	"github.com/uchaloop/secret/v2"
	"go.uber.org/fx"
)

// StoreConfig is the kind of config a library declares: plain fields, env tags,
// and no knowledge of where the values come from.
type StoreConfig struct {
	Host     string        `env:"HOST,notEmpty"`
	Timeout  time.Duration `env:"TIMEOUT"`
	Password secret.Secret `env:"PASSWORD"`
}

// SetDefaults is where the library's own defaults live, so its tests and its
// callers see the same values a deployment starts from.
func (c *StoreConfig) SetDefaults() {
	c.Timeout = 30 * time.Second
}

// Validate checks what the values mean, once they have been supplied.
func (c StoreConfig) Validate() error {
	if c.Timeout <= 0 {
		return fmt.Errorf("timeout must be positive, got %s", c.Timeout)
	}

	return nil
}

// Store is what the library provides once it has its config.
type Store struct{ addr string }

// StoreModule consumes an untagged StoreConfig, so the application decides where
// it comes from.
var StoreModule = fx.Module("store",
	fx.Provide(func(cfg StoreConfig) *Store {
		return &Store{addr: cfg.Host}
	}),
)

// An application wires the library up by naming the instance. The name gives the
// environment prefix, so "store" reads STORE_HOST and the rest of STORE_*.
func Example() {
	os.Setenv("STORE_HOST", "store:9000")
	defer os.Unsetenv("STORE_HOST")

	var store *Store

	app := fx.New(
		fx.NopLogger,

		// Module first: it checks the environment against what the Provide calls
		// below declare, so a misspelled variable is reported before a
		// constructor complains about the value it is missing.
		confx.Module(),

		confx.Provide[StoreConfig]("store"),
		StoreModule,

		fx.Populate(&store),
	)
	if err := app.Err(); err != nil {
		fmt.Println(err)

		return
	}

	fmt.Println(store.addr)

	// Output:
	// store:9000
}

// A second instance of the same type reads its own prefix and arrives under an
// Fx name tag, so a consumer can ask for either.
func ExampleProvideNamed() {
	fx.New(
		fx.NopLogger,
		confx.Module(),

		confx.Provide[StoreConfig]("store"),        // STORE_HOST, STORE_TIMEOUT
		confx.ProvideNamed[StoreConfig]("replica"), // REPLICA_HOST, REPLICA_TIMEOUT

		fx.Invoke(fx.Annotate(
			func(primary, replica StoreConfig) {},
			fx.ParamTags(``, `name:"replica"`),
		)),
	)
}

// The manifest is the same traversal that fills a config, resolved without
// building an application - enough to generate a .env.example or a config map
// from the type itself.
func ExampleManifest() {
	variables, err := confx.Manifest[StoreConfig]("store")
	if err != nil {
		fmt.Println(err)

		return
	}

	for _, v := range variables {
		fmt.Printf("%s=%s\n", v.Name, v.Default)
	}

	// Output:
	// STORE_HOST=
	// STORE_TIMEOUT=30s
	// STORE_PASSWORD=
}
