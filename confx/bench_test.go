package confx

import (
	"fmt"
	"testing"
	"time"

	"github.com/uchaloop/secret/v2"
)

// benchPool and benchConfig are the shape of a real infrastructure config: a
// required host, a secret, a slice, a map, and a nested struct under its own
// prefix. Twenty variables is what one instance of one library declares, and a
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

// BenchmarkManifest is the traversal a Provide call runs to describe one
// instance, and the one Manifest runs to generate from a type.
func BenchmarkManifest(b *testing.B) {
	b.ReportAllocs()

	for b.Loop() {
		if _, err := Manifest[benchConfig]("confxbench"); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkFillEnv is the traversal a Provide call runs to build one instance:
// the defaults, the environment over them, and the validation.
func BenchmarkFillEnv(b *testing.B) {
	b.ReportAllocs()

	for b.Loop() {
		var cfg benchConfig

		_ = fillEnv(&cfg, "CONFXBENCH_", "confxbench")
	}
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
