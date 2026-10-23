package app_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/o-guardiao/deposito/internal/adapters/memory"
	"github.com/o-guardiao/deposito/internal/app"
	"github.com/o-guardiao/deposito/internal/domain"
)

var (
	loc, _  = time.LoadLocation("America/Sao_Paulo")
	fixedTS = time.Date(2026, 10, 23, 8, 0, 0, 0, loc) // a Friday
)

type fixture struct {
	store *memory.Store
	*app.Services
	maria domain.Patient
}

func newFixtureIn(t *testing.T, loc *time.Location) *fixture {
	t.Helper()
	st := memory.NewStore()
	clock := domain.ClockFunc(func() time.Time { return fixedTS })
	f := &fixture{store: st, Services: app.NewServices(app.Deps{
		Patients: st.Patients, Evolutions: st.Evolutions, Agenda: st.Agenda, Mirror: st.Mirror, IDs: st.IDs, Clock: clock, Location: loc,
	})}
	p, err := f.Patients.Create(context.Background(), app.PatientInput{Name: "Maria", Address: "Rua A, 1", Phone: "11 9"})
	if err != nil {
		t.Fatal(err)
	}
	f.maria = p
	return f
}

func newFixture(t *testing.T) *fixture { return newFixtureIn(t, loc) }

func TestCreatePatientValidation(t *testing.T) {
	f := newFixture(t)
	_, err := f.Patients.Create(context.Background(), app.PatientInput{Name: "  "})
	ve, ok := domain.AsValidation(err)
	if !ok || ve.Fields["name"] == "" {
		t.Fatalf("want validation error on name, got %v", err)
	}
	if f.maria.ID != "P0001" || f.maria.CreatedAt != fixedTS {
		t.Fatalf("unexpected patient: %+v", f.maria)
	}
}

func TestScheduleDerivesTitleLocationAndMirrors(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	start := time.Date(2026, 10, 24, 9, 0, 0, 0, loc)
	res, err := f.Agenda.Schedule(ctx, app.ScheduleInput{PatientID: f.maria.ID, Start: start, Notes: "levar theraband"})
	if err != nil || res.MirrorWarning != nil {
		t.Fatal(err, res.MirrorWarning)
	}
	a := res.Appointment
	if a.ID == "" || a.Title != f.maria.SessionTitle() || a.Location != "Rua A, 1" || !a.End.Equal(start.Add(domain.DefaultSessionDuration)) || a.Status != domain.StatusScheduled {
		t.Fatalf("unexpected appointment: %+v", a)
	}
	if m, ok := f.store.Mirror.Get(a.ID); !ok || m.Title != a.Title {
		t.Fatalf("mirror not updated: %+v %v", m, ok)
	}
	day, _ := f.Agenda.Day(ctx, start)
	if len(day.Items) != 1 || day.Items[0].ID != a.ID {
		t.Fatalf("day view: %+v", day)
	}
	// Validation: unknown patient and missing start.
	if _, err := f.Agenda.Schedule(ctx, app.ScheduleInput{PatientID: "nope", Start: start}); err == nil {
		t.Fatal("unknown patient should fail")
	} else if ve, ok := domain.AsValidation(err); !ok || ve.Fields["patientId"] == "" {
		t.Fatalf("want validation on patientId, got %v", err)
	}
	if _, err := f.Agenda.Schedule(ctx, app.ScheduleInput{PatientID: f.maria.ID}); err == nil {
		t.Fatal("missing start should fail")
	} else if ve, ok := domain.AsValidation(err); !ok || ve.Fields["start"] == "" {
		t.Fatalf("want validation on start, got %v", err)
	}
}

func TestMirrorFailureIsAWarningNotALoss(t *testing.T) {
	f := newFixture(t)
	f.store.Mirror.Fail = errors.New("quota")
	res, err := f.Agenda.Schedule(context.Background(), app.ScheduleInput{PatientID: f.maria.ID, Start: fixedTS.Add(24 * time.Hour)})
	if err != nil {
		t.Fatalf("mirror failure must not be an error: %v", err)
	}
	if res.MirrorWarning == nil || !errors.Is(res.MirrorWarning, f.store.Mirror.Fail) {
		t.Fatalf("want warning wrapping the cause, got %v", res.MirrorWarning)
	}
	if res.Appointment.ID == "" {
		t.Fatal("appointment must still be returned")
	}
	if _, err := f.store.Agenda.Get(context.Background(), res.Appointment.ID); err != nil {
		t.Fatal("calendar write must be kept")
	}
}

