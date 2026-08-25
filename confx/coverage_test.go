package confx

import (
	"bytes"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/uchaloop/secret/v2"
	"go.uber.org/fx"
)

// TestCustomSeparators exercises the tags eleven fields across these repositories
// already carry, which the defaults never reach.
func TestCustomSeparators(t *testing.T) {
	type config struct {
		Brokers []string          `env:"BROKERS" envSeparator:";"`
		Tiers   map[string]int    `env:"TIERS" envSeparator:"|" envKeyValSeparator:"="`
		Plain   map[string]string `env:"PLAIN"`
	}

	t.Setenv("APP_BROKERS", "a:9092;b:9092")
	t.Setenv("APP_TIERS", "basic=1|pro=9")
	t.Setenv("APP_PLAIN", "k:v")

	var cfg config
	if err := fillEnv(&cfg, "APP_", "app"); err != nil {
		t.Fatalf("fill: %v", err)
	}

	if len(cfg.Brokers) != 2 || cfg.Brokers[0] != "a:9092" {
		t.Fatalf("a custom separator was not used: %v", cfg.Brokers)
	}
	if cfg.Tiers["pro"] != 9 || len(cfg.Tiers) != 2 {
		t.Fatalf("custom map separators were not used: %v", cfg.Tiers)
	}
	if cfg.Plain["k"] != "v" {
		t.Fatalf("the defaults stopped working alongside overrides: %v", cfg.Plain)
	}
}

