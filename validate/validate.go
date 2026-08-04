// Package validate provides a small error accumulator so a Validate method can
// report every problem in one pass instead of failing on the first. It has no
// external dependencies.
package validate

import (
	"errors"
	"fmt"
)

// Errors collects validation errors. The zero value is ready to use.
type Errors struct {
	errs []error
}

// Add records err when it is non-nil.
func (e *Errors) Add(err error) {
	if err != nil {
		e.errs = append(e.errs, err)
	}
}

// Addf records a new formatted error.
func (e *Errors) Addf(format string, args ...any) {
	e.errs = append(e.errs, fmt.Errorf(format, args...))
}

// Require records a formatted error when cond is false. It is the convenient
// path for "field X is required" style checks.
func (e *Errors) Require(cond bool, format string, args ...any) {
	if !cond {
		e.Addf(format, args...)
	}
}

// Err returns all accumulated errors joined into one, or nil when there are
// none. The result works with errors.Is/As over each recorded error.
func (e *Errors) Err() error {
	return errors.Join(e.errs...)
}
