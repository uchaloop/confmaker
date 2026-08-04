package filedecode

import (
	"testing"

	"github.com/uchaloop/secret/v2"
)

func TestZeroSecretsClearsReachableSecretsRegardlessOfTags(t *testing.T) {
	type node struct {
		Password secret.Secret
		Next     *node
	}
	type config struct {
		Direct secret.Secret   `json:"direct"`
		Ptr    *secret.Secret  `yaml:"ptr"`
		Slice  []secret.Secret `proto:"slice"`
		Map    map[string]secret.Secret
		Any    any
		Cycle  *node
	}

	ptrSecret := secret.New("ptr")
	cycle := &node{Password: secret.New("cycle")}
	cycle.Next = cycle
	cfg := config{
		Direct: secret.New("direct"),
		Ptr:    &ptrSecret,
		Slice:  []secret.Secret{secret.New("slice")},
		Map:    map[string]secret.Secret{"key": secret.New("map")},
		Any:    secret.New("interface"),
		Cycle:  cycle,
	}

	ZeroSecrets(&cfg)

	if cfg.Direct.Reveal() != "" ||
		cfg.Ptr.Reveal() != "" ||
		cfg.Slice[0].Reveal() != "" ||
		cfg.Map["key"].Reveal() != "" ||
		cfg.Any.(secret.Secret).Reveal() != "" ||
		cfg.Cycle.Password.Reveal() != "" {
		t.Fatal("at least one reachable secret was not cleared")
	}
}

func TestZeroSecretsLeavesUnsupportedValuesUntouched(t *testing.T) {
	type secretAlias = secret.Secret
	type internal struct {
		hiddenSecret secret.Secret
		hiddenAny    any
	}
	type config struct {
		Alias  secretAlias
		Inside internal
		Keys   map[secret.Secret]string
	}

	key := secret.New("map-key")
	cfg := config{
		Alias: secret.New("alias"),
		Inside: internal{
			hiddenSecret: secret.New("hidden"),
			hiddenAny:    secret.New("hidden-interface"),
		},
		Keys: map[secret.Secret]string{key: "value"},
	}

	ZeroSecrets(&cfg)

	if !cfg.Alias.IsZero() {
		t.Fatal("Secret alias was not cleared")
	}
	if cfg.Inside.hiddenSecret.IsZero() {
		t.Fatal("unexported Secret must be left untouched")
	}
	if cfg.Inside.hiddenAny.(secret.Secret).IsZero() {
		t.Fatal("unexported interface must be left untouched")
	}
	if _, ok := cfg.Keys[key]; !ok {
		t.Fatal("map key must be left untouched")
	}
}

func TestZeroSecretsHandlesOverlappingSlices(t *testing.T) {
	values := []secret.Secret{
		secret.New("one"),
		secret.New("two"),
		secret.New("three"),
	}
	cfg := struct {
		Short []secret.Secret
		Long  []secret.Secret
	}{
		Short: values[:1],
		Long:  values[:3],
	}

	ZeroSecrets(&cfg)

	for i, value := range values {
		if !value.IsZero() {
			t.Fatalf("values[%d] was not cleared", i)
		}
	}
}