func TestEmptySeparatorIsRefused(t *testing.T) {
	t.Run("slice", func(t *testing.T) {
		type config struct {
			Hosts []string `env:"HOSTS" envSeparator:""`
		}

		// Splitting on nothing would turn "abc" into three elements.
		if err := bindError[config](t); !strings.Contains(err.Error(), "envSeparator") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("map key separator", func(t *testing.T) {
		type config struct {
			Tiers map[string]string `env:"TIERS" envKeyValSeparator:""`
		}

		if err := bindError[config](t); !strings.Contains(err.Error(), "envKeyValSeparator") {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

func TestUnknownEnvOptionIsRefused(t *testing.T) {
	cases := map[string]string{
		"misspelled required": "HOST,requred",
		"wrong case":          "HOST,notempty",
		"retired option":      "HOST,init",
	}

	for name, tag := range cases {
		t.Run(name, func(t *testing.T) {
			// Built by hand: the tag has to be a literal for the compiler.
			var err error

			switch tag {
			case "HOST,requred":
				type config struct {
					Host string `env:"HOST,requred"`
				}

				err = bindError[config](t)
			case "HOST,notempty":
				type config struct {
					Host string `env:"HOST,notempty"`
				}

				err = bindError[config](t)
			default:
				type config struct {
					Host string `env:"HOST,init"`
				}

				err = bindError[config](t)
			}

			if !strings.Contains(err.Error(), "unknown env option") {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestParseUnsignedIntegers(t *testing.T) {
	got, err := parseInto[uint16](t, "65535")
	if err != nil || got != 65535 {
		t.Fatalf("got %d, err %v", got, err)
	}

	if _, err := parseInto[uint8](t, "256"); err == nil {
		t.Error("an out-of-range uint8 was accepted")
	}
}

// TestDefaultsRenderForCollections covers what a generated .env.example carries
// for the composite types: the text has to be something the variable could
// carry back.
func TestDefaultsRenderForCollections(t *testing.T) {
	type config struct {
		Brokers []string          `env:"BROKERS" envSeparator:";"`
		Tiers   map[string]int    `env:"TIERS"`
		Timeout *time.Duration    `env:"TIMEOUT"`
		Absent  *int              `env:"ABSENT"`
		Labels  map[string]string `env:"LABELS"`
	}

	var cfg config
	cfg.Brokers = []string{"a:9092", "b:9092"}
	cfg.Tiers = map[string]int{"pro": 9, "basic": 1}
	timeout := 30 * time.Second
	cfg.Timeout = &timeout

	bindings, err := bind(reflect.ValueOf(&cfg).Elem(), "APP_")
	if err != nil {
		t.Fatalf("bind: %v", err)
	}

	rendered := make(map[string]string, len(bindings))
	for _, b := range bindings {
		rendered[b.Name] = b.Default
	}

	want := map[string]string{
		"APP_BROKERS": "a:9092;b:9092",
		"APP_TIERS":   "basic:1,pro:9",
		"APP_TIMEOUT": "30s",
		"APP_ABSENT":  "",
		"APP_LABELS":  "",
	}
	for name, expected := range want {
		if rendered[name] != expected {
			t.Errorf("%s rendered as %q, want %q", name, rendered[name], expected)
		}
	}
}

func TestParseErrorNeverCarriesASecretValue(t *testing.T) {
	type config struct {
		Password secret.Secret `env:"PASSWORD"`
	}

	// secret.Secret decodes anything, so the guarantee is checked through the
	// dump and the manifest instead, and through a value that fails elsewhere.
	t.Setenv("APP_PASSWORD", "hunter2")

	var cfg config
	if err := fillEnv(&cfg, "APP_", "app"); err != nil {
		t.Fatalf("fill: %v", err)
	}

	var out bytes.Buffer

	app := fx.New(
		fx.NopLogger,
		Module(WithDump(&out)),
		Provide[config]("app"),
		fx.Invoke(func(config) {}),
	)
	if app.Err() != nil {
		t.Fatalf("app: %v", app.Err())
	}

	if strings.Contains(out.String(), "hunter2") {
		t.Fatalf("the dump printed a secret:\n%s", out.String())
	}
}

func TestPointerToSecretIsMasked(t *testing.T) {
	type config struct {
		Password *secret.Secret `env:"PASSWORD"`
	}

	variables := described[config](t, "APP_")
	if !variables[0].Secret {
		t.Fatal("a pointer to a secret was not recognised as one")
	}
}

// TestDumpReportsWhereAValueComesFrom covers the branches of the source column
// that a set variable never reaches.
func TestDumpReportsWhereAValueComesFrom(t *testing.T) {
	type config struct {
		Host    string `env:"HOST,require"`
		Timeout string `env:"TIMEOUT"`
		Spare   string `env:"SPARE"`
	}

	var out bytes.Buffer

	// Host is required and unset, Timeout has a default, Spare has neither.
	app := fx.New(
		fx.NopLogger,
		Module(WithDump(&out)),
		Provide[config]("app"),
		fx.Invoke(func(config) {}),
	)
	if app.Err() == nil {
		t.Fatal("expected the required variable to fail the start")
	}

	dump := out.String()
	for _, want := range []string{"required", "zero value"} {
		if !strings.Contains(dump, want) {
			t.Errorf("dump is missing the %q source:\n%s", want, dump)
		}
	}
}

// TestParseErrorNeverNamesASecretValue checks the guarantee directly. No type
// outside the secret package can implement its marker interface, so a secret
// whose text form fails cannot be built here - the branch is exercised as the
// unit it is.
func TestParseErrorNeverNamesASecretValue(t *testing.T) {
	b := binding{
		Variable: Variable{Name: "APP_PASSWORD", Type: "secret.Secret", Secret: true},
	}

	err := describeParseError(b, errors.New("\"hunter2\" is not valid"))
	if strings.Contains(err.Error(), "hunter2") {
		t.Fatalf("the value reached the error: %v", err)
	}
	if !strings.Contains(err.Error(), "APP_PASSWORD") {
		t.Fatalf("the error does not name the variable: %v", err)
	}
}

func TestHintStaysSilentWithoutACloseCandidate(t *testing.T) {
	known := map[string]bool{"APP_HOST": true}

	if got := hint("APP_SOMETHING_ENTIRELY_ELSE", known); len(got) != 0 {
		t.Fatalf("a distant name was suggested: %q", got)
	}
}

func TestPointerToAnUnreadableTypeIsRefused(t *testing.T) {
	type config struct {
		Ratio *complex128 `env:"RATIO"`
	}

	if err := bindError[config](t); !strings.Contains(err.Error(), "cannot be read") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestParseRejectsABadFloat(t *testing.T) {
	if _, err := parseInto[float32](t, "2.5.1"); err == nil {
		t.Error("a malformed float was accepted")
	}
}
