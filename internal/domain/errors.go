package domain

import (
	"errors"
	"fmt"
	"strings"
)

// ErrNotFound is returned when an entity does not exist.
var ErrNotFound = errors.New("não encontrado")

// ValidationError carries one or more field-level problems.
type ValidationError struct {
	Fields map[string]string
}

func (e *ValidationError) Error() string {
	parts := make([]string, 0, len(e.Fields))
	for k, v := range e.Fields {
		parts = append(parts, fmt.Sprintf("%s: %s", k, v))
	}
	return "validação: " + strings.Join(parts, "; ")
}

// Add records a problem for a field. A nil receiver is allocated lazily.
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
