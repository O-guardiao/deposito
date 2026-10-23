package gcalendar

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/o-guardiao/deposito/internal/domain/domaintest"

	"google.golang.org/api/calendar/v3"
	"google.golang.org/api/googleapi"

	"github.com/o-guardiao/deposito/internal/domain"
)

type fakeAPI struct {
	events  map[string]*calendar.Event
	seq     int
	patched *calendar.Event
}

// List mimics Google: timeMin/timeMax select events that OVERLAP the window.
func (f *fakeAPI) List(_ context.Context, _ string, from, to time.Time) ([]*calendar.Event, error) {
	var out []*calendar.Event
	for _, e := range f.events {
		st, _, err1 := parseEventTime(e.Start, time.UTC)
		en, _, err2 := parseEventTime(e.End, time.UTC)
		if err1 != nil || err2 != nil || (en.After(from) && st.Before(to)) {
			out = append(out, e)
		}
	}
	return out, nil
}
func (f *fakeAPI) Get(_ context.Context, _, id string) (*calendar.Event, error) {
	e, ok := f.events[id]
	if !ok {
		return nil, &googleapi.Error{Code: 404}
	}
	return e, nil
}
func (f *fakeAPI) Insert(_ context.Context, _ string, ev *calendar.Event) (*calendar.Event, error) {
	f.seq++
	ev.Id = fmt.Sprintf("new%d", f.seq)
	f.events[ev.Id] = ev
	return ev, nil
}
func (f *fakeAPI) Patch(_ context.Context, _, id string, ev *calendar.Event) (*calendar.Event, error) {
	ev.Id = id
	f.patched = ev
	f.events[id] = ev
	return ev, nil
}
func (f *fakeAPI) Delete(_ context.Context, _, id string) error { delete(f.events, id); return nil }

func TestAgendaCreateGetDeleteAndNotFound(t *testing.T) {
	api := &fakeAPI{events: map[string]*calendar.Event{
		"gone": {Id: "gone", Status: "cancelled", Start: &calendar.EventDateTime{DateTime: "2026-10-23T08:00:00Z"}, End: &calendar.EventDateTime{DateTime: "2026-10-23T09:00:00Z"}},
	}}
	g := NewAgenda(api, "primary", time.UTC, DefaultReminders())
	ctx := context.Background()
	st := time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC)
	created, err := g.Create(ctx, domain.Appointment{PatientID: "P1", Title: "Fisio: A", Start: st, End: st.Add(time.Hour), Status: domain.StatusScheduled})
	if err != nil || created.ID != "new1" || created.PatientID != "P1" {
		t.Fatalf("create: %v %+v", err, created)
	}
	items, err := g.ListBetween(ctx, st.Add(-time.Hour), st.Add(time.Hour))
	if err != nil || len(items) != 1 { // cancelled event filtered out
		t.Fatalf("list: %v %d", err, len(items))
	}
	if _, err := g.Get(ctx, "gone"); !domain.IsNotFound(err) {
		t.Fatalf("cancelled should be not found, got %v", err)
	}
	if _, err := g.Get(ctx, "nope"); !domain.IsNotFound(err) {
		t.Fatalf("404 should translate to ErrNotFound, got %v", err)
	}
	if err := g.Delete(ctx, "new1"); err != nil {
		t.Fatal(err)
	}
}

func TestAgendaContract(t *testing.T) {
	loc, _ := time.LoadLocation("America/Sao_Paulo")
	domaintest.RunAgendaContract(t, NewAgenda(&fakeAPI{events: map[string]*calendar.Event{}}, "primary", loc, DefaultReminders()), loc)
}

func TestListSkipsUnparsableEventInsteadOfFailing(t *testing.T) {
	api := &fakeAPI{events: map[string]*calendar.Event{
		"bad":  {Id: "bad", Start: &calendar.EventDateTime{DateTime: "not-a-date"}, End: &calendar.EventDateTime{DateTime: "nope"}},
		"good": {Id: "good", Start: &calendar.EventDateTime{DateTime: "2026-10-23T10:00:00Z"}, End: &calendar.EventDateTime{DateTime: "2026-10-23T11:00:00Z"}},
	}}
	g := NewAgenda(api, "primary", time.UTC, DefaultReminders())
	from := time.Date(2026, 10, 23, 0, 0, 0, 0, time.UTC)
	items, err := g.ListBetween(context.Background(), from, from.AddDate(0, 0, 1))
	if err != nil || len(items) != 1 || items[0].ID != "good" || items[0].Title != untitled {
		t.Fatalf("want only the good event, titled %q: %v %+v", untitled, err, items)
	}
}

func TestUpdateClearsTextAndAllDayFields(t *testing.T) {
	api := &fakeAPI{events: map[string]*calendar.Event{}}
	g := NewAgenda(api, "primary", time.UTC, DefaultReminders())
	st := time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC)
	if err := g.Update(context.Background(), domain.Appointment{ID: "x", Title: "T", Start: st, End: st.Add(time.Hour), Status: domain.StatusScheduled}); err != nil {
		t.Fatal(err)
	}
	raw, err := api.patched.MarshalJSON()
	if err != nil {
		t.Fatal(err)
	}
	body := string(raw)
	for _, want := range []string{`"description":""`, `"location":""`, `"date":null`} {
		if !strings.Contains(body, want) {
			t.Errorf("patch body missing %s: %s", want, body)
		}
	}
}
