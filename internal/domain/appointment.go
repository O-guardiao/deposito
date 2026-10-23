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

// ParseStatus converts a stored string into a status, defaulting to scheduled.
func ParseStatus(s string) AppointmentStatus {
	switch AppointmentStatus(s) {
	case StatusDone:
		return StatusDone
	case StatusCancelled:
		return StatusCancelled
	default:
		return StatusScheduled
	}
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
