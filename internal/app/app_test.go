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
	store    *memory.Store
	patients *app.PatientService
	agenda   *app.AgendaService
	evos     *app.EvolutionService
	sync     *app.SyncService
	maria    domain.Patient
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	st := memory.NewStore()
	clock := domain.ClockFunc(func() time.Time { return fixedTS })
	f := &fixture{
		store:    st,
		patients: app.NewPatientService(st.Patients, st.Evolutions, st.Agenda, st.IDs, clock),
		agenda:   app.NewAgendaService(st.Agenda, st.Mirror, st.Patients, clock, loc),
		evos:     app.NewEvolutionService(st.Evolutions, st.Patients, st.IDs, clock),
		sync:     app.NewSyncService(st.Agenda, st.Mirror, clock),
	}
	p, err := f.patients.Create(context.Background(), app.PatientInput{Name: "Maria", Address: "Rua A, 1", Phone: "11 9"})
	if err != nil {
		t.Fatal(err)
	}
	f.maria = p
	return f
}

func TestCreatePatientValidation(t *testing.T) {
	f := newFixture(t)
	_, err := f.patients.Create(context.Background(), app.PatientInput{Name: "  "})
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
	a, err := f.agenda.Schedule(ctx, app.ScheduleInput{PatientID: f.maria.ID, Start: start, Notes: "levar theraband"})
	if err != nil {
		t.Fatal(err)
	}
	if a.ID == "" || a.Title != "Fisio: Maria" || a.Location != "Rua A, 1" || !a.End.Equal(start.Add(time.Hour)) || a.Status != domain.StatusScheduled {
		t.Fatalf("unexpected appointment: %+v", a)
	}
	if m, ok := f.store.Mirror.Get(a.ID); !ok || m.Title != a.Title {
		t.Fatalf("mirror not updated: %+v %v", m, ok)
	}
	day, _ := f.agenda.Day(ctx, start)
	if len(day.Items) != 1 || day.Items[0].ID != a.ID {
		t.Fatalf("day view: %+v", day)
	}
	// Validation: unknown patient and missing start.
	if _, err := f.agenda.Schedule(ctx, app.ScheduleInput{PatientID: "nope", Start: start}); err == nil {
		t.Fatal("unknown patient should fail")
	} else if ve, ok := domain.AsValidation(err); !ok || ve.Fields["patientId"] == "" {
		t.Fatalf("want validation on patientId, got %v", err)
	}
	if _, err := f.agenda.Schedule(ctx, app.ScheduleInput{PatientID: f.maria.ID}); err == nil {
		t.Fatal("missing start should fail")
	}
}

func TestMirrorFailureIsAWarningNotALoss(t *testing.T) {
	f := newFixture(t)
	f.store.Mirror.Fail = errors.New("quota")
	a, err := f.agenda.Schedule(context.Background(), app.ScheduleInput{PatientID: f.maria.ID, Start: fixedTS.Add(24 * time.Hour)})
	if !errors.Is(err, app.ErrMirror) {
		t.Fatalf("want ErrMirror, got %v", err)
	}
	if a.ID == "" {
		t.Fatal("appointment must still be returned")
	}
	if _, err := f.store.Agenda.Get(context.Background(), a.ID); err != nil {
		t.Fatal("calendar write must be kept")
	}
}

