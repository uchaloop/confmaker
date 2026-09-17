package confmaker

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/uchaloop/secret/v2"
)

// benchPool and benchConfig are the shape of a real infrastructure config: a
// required host, a secret, a slice, a map, and a nested struct under its own
// prefix. Thirteen variables represent one library instance, and a
// service builds a handful of instances at startup.
type benchPool struct {
	MaxConns        int32         `env:"MAX_CONNS"`
	MinConns        int32         `env:"MIN_CONNS"`
	MinIdleConns    int32         `env:"MIN_IDLE_CONNS"`
	MaxConnLifetime time.Duration `env:"MAX_CONN_LIFETIME"`
	MaxConnIdleTime time.Duration `env:"MAX_CONN_IDLE_TIME"`
	HealthPeriod    time.Duration `env:"HEALTH_PERIOD"`
}

type benchConfig struct {
	Host     string            `env:"HOST,notEmpty"`
	Database string            `env:"DATABASE,notEmpty"`
	User     string            `env:"USER"`
	Password secret.Secret     `env:"PASSWORD"`
	AppName  string            `env:"APP_NAME"`
	Brokers  []string          `env:"BROKERS"`
	Labels   map[string]string `env:"LABELS"`
	Pool     benchPool         `envPrefix:"POOL_"`
}

func (c *benchConfig) SetDefaults() {
	c.AppName = "bench"
	c.Pool.MaxConns = 4
	c.Pool.MaxConnLifetime = time.Hour
}

// BenchmarkManifest is what Manifest costs for one instance: compiling the
// schema, calling SetDefaults and rendering every default as ENV text. Loading
// does not render defaults, so this is not the cost of loading.
func BenchmarkManifest(b *testing.B) {
	b.ReportAllocs()

	for b.Loop() {
		if _, err := Manifest[benchConfig](WithName("confxbench")); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkLoadOne loads one instance: "compiled" applies the environment with a
// prepared schema, "load" is the whole Load call. Scenarios separate defaults,
// a populated environment and the missing-required path.
func BenchmarkLoadOne(b *testing.B) {
	for _, scenario := range []string{"defaults", "populated", "missing-required"} {
		b.Run(scenario, func(b *testing.B) {
			env := map[string]string{}
			if scenario != "missing-required" {
				env["CONFXBENCH_HOST"] = "localhost"
				env["CONFXBENCH_DATABASE"] = "test"
			}

			if scenario == "populated" {
				addBenchEnv(env, "CONFXBENCH_")
			}

			fields, err := compileSchema(reflect.TypeFor[benchConfig](), "CONFXBENCH_")
			if err != nil {
				b.Fatal(err)
			}

			for _, mode := range []string{"compiled", "load"} {
				b.Run(mode, func(b *testing.B) {
					b.ReportAllocs()
					for b.Loop() {
						var cfg benchConfig
						var err error
						if mode == "compiled" {
							setDefaults(&cfg)
							err = applyAndValidate(&cfg, fields, "confxbench", env)
						} else {
							cfg, err = Load[benchConfig](WithName("confxbench"), WithEnv(env))
						}

						if scenario == "missing-required" {
							if err == nil {
								b.Fatal("missing required values were accepted")
							}
						} else if err != nil {
							b.Fatal(err)
						}
					}
				})
			}
		})
	}
}

// addBenchEnv adds every variable a benchConfig reads under prefix to env.
func addBenchEnv(env map[string]string, prefix string) {
	for name, value := range map[string]string{
		"HOST": "localhost", "DATABASE": "test",
		"USER": "app", "PASSWORD": "benchmark-secret", "APP_NAME": "worker",
		"BROKERS": "a:9092,b:9092,c:9092", "LABELS": "env:test,team:core",
		"POOL_MAX_CONNS": "20", "POOL_MIN_CONNS": "2", "POOL_MIN_IDLE_CONNS": "1",
		"POOL_MAX_CONN_LIFETIME": "1h", "POOL_MAX_CONN_IDLE_TIME": "5m", "POOL_HEALTH_PERIOD": "30s",
	} {
		env[prefix+name] = value
	}
}

// BenchmarkLoader measures six populated instances in one Loader, the handful a
// service builds at startup. "register" is MakeLoader and the Add calls alone:
// resolving names and compiling schemas. "register+load" adds the environment
// snapshot, the unknown-variable check and every load; confx's
// BenchmarkModuleStart runs the same configs and environment inside an Fx
// application. BenchmarkLoadOne/*/compiled is one load with a prepared schema.
func BenchmarkLoader(b *testing.B) {
	const instances = 6

	env := map[string]string{}
	names := make([]string, instances)
	for i := range names {
		names[i] = fmt.Sprintf("confxbench%d", i)
		addBenchEnv(env, defaultPrefix(names[i]))
	}

	register := func() *Loader {
		loader := MakeLoader(WithEnv(env))
		for _, name := range names {
			loader.Add[benchConfig](WithName(name))
		}

		return loader
	}

	b.Run("register", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			register()
		}
	})

	b.Run("register+load", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			if err := register().Load(); err != nil {
				b.Fatal(err)
			}
		}
	})
}

// BenchmarkHint is the suggestion one unknown variable costs, against the
// variables a service of six instances declares. It runs on the failing path
// only, which is why the scan is bounded rather than fast.
func BenchmarkHint(b *testing.B) {
	known := make(map[string]string, 120)

	for instance := range 6 {
		for variable := range 20 {
			known[fmt.Sprintf("CONFXINST%d_VARIABLE_NUMBER_%d", instance, variable)] = "inst"
		}
	}

	b.ReportAllocs()

	for b.Loop() {
		if got := hint("CONFXINST3_VARIABLE_NUMBRE_7", known); len(got) == 0 {
			b.Fatal("no suggestion for a one-edit typo")
		}
	}
}

// BenchmarkMapField parses a map variable of twenty entries.
func BenchmarkMapField(b *testing.B) {
	parts := make([]string, 20)
	for i := range parts {
		parts[i] = fmt.Sprintf("label_%d:value_%d", i, i)
	}

	raw := strings.Join(parts, ",")

	parse, err := fieldParser(reflect.TypeFor[map[string]string](), defaultSeparator, defaultKeyValSeparator)
	if err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	for b.Loop() {
		var target map[string]string
		if err := parse(reflect.ValueOf(&target).Elem(), raw); err != nil {
			b.Fatal(err)
		}
	}
}
