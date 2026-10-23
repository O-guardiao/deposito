package app

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/o-guardiao/deposito/internal/domain"
)

// ErrMirror signals that the agenda write succeeded but the spreadsheet mirror
// did not. Callers should treat it as a warning, not a failure.
var ErrMirror = errors.New("agenda gravada, mas a planilha não foi atualizada")

// ScheduleInput describes a new session.
type ScheduleInput struct {
	PatientID string
	Start     time.Time
	Duration  time.Duration
	Notes     string
}

// RescheduleInput describes changes to an existing session.
type RescheduleInput struct {
	Start    time.Time
	Duration time.Duration
	Notes    string
}

// DayView is the agenda for one date.
type DayView struct {
	Date  time.Time
	Items []domain.Appointment
}

// AgendaService coordinates the live agenda and its spreadsheet mirror.
type AgendaService struct {
	agenda   domain.Agenda
	mirror   domain.AppointmentMirror
	patients domain.PatientRepository
	clock    domain.Clock
	loc      *time.Location
	// DefaultDuration is used when the input omits one.
	DefaultDuration time.Duration
}

// NewAgendaService wires the dependencies. loc defines day boundaries.
func NewAgendaService(a domain.Agenda, m domain.AppointmentMirror, p domain.PatientRepository, c domain.Clock, loc *time.Location) *AgendaService {
	if loc == nil {
		loc = time.UTC
	}
	return &AgendaService{agenda: a, mirror: m, patients: p, clock: c, loc: loc, DefaultDuration: time.Hour}
}

// Location returns the time zone used for day boundaries.
func (s *AgendaService) Location() *time.Location { return s.loc }

// Day lists the appointments of one calendar day, sorted by start.
func (s *AgendaService) Day(ctx context.Context, date time.Time) (DayView, error) {
	start := startOfDay(date, s.loc)
	items, err := s.agenda.ListBetween(ctx, start, start.AddDate(0, 0, 1))
	if err != nil {
		return DayView{}, fmt.Errorf("listar agenda: %w", err)
	}
	sortByStart(items)
	return DayView{Date: start, Items: items}, nil
}

// Week lists seven consecutive days starting at the given date, each with its items.
func (s *AgendaService) Week(ctx context.Context, start time.Time) ([]DayView, error) {
	first := startOfDay(start, s.loc)
	items, err := s.agenda.ListBetween(ctx, first, first.AddDate(0, 0, 7))
	if err != nil {
		return nil, fmt.Errorf("listar agenda: %w", err)
	}
	days := make([]DayView, 7)
	for i := range days {
		days[i].Date = first.AddDate(0, 0, i)
	}
	for _, a := range items {
		idx := int(startOfDay(a.Start, s.loc).Sub(first).Hours() / 24)
		if idx < 0 || idx >= 7 {
			continue
		}
		days[idx].Items = append(days[idx].Items, a)
	}
	for i := range days {
		sortByStart(days[i].Items)
	}
	return days, nil
}

// Get returns one appointment.
func (s *AgendaService) Get(ctx context.Context, id string) (domain.Appointment, error) {
	return s.agenda.Get(ctx, id)
}

// Schedule creates a session for a patient. The title, location and reminders
// are derived from the patient record. A returned ErrMirror still carries a
// valid appointment.
func (s *AgendaService) Schedule(ctx context.Context, in ScheduleInput) (domain.Appointment, error) {
	ve := &domain.ValidationError{}
	if in.PatientID == "" {
		ve.Add("patientId", "Escolha o paciente")
	}
	if in.Start.IsZero() {
		ve.Add("start", "Informe data e hora")
	}
	if !ve.Empty() {
		return domain.Appointment{}, ve
	}
	p, err := s.patients.Get(ctx, in.PatientID)
	if err != nil {
		if domain.IsNotFound(err) {
			ve.Add("patientId", "Paciente não encontrado")
			return domain.Appointment{}, ve
		}
		return domain.Appointment{}, err
	}
	dur := in.Duration
	if dur <= 0 {
		dur = s.DefaultDuration
	}
	a := domain.Appointment{
		PatientID: p.ID,
		Title:     TitleFor(p),
		Start:     in.Start,
		End:       in.Start.Add(dur),
		Status:    domain.StatusScheduled,
		Notes:     strings.TrimSpace(in.Notes),
		Location:  p.Address,
		UpdatedAt: s.clock.Now(),
	}
	if err := a.Validate(); err != nil {
		return domain.Appointment{}, err
	}
	created, err := s.agenda.Create(ctx, a)
	if err != nil {
		return domain.Appointment{}, fmt.Errorf("criar na agenda: %w", err)
	}
	return created, s.mirrorUpsert(ctx, created)
}

