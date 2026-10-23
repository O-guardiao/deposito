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

// SessionTitle is the calendar summary used for this patient's sessions.
// It is a domain rule shared by the use cases and the demo seed.
// sessionTitlePrefix marks events created by the app; ShortTitle strips it for display.
const sessionTitlePrefix = "Fisio: "

func (p Patient) SessionTitle() string { return sessionTitlePrefix + p.Name }

// Age returns the patient's age in full years at the given instant; ok is
// false when the birth date is unknown.
func (p Patient) Age(now time.Time) (years int, ok bool) {
	if p.BirthDate.IsZero() {
		return 0, false
	}
	years = now.Year() - p.BirthDate.Year()
	if now.YearDay() < p.BirthDate.YearDay() {
		years--
	}
	return years, true
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