func TestCancelAndMarkDone(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	res, _ := f.Agenda.Schedule(ctx, app.ScheduleInput{PatientID: f.maria.ID, Start: fixedTS.Add(2 * time.Hour)})
	a := res.Appointment
	done, err := f.Agenda.MarkDone(ctx, a.ID)
	if err != nil || done.Appointment.Status != domain.StatusDone {
		t.Fatalf("mark done: %v %+v", err, done)
	}
	if m, _ := f.store.Mirror.Get(a.ID); m.Status != domain.StatusDone {
		t.Fatalf("mirror status: %+v", m)
	}
	// Rescheduling a finished session makes it scheduled again.
	re, err := f.Agenda.Reschedule(ctx, a.ID, app.RescheduleInput{Start: fixedTS.Add(72 * time.Hour)})
	if err != nil || re.Appointment.Status != domain.StatusScheduled {
		t.Fatalf("reschedule of done: %v %+v", err, re.Appointment)
	}
	cancelled, err := f.Agenda.Cancel(ctx, a.ID)
	if err != nil || cancelled.Appointment.ID != a.ID || cancelled.Appointment.Status != domain.StatusCancelled {
		t.Fatalf("cancel: %v %+v", err, cancelled)
	}
	if _, err := f.store.Agenda.Get(ctx, a.ID); !domain.IsNotFound(err) {
		t.Fatalf("event should be gone from agenda, got %v", err)
	}
	if m, ok := f.store.Mirror.Get(a.ID); !ok || m.Status != domain.StatusCancelled {
		t.Fatalf("mirror should keep a cancelled row: %+v %v", m, ok)
	}
	if _, err := f.Agenda.Cancel(ctx, a.ID); !domain.IsNotFound(err) {
		t.Fatalf("second cancel: %v", err)
	}
}

func TestRescheduleKeepsDurationAndValidates(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	res, _ := f.Agenda.Schedule(ctx, app.ScheduleInput{PatientID: f.maria.ID, Start: fixedTS, Duration: 90 * time.Minute})
	ns := fixedTS.Add(48 * time.Hour)
	r, err := f.Agenda.Reschedule(ctx, res.Appointment.ID, app.RescheduleInput{Start: ns, Notes: "novo"})
	if err != nil || !r.Appointment.Start.Equal(ns) || r.Appointment.Duration() != 90*time.Minute || r.Appointment.Notes != "novo" {
		t.Fatalf("reschedule: %v %+v", err, r)
	}
	if _, err := f.Agenda.Reschedule(ctx, res.Appointment.ID, app.RescheduleInput{}); err == nil {
		t.Fatal("zero start must fail validation")
	} else if ve, ok := domain.AsValidation(err); !ok || ve.Fields["start"] == "" {
		t.Fatalf("want validation on start, got %v", err)
	}
}

func TestLinkUnlinkedEvent(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	loose, _ := f.store.Agenda.Create(ctx, domain.Appointment{Title: "Dona Lúcia", Start: fixedTS, End: fixedTS.Add(time.Hour), Location: "Rua B"})
	if loose.Linked() {
		t.Fatal("precondition")
	}
	res, err := f.Agenda.Link(ctx, loose.ID, f.maria.ID)
	linked := res.Appointment
	if err != nil || linked.PatientID != f.maria.ID || linked.Title != "Fisio: Maria" || linked.Location != "Rua B" || linked.Status != domain.StatusScheduled {
		t.Fatalf("link: %v %+v", err, linked)
	}
	if _, ok := f.store.Mirror.Get(loose.ID); !ok {
		t.Fatal("linked event should be mirrored")
	}
	if _, err := f.Agenda.Link(ctx, loose.ID, "nope"); !domain.IsNotFound(err) {
		t.Fatalf("unknown patient: %v", err)
	}
}

func TestWeekGroupsByDayInLocation(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	// 23:30 local on Monday must land on Monday, not Tuesday (UTC would say Tuesday).
	monday := time.Date(2026, 10, 19, 23, 30, 0, 0, loc)
	_, _ = f.Agenda.Schedule(ctx, app.ScheduleInput{PatientID: f.maria.ID, Start: monday})
	_, _ = f.Agenda.Schedule(ctx, app.ScheduleInput{PatientID: f.maria.ID, Start: monday.AddDate(0, 0, 3)})
	_, _ = f.Agenda.Schedule(ctx, app.ScheduleInput{PatientID: f.maria.ID, Start: monday.AddDate(0, 0, 9)}) // outside
	week, err := f.Agenda.Week(ctx, monday.In(time.UTC))
	if err != nil || len(week) != 7 {
		t.Fatalf("week: %v %d", err, len(week))
	}
	if len(week[0].Items) != 1 || len(week[3].Items) != 1 || len(week[1].Items) != 0 {
		t.Fatalf("grouping wrong: %d %d %d", len(week[0].Items), len(week[1].Items), len(week[3].Items))
	}
	if week[0].Date.Weekday() != time.Monday || week[0].Date.Hour() != 0 {
		t.Fatalf("first day: %v", week[0].Date)
	}
}

