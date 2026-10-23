package domain

import (
	"strings"
	"time"
)

// Patient is the physiotherapist's client record (ficha).
type Patient struct {
	ID        string
	Name      string
	Phone     string
	Address   string
	BirthDate time.Time // zero when unknown
	Diagnosis string
	Notes     string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Validate checks the required fields.
func (p Patient) Validate() error {
	ve := &ValidationError{}
	if strings.TrimSpace(p.Name) == "" {
		ve.Add("name", "Informe o nome do paciente")
	}
	if ve.Empty() {
		return nil
	}
	return ve
}

// Age returns the patient's age in full years at the given instant, or -1 if unknown.
func (p Patient) Age(now time.Time) int {
	if p.BirthDate.IsZero() {
		return -1
	}
	years := now.Year() - p.BirthDate.Year()
	if now.YearDay() < p.BirthDate.YearDay() {
		years--
	}
	return years
}

// MatchesQuery reports whether the free-text query matches the patient's name or phone.
func (p Patient) MatchesQuery(q string) bool {
	q = strings.ToLower(strings.TrimSpace(q))
	if q == "" {
		return true
	}
	return strings.Contains(strings.ToLower(p.Name), q) ||
		strings.Contains(strings.ToLower(p.Phone), q)
}
