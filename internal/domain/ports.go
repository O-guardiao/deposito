package domain

import (
	"context"
	"time"
)

// PatientRepository persists patient records.
type PatientRepository interface {
	List(ctx context.Context) ([]Patient, error)
	Get(ctx context.Context, id string) (Patient, error)
	Search(ctx context.Context, query string) ([]Patient, error)
	// Save inserts or updates by ID.
	Save(ctx context.Context, p Patient) error
}

// EvolutionRepository persists session notes.
type EvolutionRepository interface {
	ListByPatient(ctx context.Context, patientID string) ([]Evolution, error)
	Add(ctx context.Context, e Evolution) error
}

// Agenda is the live source of truth for appointments (Google Calendar in production).
type Agenda interface {
	ListBetween(ctx context.Context, from, to time.Time) ([]Appointment, error)
	Get(ctx context.Context, id string) (Appointment, error)
	// Create stores the appointment and returns it with the ID assigned by the agenda.
	Create(ctx context.Context, a Appointment) (Appointment, error)
	Update(ctx context.Context, a Appointment) error
	Delete(ctx context.Context, id string) error
}

// AppointmentMirror keeps a searchable copy of appointments (the spreadsheet tab).
type AppointmentMirror interface {
	Upsert(ctx context.Context, a Appointment) error
	Delete(ctx context.Context, id string) error
	ReplaceAll(ctx context.Context, items []Appointment) error
}

// Clock abstracts time for testability.
type Clock interface {
	Now() time.Time
}

// IDGenerator produces stable unique identifiers for patients and evolutions.
type IDGenerator interface {
	NewPatientID() string
	NewEvolutionID() string
}

// ClockFunc adapts a function to the Clock interface.
type ClockFunc func() time.Time

// Now implements Clock.
func (f ClockFunc) Now() time.Time { return f() }
