package domain

import (
	"time"
)

// AppointmentStatus is the lifecycle state of a session.
type AppointmentStatus string

const (
	StatusScheduled AppointmentStatus = "agendado"
	StatusDone      AppointmentStatus = "concluido"
	StatusCancelled AppointmentStatus = "cancelado"
)

// DefaultSessionDuration is the length of a session when none is given.
const DefaultSessionDuration = time.Hour

// ParseStatus converts a stored string into a status. ok is false when the
// string is not a known status; callers decide how to treat that.
func ParseStatus(s string) (st AppointmentStatus, ok bool) {
	switch AppointmentStatus(s) {
	case StatusScheduled, StatusDone, StatusCancelled:
		return AppointmentStatus(s), true
	}
	return StatusScheduled, false
}

// Appointment is a calendar entry. PatientID is empty when the entry was
// created outside the app and has not been linked to a patient yet.
type Appointment struct {
	ID        string
	PatientID string
	Title     string
	Start     time.Time
	End       time.Time
	AllDay    bool
	Status    AppointmentStatus
	Notes     string
	Location  string
	UpdatedAt time.Time
}

// Linked reports whether the appointment belongs to a known patient.
func (a Appointment) Linked() bool { return a.PatientID != "" }

// Validate checks times and required fields.
func (a Appointment) Validate() error {
	ve := &ValidationError{}
	if a.Start.IsZero() {
		ve.Add("start", "Informe data e hora")
	}
	if !a.AllDay && !a.End.IsZero() && !a.End.After(a.Start) {
		ve.Add("end", "O término deve ser depois do início")
	}
	if ve.Empty() {
		return nil
	}
	return ve
}

// Duration returns the session length.
func (a Appointment) Duration() time.Duration { return a.End.Sub(a.Start) }

// SameDay reports whether a and b fall on the same calendar day in loc.
func SameDay(a, b time.Time, loc *time.Location) bool {
	a, b = a.In(loc), b.In(loc)
	ay, am, ad := a.Date()
	by, bm, bd := b.Date()
	return ay == by && am == bm && ad == bd
}

// StartOfDay truncates t to midnight in loc.
func StartOfDay(t time.Time, loc *time.Location) time.Time {
	t = t.In(loc)
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, loc)
}
