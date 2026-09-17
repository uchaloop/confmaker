package confmaker

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/uchaloop/secret/v2"
)

func TestParseErrorNeverCarriesASecretValue(t *testing.T) {
	t.Parallel()

	env := map[string]string{
		"CONFXAPP_PASSWORD": "hunter2",
	}

	type config struct {
		Password secret.Secret `env:"PASSWORD"`
	}

	// secret.Secret decodes anything, so the guarantee is checked through the
	// dump and the manifest instead, and through a value that fails elsewhere.

	_, err := Load[config](WithName("confxapp"), WithEnv(env))
	if err != nil {
		t.Fatalf("fill: %v", err)
	}

	var out bytes.Buffer

	if _, err := Load[config](WithDump(&out), WithEnv(env), WithName("confxapp")); err != nil {
		t.Fatalf("load: %v", err)
	}

	if strings.Contains(out.String(), "hunter2") {
		t.Fatalf("the dump printed a secret:\n%s", out.String())
	}
}

func TestPointerToSecretIsMasked(t *testing.T) {
	type config struct {
		Password *secret.Secret `env:"PASSWORD"`
	}

	variables := manifested[config](t, "confxapp")
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

func TestSecretBehindAnyPointerDepthIsMasked(t *testing.T) {
	t.Parallel()

	type single struct {
		Password ***secret.Secret `env:"PASSWORD"`
	}

	var out bytes.Buffer
	cfg, err := Load[single](WithName("confxapp"), WithDump(&out), WithEnv(map[string]string{"CONFXAPP_PASSWORD": "FAKE_SECRET"}))
	if err != nil {
		t.Fatal(err)
	}

	if (***cfg.Password).Reveal() != "FAKE_SECRET" {
		t.Fatal("the secret was not loaded")
	}

	if strings.Contains(out.String(), "FAKE_SECRET") || !strings.Contains(out.String(), "(set)") {
		t.Fatalf("the dump printed a secret behind pointers:\n%s", out.String())
	}

	variables := manifested[single](t, "confxapp")
	if !variables[0].Secret || len(variables[0].Default) != 0 {
		t.Fatalf("the manifest does not treat it as a secret: %+v", variables[0])
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

	type config struct {
		Password **secret.Secret `env:"PASSWORD,notEmpty"`
	}

	_, err := Load[config](WithName("confxapp"), WithEnv(map[string]string{"CONFXAPP_PASSWORD": ""}))
	if err == nil || strings.Contains(err.Error(), "FAKE") {
		t.Fatalf("got %v", err)
	}
}
