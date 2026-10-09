package confmaker

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

type sensitiveText string

func (*sensitiveText) IsSensitive() {}
func (s *sensitiveText) UnmarshalText(b []byte) error {
	return fmt.Errorf("do-not-disclose: %w", errSensitiveInput)
}

var errSensitiveInput = errors.New("sensitive cause")

func TestSensitiveTagAndMarker(t *testing.T) {
	type config struct {
		Password sensitiveText `env:"PASSWORD,secret"`
	}
	_, err := Load[config]("app", WithEnv(map[string]string{"APP_PASSWORD": "do-not-disclose"}))
	if err == nil || strings.Contains(err.Error(), "do-not-disclose") || errors.Is(err, errSensitiveInput) {
		t.Fatalf("unsafe error: %v", err)
	}
	problems := ConfigErrors(err)
	if len(problems) != 1 || problems[0].Kind != ErrorParse {
		t.Fatalf("expected sanitized parse error: %v", err)
	}
}
func TestPointerMarkerInMap(t *testing.T) {
	type config struct {
		Tokens map[string]sensitiveText `env:"TOKENS" envFormat:"json"`
	}
	_, err := Load[config]("app", WithEnv(map[string]string{"APP_TOKENS": `{"key":"do-not-disclose"}`}))
	if err == nil || strings.Contains(err.Error(), "do-not-disclose") || errors.Is(err, errSensitiveInput) {
		t.Fatalf("unsafe error: %v", err)
	}
	if p := ConfigErrors(err); len(p) != 1 || p[0].Kind != ErrorParse {
		t.Fatalf("expected parse error: %v", err)
	}
}
func TestSupportedPointerBoundary(t *testing.T) {
	type config struct {
		Value **int `env:"VALUE"`
	}
	_, err := Load[config]("app", WithEnv(nil))
	if err == nil {
		t.Fatal("pointer chains must be rejected")
	}
}
