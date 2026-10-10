package confexport_test

import (
	"fmt"
	"os"

	"github.com/uchaloop/confmaker/v2"
	"github.com/uchaloop/confmaker/v2/confexport"
)

func ExampleWriteEnvExample() {
	document, err := confmaker.Manifest[struct {
		Host string `env:"HOST"`
	}]("app")
	if err != nil {
		fmt.Println(err)

		return
	}

	if err := confexport.WriteEnvExample(os.Stdout, document); err != nil {
		fmt.Println(err)
	}

	// Output:
	// # Configuration: "app"
	// # APP_HOST=
}
