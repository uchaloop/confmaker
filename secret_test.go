package confmaker

import (
	"errors"
	"strings"
	"testing"

	"github.com/uchaloop/secret/v2"
)

func TestPointerToSecretIsMasked(t *testing.T) {
	variables := manifested[struct {
		Password *secret.Secret `env:"PASSWORD"`
	}](t, "confxapp")
	if !variables[0].Secret {
		t.Fatal("a pointer to a secret was not recognised as one")
	}
}

// TestParseErrorNeverNamesASecretValue checks the guarantee directly. No type
// outside the secret package can implement its marker interface, so a secret
// whose text form fails cannot be built here - the branch is exercised as the
// unit it is.
func TestParseErrorNeverNamesASecretValue(t *testing.T) {
	b := fieldSpec{Name: "CONFXAPP_PASSWORD", Type: "secret.Secret", Secret: true}

	err := describeParseError(b, errors.New("\"hunter2\" is not valid"))
	if strings.Contains(err.Error(), "hunter2") {
		t.Fatalf("the value reached the error: %v", err)
	}

	if !strings.Contains(err.Error(), "CONFXAPP_PASSWORD") {
		t.Fatalf("the error does not name the variable: %v", err)
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
		if !strings.Contains(err.Error(), "holds a secret in a") {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestParseErrorBehindPointersNeverPrintsTheValue(t *testing.T) {
	t.Parallel()

	_, err := Load[struct {
		Password **secret.Secret `env:"PASSWORD,notEmpty"`
	}]("confxapp", WithEnv(map[string]string{"CONFXAPP_PASSWORD": ""}))
	if err == nil || strings.Contains(err.Error(), "FAKE") {
		t.Fatalf("got %v", err)
	}
}
