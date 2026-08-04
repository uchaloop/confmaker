package confmaker

import (
	"fmt"
	"os"
	"slices"

	"github.com/uchaloop/secret"
)

// Required returns the instance named name from a map of configured instances,
// or an error naming what is missing. kind labels the resource in the error, for
// example "postgres" or "kafka consumer". Use it to turn a required-but-absent
// named connection into a clear startup failure.
func Required[T any](instances map[string]T, kind, name string) (T, error) {
	value, ok := instances[name]
	if !ok {
		var zero T

		return zero, fmt.Errorf("%s %q is required but not configured", kind, name)
	}

	return value, nil
}

// Registry holds a set of named instances of the same type, for resources whose
// count is only known at runtime. Prefer confx (Fx-wired named instances) for
// the fixed, mandatory instances of a service; use a Registry only for genuinely
// dynamic ones.
type Registry[T any] struct {
	items map[string]T
}

// MakeRegistry returns a Registry over items. The map is used as-is.
func MakeRegistry[T any](items map[string]T) *Registry[T] {
	return &Registry[T]{items: items}
}

// Get returns the instance named name, or an error naming what is missing. kind
// labels the resource in the error.
func (r *Registry[T]) Get(kind, name string) (T, error) {
	value, ok := r.items[name]
	if !ok {
		var zero T

		return zero, fmt.Errorf("%s %q not found", kind, name)
	}

	return value, nil
}

// Names returns the configured instance names in sorted order.
func (r *Registry[T]) Names() []string {
	names := make([]string, 0, len(r.items))
	for name := range r.items {
		names = append(names, name)
	}

	slices.Sort(names)

	return names
}

// ResolveSecret reads a secret value from the named environment variable. It
// returns an error - naming only the variable, never the value - when the
// variable is unset or empty, so a missing secret fails the startup loudly
// without leaking anything into logs.
func ResolveSecret(envName string) (secret.Secret, error) {
	value, ok := os.LookupEnv(envName)
	if !ok {
		return "", fmt.Errorf("required secret from environment variable %q is not set", envName)
	}
	if len(value) == 0 {
		return "", fmt.Errorf("required secret from environment variable %q is empty", envName)
	}

	return secret.Secret(value), nil
}
