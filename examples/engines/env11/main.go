package main

import (
	"fmt"
	"log"

	"github.com/uchaloop/confmaker/v2"
)

type config struct {
	Port  int    `env:"PORT" envDefault:"8080"`
	Label string `env:"LABEL"`
}

func (c *config) SetDefaults() {
	c.Label = "local"
}

func (c config) Validate() error {
	if c.Port <= 0 {
		return fmt.Errorf("port must be positive")
	}

	return nil
}

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	engine := makeEngine(map[string]map[string]string{
		"primary": {},
		"replica": {"PORT": "9090"},
	})
	loader := confmaker.MakeLoader(confmaker.WithEngine(engine), confmaker.WithDiagnostics())

	primary := loader.Register[config]("primary")
	replica := loader.Register[config]("replica")

	if err := loader.Load(); err != nil {
		return err
	}

	for _, handle := range []*confmaker.Handle[config]{primary, replica} {
		cfg, err := handle.Value()
		if err != nil {
			return err
		}

		fmt.Println(handle.Name(), cfg.Port, cfg.Label)
	}

	fmt.Println(loader.Report().DetailLevel)

	return nil
}
