package confmaker

import (
	"strings"
	"testing"

	secret "github.com/uchaloop/confmaker/v2/internal/testsecret"
)

func TestPointerToSecretIsMasked(t *testing.T) {
	variables := manifested[struct {
		Password *secret.Secret `env:"PASSWORD"`
	}](t, "confxapp")
	if !variables[0].Secret {
		t.Fatal("a pointer to a secret was not recognised as one")
	}
}

func TestSecretBehindPointersInCollectionIsRefused(t *testing.T) {
	t.Parallel()

	cases := map[string]error{
		"map value": bindError[struct {
			Passwords map[string]***secret.Secret `env:"PASSWORDS"`
		}](t),
		"map key": bindError[struct {
			Passwords map[**secret.Secret]string `env:"PASSWORDS"`
		}](t),
		"slice": bindError[struct {
			Passwords []***secret.Secret `env:"PASSWORDS"`
		}](t),
	}

	for name, err := range cases {
		if !strings.Contains(err.Error(), "unsupported pointer shape") {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestParseErrorBehindPointersNeverPrintsTheValue(t *testing.T) {
	t.Parallel()

	_, err := Load[struct {
		Password *secret.Secret `env:"PASSWORD,notEmpty"`
	}]("confxapp", WithEnv(map[string]string{"CONFXAPP_PASSWORD": ""}))
	if err == nil || strings.Contains(err.Error(), "FAKE") {
		t.Fatalf("got %v", err)
	}
}
