package confmaker

import (
	"fmt"
	"testing"
)

// BenchmarkLoadDiagnostics measures registration and loading with each report mode.
func BenchmarkLoadDiagnostics(b *testing.B) {
	for _, count := range []int{1, 8} {
		for _, mode := range []string{"disabled", "receiver", "handler", "both"} {
			b.Run(fmt.Sprintf("%d/%s", count, mode), func(b *testing.B) {
				names := make([]string, count)
				env := make(map[string]string, count*2)
				for i := range names {
					names[i] = fmt.Sprintf("db%d", i)
					prefix := defaultPrefix(names[i])
					env[prefix+"HOST"] = "localhost"
					env[prefix+"DATABASE"] = "app"
				}

				envOption := WithEnv(env)
				handlerOption := WithDiagnosticHandler(func(LoadReport) {})
				b.ReportAllocs()

				for b.Loop() {
					options := []LoaderOption{envOption}
					if mode == "receiver" || mode == "both" {
						options = append(options, WithDiagnostics())
					}

					if mode == "handler" || mode == "both" {
						options = append(options, handlerOption)
					}

					loader := MakeLoader(options...)
					for _, name := range names {
						loader.Register[benchConfig](name)
					}

					if err := loader.Load(); err != nil {
						b.Fatal(err)
					}
				}
			})
		}
	}
}
