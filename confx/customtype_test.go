package confx

import (
	"fmt"
	"strconv"
	"strings"
	"testing"
)

// money stands in for the library types a config reaches for - a decimal, a
// nullable, a timestamp: a struct that decodes itself through a pointer-receiver
// UnmarshalText and renders itself through a value-receiver MarshalText.
type money struct{ cents int64 }

func (m *money) UnmarshalText(text []byte) error {
	parsed, err := strconv.ParseInt(string(text), 10, 64)
	if err != nil {
		return fmt.Errorf("%q is not an amount in cents", text)
	}

	m.cents = parsed

	return nil
}

func (m money) MarshalText() ([]byte, error) {
	return []byte(strconv.FormatInt(m.cents, 10)), nil
}

type moneyConfig struct {
	Price money `env:"PRICE"`
}

func (c *moneyConfig) SetDefaults() {
	c.Price = money{cents: 499}
}

func (c moneyConfig) Validate() error {
	if c.Price.cents <= 0 {
		return fmt.Errorf("price must be positive, got %d", c.Price.cents)
	}

	return nil
}

func TestCustomTypeParsesThroughItsOwnTextForm(t *testing.T) {
	t.Setenv("APP_PRICE", "1250")

	var cfg moneyConfig
	if err := fillEnv(&cfg, "APP_", "app"); err != nil {
		t.Fatalf("fill: %v", err)
	}

	if cfg.Price.cents != 1250 {
		t.Fatalf("price = %d, want 1250", cfg.Price.cents)
	}
}

func TestCustomTypeKeepsItsDefaultAndRendersIt(t *testing.T) {
	// Nothing is set, so SetDefaults stands.
	var cfg moneyConfig
	if err := fillEnv(&cfg, "APP_", "app"); err != nil {
		t.Fatalf("fill: %v", err)
	}
	if cfg.Price.cents != 499 {
		t.Fatalf("price = %d, want the default", cfg.Price.cents)
	}

	variable := Manifest[moneyConfig]("app")[0]
	if variable.Default != "499" || !variable.HasDefault {
		t.Fatalf("the default did not render through the type: %+v", variable)
	}
}

func TestCustomTypeReportsItsOwnParseError(t *testing.T) {
	t.Setenv("APP_PRICE", "free")

	var cfg moneyConfig
	err := fillEnv(&cfg, "APP_", "app")
	if err == nil {
		t.Fatal("a value the type rejects was accepted")
	}
	if !strings.Contains(err.Error(), "APP_PRICE") || !strings.Contains(err.Error(), "cents") {
		t.Fatalf("the error names neither the variable nor the type's reason: %v", err)
	}
}

func TestCustomTypeIsValidatedByTheConfig(t *testing.T) {
	t.Setenv("APP_PRICE", "0")

	var cfg moneyConfig
	err := fillEnv(&cfg, "APP_", "app")
	if err == nil || !strings.Contains(err.Error(), "price must be positive") {
		t.Fatalf("Validate did not see the parsed value: %v", err)
	}
}

func TestCustomTypeInCollections(t *testing.T) {
	type config struct {
		Prices []money          `env:"PRICES"`
		Tiers  map[string]money `env:"TIERS"`
	}

	t.Setenv("APP_PRICES", "1,2,3")
	t.Setenv("APP_TIERS", "basic:100,pro:900")

	var cfg config
	if err := fillEnv(&cfg, "APP_", "app"); err != nil {
		t.Fatalf("fill: %v", err)
	}

	if len(cfg.Prices) != 3 || cfg.Prices[2].cents != 3 {
		t.Fatalf("slice elements were not decoded: %v", cfg.Prices)
	}
	if cfg.Tiers["pro"].cents != 900 {
		t.Fatalf("map values were not decoded: %v", cfg.Tiers)
	}
}

// TestUntaggedFieldHoldingAStructIsNotConfiguration guards the distinction the
// nesting rule rests on: a field that merely reaches a struct is skipped like
// any other untagged field, and only a nested config is refused.
func TestUntaggedFieldHoldingAStructIsNotConfiguration(t *testing.T) {
	type config struct {
		Host   string `env:"HOST"`
		Prices []money
		Price  *money
		Tiers  map[string]money
	}

	got := names(described[config](t, "APP_"))
	if len(got) != 1 || got[0] != "APP_HOST" {
		t.Fatalf("manifest = %v, want only APP_HOST", got)
	}
}

func TestNestedConfigIsStillRefused(t *testing.T) {
	type shard struct {
		Host string `env:"HOST"`
	}

	t.Run("slice", func(t *testing.T) {
		type config struct {
			Shards []shard
		}

		if err := bindError[config](t); !strings.Contains(err.Error(), "nest by value") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("map", func(t *testing.T) {
		type config struct {
			Shards map[string]shard
		}

		if err := bindError[config](t); !strings.Contains(err.Error(), "nest by value") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("nested deeper", func(t *testing.T) {
		type outer struct {
			Inner shard
		}
		type config struct {
			Items []outer
		}

		if err := bindError[config](t); !strings.Contains(err.Error(), "nest by value") {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}
