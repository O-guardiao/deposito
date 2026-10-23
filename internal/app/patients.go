// Package app holds the use cases, independent of transport and storage.
package app

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/o-guardiao/deposito/internal/domain"
)

// PatientInput is the data accepted when creating or updating a patient.
type PatientInput struct {
	Name      string
	Phone     string
	Address   string
	BirthDate time.Time
	Diagnosis string
	Notes     string
}

// PatientRecord is the full "ficha": patient, notes and appointments.
type PatientRecord struct {
	Patient    domain.Patient
	Evolutions []domain.Evolution   // newest first
	Upcoming   []domain.Appointment // soonest first
	Past       []domain.Appointment // most recent first
}

// PatientService manages patient records.
type PatientService struct {
	patients   domain.PatientRepository
	evolutions domain.EvolutionRepository
	agenda     domain.Agenda
	ids        domain.IDGenerator
	clock      domain.Clock
	// Window bounds how far back/forward the record looks for appointments.
	Window Window
}

// NewPatientService wires the dependencies.
func NewPatientService(p domain.PatientRepository, e domain.EvolutionRepository, a domain.Agenda, ids domain.IDGenerator, c domain.Clock) *PatientService {
	return &PatientService{
		patients: p, evolutions: e, agenda: a, ids: ids, clock: c,
		Window: Window{Behind: 365 * 24 * time.Hour, Ahead: 180 * 24 * time.Hour},
	}
}

// Create validates and stores a new patient, returning it with its ID.
func (s *PatientService) Create(ctx context.Context, in PatientInput) (domain.Patient, error) {
	now := s.clock.Now()
	p := domain.Patient{
		ID:        s.ids.NewPatientID(),
		CreatedAt: now,
	}
	in.Apply(&p, now)
	if err := p.Validate(); err != nil {
		return domain.Patient{}, err
	}
	if err := s.patients.Save(ctx, p); err != nil {
		return domain.Patient{}, fmt.Errorf("salvar paciente: %w", err)
	}
	return p, nil
}

// Update applies the input to an existing patient.
func (s *PatientService) Update(ctx context.Context, id string, in PatientInput) (domain.Patient, error) {
	p, err := s.patients.Get(ctx, id)
	if err != nil {
		return domain.Patient{}, err
	}
	in.Apply(&p, s.clock.Now())
	if err := p.Validate(); err != nil {
		return domain.Patient{}, err
	}
	if err := s.patients.Save(ctx, p); err != nil {
		return domain.Patient{}, fmt.Errorf("salvar paciente: %w", err)
	}
	return p, nil
}

// Apply copies the (trimmed) input onto an existing patient, keeping its ID
// and creation time. The delivery layer reuses it to re-render invalid forms.
func (in PatientInput) Apply(p *domain.Patient, now time.Time) {
	p.Name = strings.TrimSpace(in.Name)
	p.Phone = strings.TrimSpace(in.Phone)
	p.Address = strings.TrimSpace(in.Address)
	p.BirthDate = in.BirthDate
	p.Diagnosis = strings.TrimSpace(in.Diagnosis)
	p.Notes = strings.TrimSpace(in.Notes)
	p.UpdatedAt = now
}

// Get returns one patient.
func (s *PatientService) Get(ctx context.Context, id string) (domain.Patient, error) {
	return s.patients.Get(ctx, id)
}

// List returns all patients sorted by name.
func (s *PatientService) List(ctx context.Context) ([]domain.Patient, error) {
	items, err := s.patients.List(ctx)
	if err != nil {
		return nil, err
	}
	sortPatients(items)
	return items, nil
}

// Search filters patients by name or phone; an empty query lists everyone.
func (s *PatientService) Search(ctx context.Context, query string) ([]domain.Patient, error) {
	all, err := s.patients.List(ctx)
	if err != nil {
		return nil, err
	}
	items := all[:0]
	for _, p := range all {
		if p.MatchesQuery(query) {
			items = append(items, p)
		}
	}
	sortPatients(items)
	return items, nil
}

// GetRecord assembles the full ficha for a patient.
func (s *PatientService) GetRecord(ctx context.Context, id string) (PatientRecord, error) {
	p, err := s.patients.Get(ctx, id)
	if err != nil {
		return PatientRecord{}, err
	}
	evs, err := s.evolutions.ListByPatient(ctx, id)
	if err != nil {
		return PatientRecord{}, fmt.Errorf("listar evoluções: %w", err)
	}
	sort.Slice(evs, func(i, j int) bool { return evs[i].Date.After(evs[j].Date) })

	now := s.clock.Now()
	all, err := s.agenda.ListBetween(ctx, now.Add(-s.Window.Behind), now.Add(s.Window.Ahead))
	if err != nil {
		return PatientRecord{}, fmt.Errorf("listar agenda: %w", err)
	}
	rec := PatientRecord{Patient: p, Evolutions: evs}
	for _, a := range all {
		if a.PatientID != id {
			continue
		}
		if a.Start.Before(now) {
			rec.Past = append(rec.Past, a)
		} else {
			rec.Upcoming = append(rec.Upcoming, a)
		}
	}
	sort.Slice(rec.Upcoming, func(i, j int) bool { return rec.Upcoming[i].Start.Before(rec.Upcoming[j].Start) })
	sort.Slice(rec.Past, func(i, j int) bool { return rec.Past[i].Start.After(rec.Past[j].Start) })
	return rec, nil
}

func sortPatients(items []domain.Patient) {
	sort.Slice(items, func(i, j int) bool {
		return strings.ToLower(items[i].Name) < strings.ToLower(items[j].Name)
	})
}
