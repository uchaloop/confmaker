package main

import (
	_ "embed"
	"fmt"
	"log"

	"github.com/uchaloop/confmaker/v2"
)

//go:embed primary.yaml
var primaryDocument []byte

//go:embed replica.yaml
var replicaDocument []byte

type config struct {
	Port  int    `yaml:"listen_port"`
	Label string `yaml:"label"`
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
	engine := makeEngine(map[string][]byte{"primary": primaryDocument, "replica": replicaDocument})
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
