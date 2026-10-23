package domain

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

// ErrNotFound is returned when an entity does not exist.
var ErrNotFound = errors.New("não encontrado")

// ValidationError carries one or more field-level problems.
type ValidationError struct {
	Fields map[string]string
}

// NewValidationError returns an empty, ready-to-use error.
func NewValidationError() *ValidationError { return &ValidationError{Fields: map[string]string{}} }

func (e *ValidationError) Error() string {
	keys := make([]string, 0, len(e.Fields))
	for k := range e.Fields {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s: %s", k, e.Fields[k]))
	}
	return "validação: " + strings.Join(parts, "; ")
}

// Add records a problem for a field. The receiver must be non-nil; the
// Fields map is allocated lazily so a zero ValidationError is usable.
func (e *ValidationError) Add(field, msg string) {
	if e.Fields == nil {
		e.Fields = map[string]string{}
	}
	e.Fields[field] = msg
}

// Empty reports whether no problems were recorded.
func (e *ValidationError) Empty() bool { return e == nil || len(e.Fields) == 0 }

// AsValidation extracts a ValidationError from err, if present.
func AsValidation(err error) (*ValidationError, bool) {
	var ve *ValidationError
	if errors.As(err, &ve) {
		return ve, true
	}
	return nil, false
}

// IsNotFound reports whether err wraps ErrNotFound.
func IsNotFound(err error) bool { return errors.Is(err, ErrNotFound) }
