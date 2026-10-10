package confcli_test

import (
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/uchaloop/confmaker/v2"
	"github.com/uchaloop/confmaker/v2/confcli"
)

func ExampleMakeDescribeFlag() {
	flags := flag.NewFlagSet("app", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	describe := confcli.MakeDescribeFlag(flags)

	if err := flags.Parse([]string{"-describe=env"}); err != nil {
		fmt.Println(err)

		return
	}

	if !describe.Requested() {
		return
	}

	document, err := confmaker.Manifest[struct {
		Host string `env:"HOST,required"`
	}]("app")
	if err != nil {
		fmt.Println(err)

		return
	}

	if err := describe.Write(os.Stdout, document); err != nil {
		fmt.Println(err)
	}

	// Output:
	// # Configuration: "app"
	// # Required.
	// # APP_HOST=
}