func TestCancelAndMarkDone(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	a, _ := f.agenda.Schedule(ctx, app.ScheduleInput{PatientID: f.maria.ID, Start: fixedTS.Add(2 * time.Hour)})
	done, err := f.agenda.MarkDone(ctx, a.ID)
	if err != nil || done.Status != domain.StatusDone {
		t.Fatalf("mark done: %v %+v", err, done)
	}
	if m, _ := f.store.Mirror.Get(a.ID); m.Status != domain.StatusDone {
		t.Fatalf("mirror status: %+v", m)
	}
	if err := f.agenda.Cancel(ctx, a.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.Agenda.Get(ctx, a.ID); !domain.IsNotFound(err) {
		t.Fatalf("event should be gone from agenda, got %v", err)
	}
	if m, ok := f.store.Mirror.Get(a.ID); !ok || m.Status != domain.StatusCancelled {
		t.Fatalf("mirror should keep a cancelled row: %+v %v", m, ok)
	}
	if err := f.agenda.Cancel(ctx, a.ID); !domain.IsNotFound(err) {
		t.Fatalf("second cancel: %v", err)
	}
}

func TestRescheduleKeepsDuration(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	a, _ := f.agenda.Schedule(ctx, app.ScheduleInput{PatientID: f.maria.ID, Start: fixedTS, Duration: 90 * time.Minute})
	ns := fixedTS.Add(48 * time.Hour)
	r, err := f.agenda.Reschedule(ctx, a.ID, app.RescheduleInput{Start: ns, Notes: "novo"})
	if err != nil || !r.Start.Equal(ns) || r.Duration() != 90*time.Minute || r.Notes != "novo" {
		t.Fatalf("reschedule: %v %+v", err, r)
	}
}

func TestLinkUnlinkedEvent(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	loose, _ := f.store.Agenda.Create(ctx, domain.Appointment{Title: "Dona Lúcia", Start: fixedTS, End: fixedTS.Add(time.Hour), Location: "Rua B"})
	if loose.Linked() {
		t.Fatal("precondition")
	}
	linked, err := f.agenda.Link(ctx, loose.ID, f.maria.ID)
	if err != nil || linked.PatientID != f.maria.ID || linked.Title != "Fisio: Maria" || linked.Location != "Rua B" || linked.Status != domain.StatusScheduled {
		t.Fatalf("link: %v %+v", err, linked)
	}
	if _, ok := f.store.Mirror.Get(loose.ID); !ok {
		t.Fatal("linked event should be mirrored")
	}
	if _, err := f.agenda.Link(ctx, loose.ID, "nope"); !domain.IsNotFound(err) {
		t.Fatalf("unknown patient: %v", err)
	}
}

func TestWeekGroupsByDayInLocation(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	// 23:30 local on Monday must land on Monday, not Tuesday (UTC would say Tuesday).
	monday := time.Date(2026, 10, 19, 23, 30, 0, 0, loc)
	_, _ = f.agenda.Schedule(ctx, app.ScheduleInput{PatientID: f.maria.ID, Start: monday})
	_, _ = f.agenda.Schedule(ctx, app.ScheduleInput{PatientID: f.maria.ID, Start: monday.AddDate(0, 0, 3)})
	_, _ = f.agenda.Schedule(ctx, app.ScheduleInput{PatientID: f.maria.ID, Start: monday.AddDate(0, 0, 9)}) // outside
	week, err := f.agenda.Week(ctx, monday.In(time.UTC))
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

func TestRecordSplitsPastAndUpcoming(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	_, _ = f.agenda.Schedule(ctx, app.ScheduleInput{PatientID: f.maria.ID, Start: fixedTS.AddDate(0, 0, -3)})
	_, _ = f.agenda.Schedule(ctx, app.ScheduleInput{PatientID: f.maria.ID, Start: fixedTS.AddDate(0, 0, 3)})
	other, _ := f.patients.Create(ctx, app.PatientInput{Name: "Zé"})
	_, _ = f.agenda.Schedule(ctx, app.ScheduleInput{PatientID: other.ID, Start: fixedTS.AddDate(0, 0, 1)})
	if _, err := f.evos.Add(ctx, f.maria.ID, time.Time{}, "primeira sessão"); err != nil {
		t.Fatal(err)
	}
	if _, err := f.evos.Add(ctx, f.maria.ID, fixedTS, "   "); err == nil {
		t.Fatal("empty text should fail validation")
	}
	if _, err := f.evos.Add(ctx, "nope", fixedTS, "x"); !domain.IsNotFound(err) {
		t.Fatalf("unknown patient: %v", err)
	}
	rec, err := f.patients.Record(ctx, f.maria.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(rec.Past) != 1 || len(rec.Upcoming) != 1 || len(rec.Evolutions) != 1 || rec.Evolutions[0].Date != fixedTS {
		t.Fatalf("record: past=%d upcoming=%d evos=%+v", len(rec.Past), len(rec.Upcoming), rec.Evolutions)
	}
	if _, err := f.patients.Record(ctx, "nope"); !domain.IsNotFound(err) {
		t.Fatalf("want not found: %v", err)
	}
}

func TestSearchAndListSorted(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	_, _ = f.patients.Create(ctx, app.PatientInput{Name: "antônio", Phone: "999"})
	_, _ = f.patients.Create(ctx, app.PatientInput{Name: "Beatriz"})
	all, _ := f.patients.List(ctx)
	if len(all) != 3 || all[0].Name != "antônio" || all[2].Name != "Maria" {
		t.Fatalf("sort: %+v", all)
	}
	if res, _ := f.patients.Search(ctx, "999"); len(res) != 1 || res[0].Name != "antônio" {
		t.Fatalf("search by phone: %+v", res)
	}
	if res, _ := f.patients.Search(ctx, "MAR"); len(res) != 1 {
		t.Fatalf("case-insensitive search: %+v", res)
	}
}

func TestReconcileCopiesOnlyLinked(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	_, _ = f.agenda.Schedule(ctx, app.ScheduleInput{PatientID: f.maria.ID, Start: fixedTS.Add(time.Hour)})
	_, _ = f.store.Agenda.Create(ctx, domain.Appointment{Title: "Dentista", Start: fixedTS, End: fixedTS.Add(time.Hour)})
	_ = f.store.Mirror.Upsert(ctx, domain.Appointment{ID: "stale", Status: domain.StatusCancelled})
	n, err := f.sync.Reconcile(ctx)
	if err != nil || n != 1 || f.store.Mirror.Len() != 1 {
		t.Fatalf("reconcile: %v n=%d len=%d", err, n, f.store.Mirror.Len())
	}
}
