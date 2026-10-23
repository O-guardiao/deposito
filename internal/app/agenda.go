package app

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/o-guardiao/deposito/internal/domain"
)

// ScheduleInput describes a new session.
type ScheduleInput struct {
	PatientID string
	Start     time.Time
	Duration  time.Duration
	Notes     string
}

func (in ScheduleInput) validate() error {
	ve := &domain.ValidationError{}
	if in.PatientID == "" {
		ve.Add("patientId", "Escolha o paciente")
	}
	if in.Start.IsZero() {
		ve.Add("start", "Informe data e hora")
	}
	if ve.Empty() {
		return nil
	}
	return ve
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

// WriteResult is the outcome of a successful agenda write. MirrorWarning is
// non-nil when the live agenda was updated but the spreadsheet mirror could
// not be; the appointment is valid and persisted either way, so callers show
// the warning instead of failing. Real failures come back as the error
// return of the method instead.
type WriteResult struct {
	Appointment   domain.Appointment
	MirrorWarning error
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
	return &AgendaService{agenda: a, mirror: m, patients: p, clock: c, loc: loc, DefaultDuration: domain.DefaultSessionDuration}
}

// Day lists the appointments of one calendar day, sorted by start.
func (s *AgendaService) Day(ctx context.Context, date time.Time) (DayView, error) {
	start := domain.StartOfDay(date, s.loc)
	items, err := s.agenda.ListBetween(ctx, start, nextDay(start))
	if err != nil {
		return DayView{}, fmt.Errorf("listar agenda: %w", err)
	}
	sortByStart(items)
	return DayView{Date: start, Items: items}, nil
}

// Week lists seven consecutive days starting at the given date, each with its
// items. Days are built by calendar arithmetic so the grouping is right even
// in time zones with daylight saving.
func (s *AgendaService) Week(ctx context.Context, start time.Time) ([]DayView, error) {
	first := domain.StartOfDay(start, s.loc)
	days := make([]DayView, 7)
	d := first
	for i := range days {
		days[i].Date = d
		d = nextDay(d)
	}
	items, err := s.agenda.ListBetween(ctx, first, d)
	if err != nil {
		return nil, fmt.Errorf("listar agenda: %w", err)
	}
	for _, a := range items {
		for i := range days {
			if domain.SameDay(a.Start, days[i].Date, s.loc) {
				days[i].Items = append(days[i].Items, a)
				break
			}
		}
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
// are derived from the patient record.
func (s *AgendaService) Schedule(ctx context.Context, in ScheduleInput) (WriteResult, error) {
	if err := in.validate(); err != nil {
		return WriteResult{}, err
	}
	p, err := s.patients.Get(ctx, in.PatientID)
	if err != nil {
		if domain.IsNotFound(err) {
			ve := &domain.ValidationError{}
			ve.Add("patientId", "Paciente não encontrado")
			return WriteResult{}, ve
		}
		return WriteResult{}, err
	}
	dur := in.Duration
	if dur <= 0 {
		dur = s.DefaultDuration
	}
	a := domain.Appointment{
		PatientID: p.ID,
		Title:     p.SessionTitle(),
		Start:     in.Start,
		End:       in.Start.Add(dur),
		Status:    domain.StatusScheduled,
		Notes:     strings.TrimSpace(in.Notes),
		Location:  p.Address,
		UpdatedAt: s.clock.Now(),
	}
	if err := a.Validate(); err != nil {
		return WriteResult{}, err
	}
	created, err := s.agenda.Create(ctx, a)
	if err != nil {
		return WriteResult{}, fmt.Errorf("criar na agenda: %w", err)
	}
	return s.mirrored(ctx, created), nil
}

// Reschedule changes time, duration or notes of an existing session. A
// session that had been marked as done goes back to scheduled, since it is
// being moved to a new moment.
func (s *AgendaService) Reschedule(ctx context.Context, id string, in RescheduleInput) (WriteResult, error) {
	a, err := s.agenda.Get(ctx, id)
	if err != nil {
		return WriteResult{}, err
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
	if a.Status == domain.StatusDone {
		a.Status = domain.StatusScheduled
	}
	a.UpdatedAt = s.clock.Now()
	if err := a.Validate(); err != nil {
		return WriteResult{}, err
	}
	if err := s.agenda.Update(ctx, a); err != nil {
		return WriteResult{}, fmt.Errorf("atualizar agenda: %w", err)
	}
	return s.mirrored(ctx, a), nil
}

// Cancel removes the session from the agenda and marks it cancelled in the
// mirror. The returned appointment is the cancelled one.
func (s *AgendaService) Cancel(ctx context.Context, id string) (WriteResult, error) {
	a, err := s.agenda.Get(ctx, id)
	if err != nil {
		return WriteResult{}, err
	}
	if err := s.agenda.Delete(ctx, id); err != nil {
		return WriteResult{}, fmt.Errorf("remover da agenda: %w", err)
	}
	a.Status = domain.StatusCancelled
	a.UpdatedAt = s.clock.Now()
	return s.mirrored(ctx, a), nil
}

// MarkDone flags the session as attended.
func (s *AgendaService) MarkDone(ctx context.Context, id string) (WriteResult, error) {
	a, err := s.agenda.Get(ctx, id)
	if err != nil {
		return WriteResult{}, err
	}
	a.Status = domain.StatusDone
	a.UpdatedAt = s.clock.Now()
	if err := s.agenda.Update(ctx, a); err != nil {
		return WriteResult{}, fmt.Errorf("atualizar agenda: %w", err)
	}
	return s.mirrored(ctx, a), nil
}

// Link attaches an agenda entry created outside the app to a patient.
func (s *AgendaService) Link(ctx context.Context, appointmentID, patientID string) (WriteResult, error) {
	p, err := s.patients.Get(ctx, patientID)
	if err != nil {
		return WriteResult{}, err
	}
	a, err := s.agenda.Get(ctx, appointmentID)
	if err != nil {
		return WriteResult{}, err
	}
	a.PatientID = p.ID
	a.Title = p.SessionTitle()
	if a.Location == "" {
		a.Location = p.Address
	}
	if a.Status == "" {
		a.Status = domain.StatusScheduled
	}
	a.UpdatedAt = s.clock.Now()
	if err := s.agenda.Update(ctx, a); err != nil {
		return WriteResult{}, fmt.Errorf("atualizar agenda: %w", err)
	}
	return s.mirrored(ctx, a), nil
}

// mirrored copies the appointment to the mirror and turns a failure there
// into a warning, because the agenda write has already succeeded.
func (s *AgendaService) mirrored(ctx context.Context, a domain.Appointment) WriteResult {
	res := WriteResult{Appointment: a}
	if err := s.mirror.Upsert(ctx, a); err != nil {
		res.MirrorWarning = fmt.Errorf("agenda gravada, mas a planilha não foi atualizada: %w", err)
	}
	return res
}

func nextDay(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day()+1, 0, 0, 0, 0, t.Location())
}

func sortByStart(items []domain.Appointment) {
	sort.SliceStable(items, func(i, j int) bool { return items[i].Start.Before(items[j].Start) })
}
