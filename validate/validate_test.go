package validate

import (
	"errors"
	"strings"
	"testing"
)

func TestErrorsAggregate(t *testing.T) {
	var e Errors

	e.Add(nil) // ignored
	e.Add(errors.New("first"))
	e.Addf("second %d", 2)
	e.Require(false, "third %s", "x")
	e.Require(true, "not added")

	err := e.Err()
	if err == nil {
		t.Fatal("Err() = nil, want aggregated error")
	}

	msg := err.Error()
	for _, want := range []string{"first", "second 2", "third x"} {
		if !strings.Contains(msg, want) {
			t.Errorf("aggregated error %q does not contain %q", msg, want)
		}
	}
	if strings.Contains(msg, "not added") {
		t.Errorf("satisfied Require must not add an error: %q", msg)
	}
}

func TestEmptyIsNil(t *testing.T) {
	var e Errors
	if err := e.Err(); err != nil {
		t.Fatalf("empty Err() = %v, want nil", err)
	}
}

func TestErrorsIsCompatible(t *testing.T) {
	sentinel := errors.New("sentinel")

	var e Errors
	e.Add(sentinel)
	e.Addf("other")

	if !errors.Is(e.Err(), sentinel) {
		t.Fatal("errors.Is should find the recorded sentinel")
	}
}
