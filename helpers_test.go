package confmaker

import (
	"reflect"
	"strings"
	"testing"
)

func TestRequired(t *testing.T) {
	instances := map[string]int{"alpha": 1}

	got, err := Required(instances, "widget", "alpha")
	if err != nil {
		t.Fatalf("Required(alpha): %v", err)
	}
	if got != 1 {
		t.Fatalf("Required(alpha) = %d, want 1", got)
	}

	_, err = Required(instances, "widget", "beta")
	if err == nil {
		t.Fatal("Required(beta): expected an error")
	}
	for _, want := range []string{"widget", "beta"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err, want)
		}
	}
}

func TestRegistry(t *testing.T) {
	r := MakeRegistry(map[string]int{"b": 2, "a": 1})

	got, err := r.Get("worker", "a")
	if err != nil {
		t.Fatalf("Get(a): %v", err)
	}
	if got != 1 {
		t.Fatalf("Get(a) = %d, want 1", got)
	}

	if _, err := r.Get("worker", "z"); err == nil {
		t.Fatal("Get(z): expected an error")
	} else if !strings.Contains(err.Error(), "z") {
		t.Errorf("error %q does not mention the missing name", err)
	}

	if names := r.Names(); !reflect.DeepEqual(names, []string{"a", "b"}) {
		t.Fatalf("Names() = %v, want [a b] (sorted)", names)
	}
}

func TestResolveSecret(t *testing.T) {
	t.Setenv("CONFMAKER_TEST_SECRET", "topsecret")

	s, err := ResolveSecret("CONFMAKER_TEST_SECRET")
	if err != nil {
		t.Fatalf("ResolveSecret: %v", err)
	}
	if s.Reveal() != "topsecret" {
		t.Fatalf("Reveal() = %q, want topsecret", s.Reveal())
	}

	// Missing variable: an error that names the variable, not a value.
	if _, err := ResolveSecret("CONFMAKER_TEST_MISSING_XYZ"); err == nil {
		t.Fatal("missing variable: expected an error")
	} else if !strings.Contains(err.Error(), "CONFMAKER_TEST_MISSING_XYZ") {
		t.Errorf("error %q does not name the variable", err)
	}

	// Empty variable is treated as missing.
	t.Setenv("CONFMAKER_TEST_EMPTY", "")
	if _, err := ResolveSecret("CONFMAKER_TEST_EMPTY"); err == nil {
		t.Fatal("empty variable: expected an error")
	}
}
