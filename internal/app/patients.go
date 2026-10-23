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
	// HistoryWindow bounds how far back/forward the record looks for appointments.
	HistoryWindow time.Duration
	FutureWindow  time.Duration
}

// NewPatientService wires the dependencies.
func NewPatientService(p domain.PatientRepository, e domain.EvolutionRepository, a domain.Agenda, ids domain.IDGenerator, c domain.Clock) *PatientService {
	return &PatientService{
		patients: p, evolutions: e, agenda: a, ids: ids, clock: c,
		HistoryWindow: 365 * 24 * time.Hour,
		FutureWindow:  180 * 24 * time.Hour,
	}
}

// Create validates and stores a new patient, returning it with its ID.
func (s *PatientService) Create(ctx context.Context, in PatientInput) (domain.Patient, error) {
	now := s.clock.Now()
	p := domain.Patient{
		ID:        s.ids.NewPatientID(),
		CreatedAt: now,
	}
	applyPatientInput(&p, in, now)
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
	applyPatientInput(&p, in, s.clock.Now())
	if err := p.Validate(); err != nil {
		return domain.Patient{}, err
	}
	if err := s.patients.Save(ctx, p); err != nil {
		return domain.Patient{}, fmt.Errorf("salvar paciente: %w", err)
	}
	return p, nil
}

func applyPatientInput(p *domain.Patient, in PatientInput, now time.Time) {
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
	items, err := s.patients.Search(ctx, query)
	if err != nil {
		return nil, err
	}
	sortPatients(items)
	return items, nil
}

// Record assembles the full ficha for a patient.
func (s *PatientService) Record(ctx context.Context, id string) (PatientRecord, error) {
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
	all, err := s.agenda.ListBetween(ctx, now.Add(-s.HistoryWindow), now.Add(s.FutureWindow))
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
