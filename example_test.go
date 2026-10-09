package confmaker_test

import (
	"fmt"
	"time"

	"github.com/uchaloop/confmaker/v2"
)

// StoreConfig is the kind of config a library declares: plain fields, env tags,
// and no knowledge of where the values come from.
type StoreConfig struct {
	Host    string        `env:"HOST,notEmpty"`
	Timeout time.Duration `env:"TIMEOUT"`
}

func (c *StoreConfig) SetDefaults() { c.Timeout = 30 * time.Second }

func (c StoreConfig) Validate() error {
	if c.Timeout <= 0 {
		return fmt.Errorf("timeout must be positive, got %s", c.Timeout)
	}

	return nil
}

// Load reads one config. WithEnv stands in for the process environment here, so
// the example runs the same everywhere; an application calls Load without it.
func ExampleLoad() {
	cfg, err := confmaker.Load[StoreConfig]("store", confmaker.WithEnv(map[string]string{
		"STORE_HOST": "db:5432",
	}))
	if err != nil {
		fmt.Println(err)

		return
	}

	fmt.Println(cfg.Host, cfg.Timeout)

	// Output:
	// db:5432 30s
}

// One error reports every problem, with a suggestion for a misspelled variable.
// Validate would refuse the zero timeout, but it does not run while a variable of
// the config is missing: its report comes once the host is fixed.
func ExampleLoad_errors() {
	_, err := confmaker.Load[StoreConfig]("store", confmaker.WithEnv(map[string]string{
		"STORE_HSOT":    "db:5432",
		"STORE_TIMEOUT": "0s",
	}))

	fmt.Println(err)

	// Output:
	// unknown configuration variable "STORE_HSOT" (did you mean "STORE_HOST"?)
	// config "store": required variable "STORE_HOST" is not set
}

// A Loader loads several configs, here two instances of one type, and reports
// their problems together.
func ExampleLoader() {
	loader := confmaker.MakeLoader(confmaker.WithEnv(map[string]string{
		"STORE_HOST":   "db:5432",
		"REPLICA_HOST": "replica:5432",
	}))

	primary := loader.Register[StoreConfig]("store")   // STORE_*
	replica := loader.Register[StoreConfig]("replica") // REPLICA_*

	if err := loader.Load(); err != nil {
		fmt.Println(err)

		return
	}

	primaryCfg, _ := primary.Value()
	replicaCfg, _ := replica.Value()
	fmt.Println(primaryCfg.Host, replicaCfg.Host)

	// Output:
	// db:5432 replica:5432
}

// Manifest lists the variables and their defaults without reading the
// environment. The output is ENV names and values, not an escaped shell script.
func ExampleManifest() {
	variables, err := confmaker.Manifest[StoreConfig]("store", confmaker.IncludeDefaults())
	if err != nil {
		fmt.Println(err)

		return
	}

	for _, v := range variables.Configs[0].Variables {
		fmt.Printf("%s=%s\n", v.Name, v.Default.Text)
	}

	// Output:
	// STORE_HOST=
	// STORE_TIMEOUT=30s
}

// The instance name stays primary while WithPrefix selects DATABASE_* variables.
func ExampleWithPrefix() {
	cfg, err := confmaker.Load[StoreConfig]("primary",
		confmaker.WithPrefix("DATABASE_"),
		confmaker.WithEnv(map[string]string{"DATABASE_HOST": "db:5432"}),
	)
	if err != nil {
		fmt.Println(err)

		return
	}

	variables, err := confmaker.Manifest[StoreConfig]("primary", confmaker.WithPrefix("DATABASE_"))
	if err != nil {
		fmt.Println(err)

		return
	}

	fmt.Println(cfg.Host, variables.Configs[0].Variables[0].Name)

	// Output:
	// db:5432 DATABASE_HOST
}

// Manifest describes the registered set without reading required ENV values.
func ExampleLoader_Manifest() {
	loader := confmaker.MakeLoader()
	loader.Register[StoreConfig]("store")
	loader.Register[StoreConfig]("replica", confmaker.WithPrefix("REPLICA_STORE_"))

	configs, err := loader.Manifest(confmaker.IncludeDefaults())
	if err != nil {
		fmt.Println(err)

		return
	}

	for _, config := range configs.Configs {
		fmt.Println(config.InstanceName, config.Prefix)
		for _, variable := range config.Variables {
			fmt.Printf("%s=%s\n", variable.Name, variable.Default.Text)
		}
	}

	// Output:
	// replica REPLICA_STORE_
	// REPLICA_STORE_HOST=
	// REPLICA_STORE_TIMEOUT=30s
	// store STORE_
	// STORE_HOST=
	// STORE_TIMEOUT=30s
}
