// Package gcalendar implements domain.Agenda on top of Google Calendar.
package gcalendar

import (
	"time"

	"google.golang.org/api/calendar/v3"

	"github.com/o-guardiao/deposito/internal/domain"
)

// Private extended-property keys stored on each event the app manages.
const (
	propApp       = "fisio_app"
	propPatientID = "fisio_patient_id"
	propStatus    = "fisio_status"
)

// untitled is shown for events the user saved without a summary.
const untitled = "(sem título)"

// ReminderPolicy defines the popup reminders attached to app-created events.
type ReminderPolicy struct {
	// MinutesBefore lists each popup offset, e.g. {1440, 60} = 1 day and 1 hour before.
	MinutesBefore []int64
}

// DefaultReminders reminds one day and one hour before the session.
func DefaultReminders() ReminderPolicy { return ReminderPolicy{MinutesBefore: []int64{1440, 60}} }

func (p ReminderPolicy) toEvent() *calendar.EventReminders {
	r := &calendar.EventReminders{UseDefault: false, ForceSendFields: []string{"UseDefault"}}
	for _, m := range p.MinutesBefore {
		r.Overrides = append(r.Overrides, &calendar.EventReminder{Method: "popup", Minutes: m})
	}
	return r
}

// eventToAppointment converts a calendar event. Events without the app's
// patient property are returned unlinked with the raw summary as title.
func eventToAppointment(ev *calendar.Event, loc *time.Location) (domain.Appointment, error) {
	a := domain.Appointment{ID: ev.Id, Title: ev.Summary, Notes: ev.Description, Location: ev.Location, Status: domain.StatusScheduled}
	if a.Title == "" {
		a.Title = untitled
	}
	if ev.ExtendedProperties != nil && ev.ExtendedProperties.Private != nil {
		a.PatientID = ev.ExtendedProperties.Private[propPatientID]
		if st, ok := domain.ParseStatus(ev.ExtendedProperties.Private[propStatus]); ok {
			a.Status = st
		}
	}
	var err error
	a.Start, a.AllDay, err = parseEventTime(ev.Start, loc)
	if err != nil {
		return domain.Appointment{}, err
	}
	a.End, _, err = parseEventTime(ev.End, loc)
	if err != nil {
		return domain.Appointment{}, err
	}
	if ev.Updated != "" {
		if t, perr := time.Parse(time.RFC3339, ev.Updated); perr == nil {
			a.UpdatedAt = t
		}
	}
	return a, nil
}

func parseEventTime(dt *calendar.EventDateTime, loc *time.Location) (time.Time, bool, error) {
	if dt == nil {
		return time.Time{}, false, nil
	}
	if dt.Date != "" {
		t, err := time.ParseInLocation("2006-01-02", dt.Date, loc)
		return t, true, err
	}
	t, err := time.Parse(time.RFC3339, dt.DateTime)
	if err != nil {
		return time.Time{}, false, err
	}
	return t.In(loc), false, nil
}

// appointmentToEvent builds the writable event body for an app-managed
// appointment. Description and Location are always sent, so clearing them in
// the app clears them in Google too (a plain omitempty patch would keep the
// old text).
func appointmentToEvent(a domain.Appointment, rem ReminderPolicy) *calendar.Event {
	ev := &calendar.Event{
		Summary:     a.Title,
		Description: a.Notes,
		Location:    a.Location,
		ExtendedProperties: &calendar.EventExtendedProperties{Private: map[string]string{
			propApp:       "1",
			propPatientID: a.PatientID,
			propStatus:    string(a.Status),
		}},
		Reminders:       rem.toEvent(),
		ForceSendFields: []string{"Description", "Location"},
	}
	if a.AllDay {
		ev.Start = &calendar.EventDateTime{Date: a.Start.Format("2006-01-02")}
		ev.End = &calendar.EventDateTime{Date: a.End.Format("2006-01-02")}
	} else {
		ev.Start = &calendar.EventDateTime{DateTime: a.Start.Format(time.RFC3339), TimeZone: a.Start.Location().String()}
		ev.End = &calendar.EventDateTime{DateTime: a.End.Format(time.RFC3339), TimeZone: a.End.Location().String()}
	}
	return ev
}

// forPatch adapts a body built by appointmentToEvent to a PATCH: when the
// appointment is timed, the all-day "date" fields are explicitly nulled so an
// all-day event that was rescheduled to a time stops being all-day.
func forPatch(ev *calendar.Event) *calendar.Event {
	if ev.Start != nil && ev.Start.Date == "" {
		ev.Start.NullFields = []string{"Date"}
	}
	if ev.End != nil && ev.End.Date == "" {
		ev.End.NullFields = []string{"Date"}
	}
	return ev
}
