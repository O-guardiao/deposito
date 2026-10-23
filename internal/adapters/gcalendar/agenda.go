package gcalendar

import (
	"context"
	"errors"
	"fmt"
	"time"

	"google.golang.org/api/calendar/v3"
	"google.golang.org/api/googleapi"

	"github.com/o-guardiao/deposito/internal/domain"
)

// EventsAPI is the slice of the Calendar API the adapter needs; it lets tests
// substitute a fake without network access.
type EventsAPI interface {
	List(ctx context.Context, calendarID string, from, to time.Time) ([]*calendar.Event, error)
	Get(ctx context.Context, calendarID, eventID string) (*calendar.Event, error)
	Insert(ctx context.Context, calendarID string, ev *calendar.Event) (*calendar.Event, error)
	Patch(ctx context.Context, calendarID, eventID string, ev *calendar.Event) (*calendar.Event, error)
	Delete(ctx context.Context, calendarID, eventID string) error
}

// Agenda implements domain.Agenda for one Google Calendar.
type Agenda struct {
	api        EventsAPI
	calendarID string
	loc        *time.Location
	reminders  ReminderPolicy
}

// NewAgenda creates the adapter. calendarID is usually "primary".
func NewAgenda(api EventsAPI, calendarID string, loc *time.Location, rem ReminderPolicy) *Agenda {
	if loc == nil {
		loc = time.UTC
	}
	return &Agenda{api: api, calendarID: calendarID, loc: loc, reminders: rem}
}

// ListBetween implements domain.Agenda.
func (g *Agenda) ListBetween(ctx context.Context, from, to time.Time) ([]domain.Appointment, error) {
	evs, err := g.api.List(ctx, g.calendarID, from, to)
	if err != nil {
		return nil, fmt.Errorf("calendar list: %w", err)
	}
	out := make([]domain.Appointment, 0, len(evs))
	for _, ev := range evs {
		if ev.Status == "cancelled" {
			continue
		}
		a, err := eventToAppointment(ev, g.loc)
		if err != nil {
			return nil, fmt.Errorf("evento %s: %w", ev.Id, err)
		}
		out = append(out, a)
	}
	return out, nil
}

// Get implements domain.Agenda.
func (g *Agenda) Get(ctx context.Context, id string) (domain.Appointment, error) {
	ev, err := g.api.Get(ctx, g.calendarID, id)
	if err != nil {
		return domain.Appointment{}, translate(id, err)
	}
	if ev.Status == "cancelled" {
		return domain.Appointment{}, fmt.Errorf("agendamento %q: %w", id, domain.ErrNotFound)
	}
	return eventToAppointment(ev, g.loc)
}

// Create implements domain.Agenda.
func (g *Agenda) Create(ctx context.Context, a domain.Appointment) (domain.Appointment, error) {
	ev, err := g.api.Insert(ctx, g.calendarID, appointmentToEvent(a, g.reminders))
	if err != nil {
		return domain.Appointment{}, fmt.Errorf("calendar insert: %w", err)
	}
	return eventToAppointment(ev, g.loc)
}

// Update implements domain.Agenda. Reminders are only (re)applied to
// app-managed fields; a patch keeps attendees and other data untouched.
func (g *Agenda) Update(ctx context.Context, a domain.Appointment) error {
	if _, err := g.api.Patch(ctx, g.calendarID, a.ID, appointmentToEvent(a, g.reminders)); err != nil {
		return translate(a.ID, err)
	}
	return nil
}

// Delete implements domain.Agenda.
func (g *Agenda) Delete(ctx context.Context, id string) error {
	if err := g.api.Delete(ctx, g.calendarID, id); err != nil {
		return translate(id, err)
	}
	return nil
}

func translate(id string, err error) error {
	var ge *googleapi.Error
	if errors.As(err, &ge) && (ge.Code == 404 || ge.Code == 410) {
		return fmt.Errorf("agendamento %q: %w", id, domain.ErrNotFound)
	}
	return fmt.Errorf("calendar: %w", err)
}

// ServiceAPI adapts the real *calendar.Service to EventsAPI.
type ServiceAPI struct{ svc *calendar.Service }

// NewServiceAPI wraps a calendar service.
func NewServiceAPI(svc *calendar.Service) *ServiceAPI { return &ServiceAPI{svc: svc} }

// List implements EventsAPI, expanding recurring events into single instances.
func (s *ServiceAPI) List(ctx context.Context, calendarID string, from, to time.Time) ([]*calendar.Event, error) {
	var out []*calendar.Event
	call := s.svc.Events.List(calendarID).Context(ctx).
		TimeMin(from.Format(time.RFC3339)).TimeMax(to.Format(time.RFC3339)).
		SingleEvents(true).OrderBy("startTime").MaxResults(2500)
	err := call.Pages(ctx, func(page *calendar.Events) error {
		out = append(out, page.Items...)
		return nil
	})
	return out, err
}

// Get implements EventsAPI.
func (s *ServiceAPI) Get(ctx context.Context, calendarID, eventID string) (*calendar.Event, error) {
	return s.svc.Events.Get(calendarID, eventID).Context(ctx).Do()
}

// Insert implements EventsAPI.
func (s *ServiceAPI) Insert(ctx context.Context, calendarID string, ev *calendar.Event) (*calendar.Event, error) {
	return s.svc.Events.Insert(calendarID, ev).Context(ctx).Do()
}

// Patch implements EventsAPI.
func (s *ServiceAPI) Patch(ctx context.Context, calendarID, eventID string, ev *calendar.Event) (*calendar.Event, error) {
	return s.svc.Events.Patch(calendarID, eventID, ev).Context(ctx).Do()
}

// Delete implements EventsAPI.
func (s *ServiceAPI) Delete(ctx context.Context, calendarID, eventID string) error {
	return s.svc.Events.Delete(calendarID, eventID).Context(ctx).Do()
}
