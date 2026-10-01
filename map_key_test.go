package confmaker

import (
	"math"
	"strconv"
	"testing"
)

type floatStructKey struct{ Number float64 }

func (k *floatStructKey) UnmarshalText(text []byte) error {
	number, err := strconv.ParseFloat(string(text), 64)
	if err != nil {
		return err
	}
	k.Number = number
	return nil
}

func TestMapRejectsNonReflexiveKeys(t *testing.T) {
	type floatConfig struct {
		Values map[float64]int `env:"VALUES"`
	}
	type customConfig struct {
		Values map[floatStructKey]int `env:"VALUES"`
	}
	for _, raw := range []string{"NaN:1", "NaN:1,NaN:2"} {
		t.Run(raw, func(t *testing.T) {
			_, floatErr := Load[floatConfig]("app", WithEnv(map[string]string{"APP_VALUES": raw}))
			_, customErr := Load[customConfig]("app", WithEnv(map[string]string{"APP_VALUES": raw}))
			for _, err := range []error{floatErr, customErr} {
				problems := ConfigErrors(err)
				if len(problems) != 1 || problems[0].Kind != ErrorParse || problems[0].InstanceName != "app" || problems[0].VariableName != "APP_VALUES" || problems[0].FieldPath != "Values" {
					t.Fatalf("expected contextual parse error, got %v", err)
				}
			}
		})
	}
}

func TestMapAllowsNumericKeysAndNaNValues(t *testing.T) {
	type config struct {
		Values map[float64]float64 `env:"VALUES"`
	}
	cfg, err := Load[config]("app", WithEnv(map[string]string{"APP_VALUES": "1.5:NaN,-2:3,+Inf:4"}))
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Values) != 3 || !math.IsNaN(cfg.Values[1.5]) || cfg.Values[-2] != 3 || cfg.Values[math.Inf(1)] != 4 {
		t.Fatalf("unexpected values: %v", cfg.Values)
	}
	_, err = Load[config]("app", WithEnv(map[string]string{"APP_VALUES": "1:2,1.0:3"}))
	if err == nil {
		t.Fatal("equivalent numeric keys must still be rejected")
	}
}