// TestWeekAcrossDaylightSaving uses a zone with DST (America/Santiago, which
// springs forward on the first Sunday of September) so a 23-hour day cannot
// shift the grouping.
func TestWeekAcrossDaylightSaving(t *testing.T) {
	stgo, err := time.LoadLocation("America/Santiago")
	if err != nil {
		t.Skip("tzdata unavailable")
	}
	f := newFixtureIn(t, stgo)
	ctx := context.Background()
	sat := time.Date(2026, 9, 5, 0, 0, 0, 0, stgo) // week with the DST change on Sunday 6th
	for d := 0; d < 7; d++ {
		_, _ = f.Agenda.Schedule(ctx, app.ScheduleInput{PatientID: f.maria.ID, Start: time.Date(2026, 9, 5+d, 10, 0, 0, 0, stgo)})
		_, _ = f.Agenda.Schedule(ctx, app.ScheduleInput{PatientID: f.maria.ID, Start: time.Date(2026, 9, 5+d, 23, 30, 0, 0, stgo)})
	}
	week, err := f.Agenda.Week(ctx, sat)
	if err != nil {
		t.Fatal(err)
	}
	for i, d := range week {
		if d.Date.Day() != 5+i || d.Date.Hour() != 0 {
			t.Fatalf("day %d label wrong: %v", i, d.Date)
		}
		if len(d.Items) != 2 {
			t.Fatalf("day %d (%v) has %d items, want 2", i, d.Date, len(d.Items))
		}
		for _, a := range d.Items {
			if a.Start.In(stgo).Day() != d.Date.Day() {
				t.Fatalf("item %v grouped under %v", a.Start, d.Date)
			}
		}
	}
}

func TestRecordSplitsPastAndUpcoming(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	_, _ = f.Agenda.Schedule(ctx, app.ScheduleInput{PatientID: f.maria.ID, Start: fixedTS.AddDate(0, 0, -3)})
	_, _ = f.Agenda.Schedule(ctx, app.ScheduleInput{PatientID: f.maria.ID, Start: fixedTS.AddDate(0, 0, 3)})
	other, _ := f.Patients.Create(ctx, app.PatientInput{Name: "Zé"})
	_, _ = f.Agenda.Schedule(ctx, app.ScheduleInput{PatientID: other.ID, Start: fixedTS.AddDate(0, 0, 1)})
	if _, err := f.Evolutions.Add(ctx, app.EvolutionInput{PatientID: f.maria.ID, Text: "primeira sessão"}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.Evolutions.Add(ctx, app.EvolutionInput{PatientID: f.maria.ID, Date: fixedTS, Text: "   "}); err == nil {
		t.Fatal("empty text should fail validation")
	}
	if _, err := f.Evolutions.Add(ctx, app.EvolutionInput{PatientID: "nope", Date: fixedTS, Text: "x"}); !domain.IsNotFound(err) {
		t.Fatalf("unknown patient: %v", err)
	}
	rec, err := f.Patients.GetRecord(ctx, f.maria.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(rec.Past) != 1 || len(rec.Upcoming) != 1 || len(rec.Evolutions) != 1 || rec.Evolutions[0].Date != fixedTS {
		t.Fatalf("record: past=%d upcoming=%d evos=%+v", len(rec.Past), len(rec.Upcoming), rec.Evolutions)
	}
	if _, err := f.Patients.GetRecord(ctx, "nope"); !domain.IsNotFound(err) {
		t.Fatalf("want not found: %v", err)
	}
}

func TestSearchAndListSorted(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	_, _ = f.Patients.Create(ctx, app.PatientInput{Name: "antônio", Phone: "999"})
	_, _ = f.Patients.Create(ctx, app.PatientInput{Name: "Beatriz"})
	all, _ := f.Patients.List(ctx)
	if len(all) != 3 || all[0].Name != "antônio" || all[2].Name != "Maria" {
		t.Fatalf("sort: %+v", all)
	}
	if res, _ := f.Patients.Search(ctx, "999"); len(res) != 1 || res[0].Name != "antônio" {
		t.Fatalf("search by phone: %+v", res)
	}
	if res, _ := f.Patients.Search(ctx, "MAR"); len(res) != 1 {
		t.Fatalf("case-insensitive search: %+v", res)
	}
}

func TestReconcileCopiesLinkedAndKeepsCancelled(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	_, _ = f.Agenda.Schedule(ctx, app.ScheduleInput{PatientID: f.maria.ID, Start: fixedTS.Add(time.Hour)})
	_, _ = f.store.Agenda.Create(ctx, domain.Appointment{Title: "Dentista", Start: fixedTS, End: fixedTS.Add(time.Hour)})
	_ = f.store.Mirror.Upsert(ctx, domain.Appointment{ID: "stale", Status: domain.StatusScheduled})
	_ = f.store.Mirror.Upsert(ctx, domain.Appointment{ID: "cancelled-history", Status: domain.StatusCancelled})
	n, err := f.Sync.Reconcile(ctx)
	if err != nil || n != 1 || f.store.Mirror.Len() != 2 {
		t.Fatalf("reconcile: %v n=%d len=%d", err, n, f.store.Mirror.Len())
	}
	if _, ok := f.store.Mirror.Get("cancelled-history"); !ok {
		t.Fatal("cancelled rows are history and must survive a sync")
	}
	if _, ok := f.store.Mirror.Get("stale"); ok {
		t.Fatal("stale scheduled rows must be dropped")
	}
}
