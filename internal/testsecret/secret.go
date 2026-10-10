// Package testsecret supplies a minimal sensitive text value for core tests.
// It is not a secret-storage implementation and must not be used by applications.
package testsecret

type Secret struct{ value string }

func (Secret) IsSensitive()                      {}
func (s Secret) Reveal() string                  { return s.value }
func (s Secret) IsZero() bool                    { return s.value == "" }
func (s *Secret) UnmarshalText(raw []byte) error { s.value = string(raw); return nil }
func (s Secret) MarshalText() ([]byte, error)    { return []byte("****"), nil }