// Reschedule changes time, duration or notes of an existing session.
func (s *AgendaService) Reschedule(ctx context.Context, id string, in RescheduleInput) (domain.Appointment, error) {
	a, err := s.agenda.Get(ctx, id)
	if err != nil {
		return domain.Appointment{}, err
	}
	if in.Start.IsZero() {
		ve := &domain.ValidationError{}
		ve.Add("start", "Informe data e hora")
		return domain.Appointment{}, ve
	}
	dur := in.Duration
	if dur <= 0 {
		dur = a.Duration()
		if dur <= 0 {
			dur = s.DefaultDuration
		}
	}
	a.Start = in.Start
	a.End = in.Start.Add(dur)
	a.AllDay = false
	a.Notes = strings.TrimSpace(in.Notes)
	a.UpdatedAt = s.clock.Now()
	if err := a.Validate(); err != nil {
		return domain.Appointment{}, err
	}
	if err := s.agenda.Update(ctx, a); err != nil {
		return domain.Appointment{}, fmt.Errorf("atualizar agenda: %w", err)
	}
	return a, s.mirrorUpsert(ctx, a)
}

// Cancel removes the session from the agenda and marks it cancelled in the mirror.
func (s *AgendaService) Cancel(ctx context.Context, id string) error {
	a, err := s.agenda.Get(ctx, id)
	if err != nil {
		return err
	}
	if err := s.agenda.Delete(ctx, id); err != nil {
		return fmt.Errorf("remover da agenda: %w", err)
	}
	a.Status = domain.StatusCancelled
	a.UpdatedAt = s.clock.Now()
	return s.mirrorUpsert(ctx, a)
}

// MarkDone flags the session as attended.
func (s *AgendaService) MarkDone(ctx context.Context, id string) (domain.Appointment, error) {
	return s.setStatus(ctx, id, domain.StatusDone)
}

func (s *AgendaService) setStatus(ctx context.Context, id string, st domain.AppointmentStatus) (domain.Appointment, error) {
	a, err := s.agenda.Get(ctx, id)
	if err != nil {
		return domain.Appointment{}, err
	}
	a.Status = st
	a.UpdatedAt = s.clock.Now()
	if err := s.agenda.Update(ctx, a); err != nil {
		return domain.Appointment{}, fmt.Errorf("atualizar agenda: %w", err)
	}
	return a, s.mirrorUpsert(ctx, a)
}

// Link attaches an agenda entry created outside the app to a patient.
func (s *AgendaService) Link(ctx context.Context, id, patientID string) (domain.Appointment, error) {
	p, err := s.patients.Get(ctx, patientID)
	if err != nil {
		return domain.Appointment{}, err
	}
	a, err := s.agenda.Get(ctx, id)
	if err != nil {
		return domain.Appointment{}, err
	}
	a.PatientID = p.ID
	a.Title = TitleFor(p)
	if a.Location == "" {
		a.Location = p.Address
	}
	if a.Status == "" {
		a.Status = domain.StatusScheduled
	}
	a.UpdatedAt = s.clock.Now()
	if err := s.agenda.Update(ctx, a); err != nil {
		return domain.Appointment{}, fmt.Errorf("atualizar agenda: %w", err)
	}
	return a, s.mirrorUpsert(ctx, a)
}

func (s *AgendaService) mirrorUpsert(ctx context.Context, a domain.Appointment) error {
	if err := s.mirror.Upsert(ctx, a); err != nil {
		return fmt.Errorf("%w: %v", ErrMirror, err)
	}
	return nil
}

// TitleFor is the calendar summary used for a patient's sessions.
func TitleFor(p domain.Patient) string { return "Fisio: " + p.Name }

func startOfDay(t time.Time, loc *time.Location) time.Time {
	t = t.In(loc)
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, loc)
}

func sortByStart(items []domain.Appointment) {
	sort.SliceStable(items, func(i, j int) bool { return items[i].Start.Before(items[j].Start) })
}
