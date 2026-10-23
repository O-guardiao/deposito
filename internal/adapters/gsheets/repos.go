package gsheets

import (
	"context"
	"fmt"
	"time"

	"github.com/o-guardiao/deposito/internal/domain"
)

// PatientRepo implements domain.PatientRepository on the Pacientes tab.
type PatientRepo struct {
	t   table
	loc *time.Location
}

// NewPatientRepo creates the repository for a spreadsheet.
func NewPatientRepo(api SheetsAPI, spreadsheetID string, loc *time.Location) *PatientRepo {
	return &PatientRepo{t: table{api, spreadsheetID, TabPatients}, loc: loc}
}

// List implements domain.PatientRepository.
func (r *PatientRepo) List(ctx context.Context) ([]domain.Patient, error) {
	rows, err := r.t.rows(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]domain.Patient, 0, len(rows))
	for _, row := range rows {
		out = append(out, rowToPatient(row, r.loc))
	}
	return out, nil
}

// Get implements domain.PatientRepository.
func (r *PatientRepo) Get(ctx context.Context, id string) (domain.Patient, error) {
	all, err := r.List(ctx)
	if err != nil {
		return domain.Patient{}, err
	}
	for _, p := range all {
		if p.ID == id {
			return p, nil
		}
	}
	return domain.Patient{}, fmt.Errorf("paciente %q: %w", id, domain.ErrNotFound)
}

// Search implements domain.PatientRepository.
func (r *PatientRepo) Search(ctx context.Context, q string) ([]domain.Patient, error) {
	all, err := r.List(ctx)
	if err != nil {
		return nil, err
	}
	out := all[:0]
	for _, p := range all {
		if p.MatchesQuery(q) {
			out = append(out, p)
		}
	}
	return out, nil
}

// Save implements domain.PatientRepository.
func (r *PatientRepo) Save(ctx context.Context, p domain.Patient) error {
	return r.t.upsert(ctx, patientToRow(p))
}

// EvolutionRepo implements domain.EvolutionRepository on the Evolucoes tab.
type EvolutionRepo struct {
	t   table
	loc *time.Location
}

// NewEvolutionRepo creates the repository for a spreadsheet.
func NewEvolutionRepo(api SheetsAPI, spreadsheetID string, loc *time.Location) *EvolutionRepo {
	return &EvolutionRepo{t: table{api, spreadsheetID, TabEvolutions}, loc: loc}
}

// ListByPatient implements domain.EvolutionRepository.
func (r *EvolutionRepo) ListByPatient(ctx context.Context, pid string) ([]domain.Evolution, error) {
	rows, err := r.t.rows(ctx)
	if err != nil {
		return nil, err
	}
	var out []domain.Evolution
	for _, row := range rows {
		e := rowToEvolution(row, r.loc)
		if e.PatientID == pid {
			out = append(out, e)
		}
	}
	return out, nil
}

// Add implements domain.EvolutionRepository.
func (r *EvolutionRepo) Add(ctx context.Context, e domain.Evolution) error {
	return r.t.append(ctx, evolutionToRow(e))
}

// Mirror implements domain.AppointmentMirror on the Agendamentos tab.
type Mirror struct {
	t table
}

// NewMirror creates the mirror for a spreadsheet.
func NewMirror(api SheetsAPI, spreadsheetID string) *Mirror {
	return &Mirror{t: table{api, spreadsheetID, TabAppointments}}
}

// Upsert implements domain.AppointmentMirror.
func (m *Mirror) Upsert(ctx context.Context, a domain.Appointment) error {
	return m.t.upsert(ctx, appointmentToRow(a))
}

// Delete implements domain.AppointmentMirror.
func (m *Mirror) Delete(ctx context.Context, id string) error {
	return m.t.deleteByKey(ctx, id)
}

// ReplaceAll implements domain.AppointmentMirror.
func (m *Mirror) ReplaceAll(ctx context.Context, items []domain.Appointment) error {
	rows := make([][]string, 0, len(items))
	for _, a := range items {
		rows = append(rows, appointmentToRow(a))
	}
	return m.t.replaceAll(ctx, rows)
}
