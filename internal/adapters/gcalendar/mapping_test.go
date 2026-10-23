package gcalendar

import (
	"testing"
	"time"

	"google.golang.org/api/calendar/v3"

	"github.com/o-guardiao/deposito/internal/domain"
)

func TestRoundTripLinkedEvent(t *testing.T) {
	loc, _ := time.LoadLocation("America/Sao_Paulo")
	start := time.Date(2026, 10, 23, 9, 0, 0, 0, loc)
	in := domain.Appointment{
		ID: "abc", PatientID: "P0001", Title: "Fisio: Maria", Start: start, End: start.Add(time.Hour),
		Status: domain.StatusDone, Notes: "levar faixa", Location: "Rua X, 1",
	}
	ev := appointmentToEvent(in, DefaultReminders())
	if ev.ExtendedProperties.Private[propApp] != "1" || ev.ExtendedProperties.Private[propPatientID] != "P0001" {
		t.Fatalf("extended props missing: %+v", ev.ExtendedProperties.Private)
	}
	if ev.Reminders.UseDefault || len(ev.Reminders.Overrides) != 2 || ev.Reminders.Overrides[0].Minutes != 1440 || ev.Reminders.Overrides[1].Minutes != 60 {
		t.Fatalf("reminders wrong: %+v", ev.Reminders)
	}
	if ev.Start.TimeZone != "America/Sao_Paulo" {
		t.Fatalf("timezone: %q", ev.Start.TimeZone)
	}
	ev.Id = "abc"
	ev.Updated = "2026-10-20T10:00:00Z"
	out, err := eventToAppointment(ev, loc)
	if err != nil {
		t.Fatal(err)
	}
	if out.ID != in.ID || out.PatientID != in.PatientID || out.Title != in.Title || out.Notes != in.Notes ||
		out.Location != in.Location || out.Status != in.Status || !out.Start.Equal(in.Start) || !out.End.Equal(in.End) || out.AllDay {
		t.Fatalf("round trip mismatch:\n in=%+v\nout=%+v", in, out)
	}
	if out.UpdatedAt.IsZero() {
		t.Fatal("UpdatedAt not parsed")
	}
}

func TestUnlinkedEvent(t *testing.T) {
	ev := &calendar.Event{Id: "x", Summary: "Dentista",
		Start: &calendar.EventDateTime{DateTime: "2026-10-23T08:00:00-03:00"},
		End:   &calendar.EventDateTime{DateTime: "2026-10-23T09:00:00-03:00"}}
	out, err := eventToAppointment(ev, time.UTC)
	if err != nil {
		t.Fatal(err)
	}
	if out.Linked() || out.Title != "Dentista" || out.Status != domain.StatusScheduled {
		t.Fatalf("unexpected: %+v", out)
	}
	if out.Start.Hour() != 11 { // 08:00-03:00 in UTC
		t.Fatalf("start not converted to loc: %v", out.Start)
	}
}

func TestAllDayEvent(t *testing.T) {
	loc, _ := time.LoadLocation("America/Sao_Paulo")
	ev := &calendar.Event{Id: "d", Summary: "Feriado",
		Start: &calendar.EventDateTime{Date: "2026-11-15"}, End: &calendar.EventDateTime{Date: "2026-11-16"}}
	out, err := eventToAppointment(ev, loc)
	if err != nil {
		t.Fatal(err)
	}
	if !out.AllDay || out.Start.Hour() != 0 || out.Start.Location() != loc || out.Start.Day() != 15 {
		t.Fatalf("all-day mapping wrong: %+v", out)
	}
	back := appointmentToEvent(out, ReminderPolicy{})
	if back.Start.Date != "2026-11-15" || back.End.Date != "2026-11-16" {
		t.Fatalf("all-day not preserved: %+v %+v", back.Start, back.End)
	}
	if forPatch(back).Start.NullFields != nil {
		t.Fatal("an all-day patch must not null its own date")
	}
}

func TestUnknownStatusPropertyIsScheduled(t *testing.T) {
	ev := &calendar.Event{Id: "x", Summary: "S",
		ExtendedProperties: &calendar.EventExtendedProperties{Private: map[string]string{propPatientID: "P1", propStatus: "garbage"}},
		Start:              &calendar.EventDateTime{DateTime: "2026-10-23T08:00:00Z"}, End: &calendar.EventDateTime{DateTime: "2026-10-23T09:00:00Z"}}
	out, err := eventToAppointment(ev, time.UTC)
	if err != nil || out.Status != domain.StatusScheduled || out.PatientID != "P1" {
		t.Fatalf("unexpected: %v %+v", err, out)
	}
}
