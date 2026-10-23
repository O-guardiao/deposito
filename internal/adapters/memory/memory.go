// Package memory provides in-memory implementations of the domain ports.
// They back the demo mode and the tests; nothing here talks to Google.
package memory

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/o-guardiao/deposito/internal/domain"
)

// PatientRepo is a thread-safe in-memory PatientRepository.
type PatientRepo struct {
	mu    sync.RWMutex
	items map[string]domain.Patient
}

// NewPatientRepo creates an empty repository.
func NewPatientRepo() *PatientRepo { return &PatientRepo{items: map[string]domain.Patient{}} }

// List implements domain.PatientRepository.
func (r *PatientRepo) List(context.Context) ([]domain.Patient, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]domain.Patient, 0, len(r.items))
	for _, p := range r.items {
		out = append(out, p)
	}
	return out, nil
}

// Get implements domain.PatientRepository.
func (r *PatientRepo) Get(_ context.Context, id string) (domain.Patient, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.items[id]
	if !ok {
		return domain.Patient{}, fmt.Errorf("paciente %q: %w", id, domain.ErrNotFound)
	}
	return p, nil
}

// Save implements domain.PatientRepository.
func (r *PatientRepo) Save(_ context.Context, p domain.Patient) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.items[p.ID] = p
	return nil
}

// EvolutionRepo is a thread-safe in-memory EvolutionRepository.
type EvolutionRepo struct {
	mu    sync.RWMutex
	items []domain.Evolution
}

// NewEvolutionRepo creates an empty repository.
func NewEvolutionRepo() *EvolutionRepo { return &EvolutionRepo{} }

// ListByPatient implements domain.EvolutionRepository.
func (r *EvolutionRepo) ListByPatient(_ context.Context, pid string) ([]domain.Evolution, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var out []domain.Evolution
	for _, e := range r.items {
		if e.PatientID == pid {
			out = append(out, e)
		}
	}
	return out, nil
}

// Add implements domain.EvolutionRepository.
func (r *EvolutionRepo) Add(_ context.Context, e domain.Evolution) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.items = append(r.items, e)
	return nil
}

// Agenda is a thread-safe in-memory domain.Agenda that assigns its own IDs.
type Agenda struct {
	mu    sync.RWMutex
	items map[string]domain.Appointment
	seq   int
}

// NewAgenda creates an empty agenda.
func NewAgenda() *Agenda { return &Agenda{items: map[string]domain.Appointment{}} }

// ListBetween implements domain.Agenda (start in [from, to)).
func (a *Agenda) ListBetween(_ context.Context, from, to time.Time) ([]domain.Appointment, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	var out []domain.Appointment
	for _, it := range a.items {
		if !it.Start.Before(from) && it.Start.Before(to) {
			out = append(out, it)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Start.Before(out[j].Start) })
	return out, nil
}

// Get implements domain.Agenda.
func (a *Agenda) Get(_ context.Context, id string) (domain.Appointment, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	it, ok := a.items[id]
	if !ok {
		return domain.Appointment{}, fmt.Errorf("agendamento %q: %w", id, domain.ErrNotFound)
	}
	return it, nil
}

// Create implements domain.Agenda.
func (a *Agenda) Create(_ context.Context, it domain.Appointment) (domain.Appointment, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.seq++
	it.ID = fmt.Sprintf("evt%04d", a.seq)
	a.items[it.ID] = it
	return it, nil
}

// Update implements domain.Agenda.
func (a *Agenda) Update(_ context.Context, it domain.Appointment) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if _, ok := a.items[it.ID]; !ok {
		return fmt.Errorf("agendamento %q: %w", it.ID, domain.ErrNotFound)
	}
	a.items[it.ID] = it
	return nil
}

// Delete implements domain.Agenda.
func (a *Agenda) Delete(_ context.Context, id string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if _, ok := a.items[id]; !ok {
		return fmt.Errorf("agendamento %q: %w", id, domain.ErrNotFound)
	}
	delete(a.items, id)
	return nil
}

// Mirror is a thread-safe in-memory domain.AppointmentMirror.
type Mirror struct {
	mu    sync.RWMutex
	items map[string]domain.Appointment
	// Fail, when set, makes every write return this error (used in tests).
	Fail error
}

// NewMirror creates an empty mirror.
func NewMirror() *Mirror { return &Mirror{items: map[string]domain.Appointment{}} }

// Upsert implements domain.AppointmentMirror.
func (m *Mirror) Upsert(_ context.Context, a domain.Appointment) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Fail != nil {
		return m.Fail
	}
	m.items[a.ID] = a
	return nil
}

// Reconcile implements domain.AppointmentMirror: live items replace every
// row except the cancelled ones, which are history and stay.
func (m *Mirror) Reconcile(_ context.Context, live []domain.Appointment) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Fail != nil {
		return m.Fail
	}
	next := make(map[string]domain.Appointment, len(live)+len(m.items))
	for id, a := range m.items {
		if a.Status == domain.StatusCancelled {
			next[id] = a
		}
	}
	for _, a := range live {
		next[a.ID] = a
	}
	m.items = next
	return nil
}

// Get returns a mirrored entry (test helper).
func (m *Mirror) Get(id string) (domain.Appointment, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	a, ok := m.items[id]
	return a, ok
}

// Len returns the number of mirrored entries (test helper).
func (m *Mirror) Len() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.items)
}

// SequentialIDs generates readable IDs like P0001 / E0001.
type SequentialIDs struct {
	mu   sync.Mutex
	p, e int
}

// NewPatientID implements domain.IDGenerator.
func (g *SequentialIDs) NewPatientID() string {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.p++
	return fmt.Sprintf("P%04d", g.p)
}

// NewEvolutionID implements domain.IDGenerator.
func (g *SequentialIDs) NewEvolutionID() string {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.e++
	return fmt.Sprintf("E%04d", g.e)
}
