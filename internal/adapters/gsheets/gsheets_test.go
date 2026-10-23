package gsheets

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/o-guardiao/deposito/internal/domain"
)

// fakeSheets is an in-memory grid per tab, enough to exercise the A1 ranges
// the adapter produces.
type fakeSheets struct {
	tabs map[string][][]string // includes header row at index 0
	ids  map[string]int64
}

func newFakeSheets() *fakeSheets {
	return &fakeSheets{tabs: map[string][][]string{}, ids: map[string]int64{}}
}

var rangeRe = regexp.MustCompile(`^([^!]+)!A(\d+)(?::[A-Z](\d*))?$`)

func (f *fakeSheets) parse(rng string) (tab string, start int, end int) {
	m := rangeRe.FindStringSubmatch(rng)
	if m == nil {
		panic("bad range " + rng)
	}
	start, _ = strconv.Atoi(m[2])
	end = -1
	if m[3] != "" {
		end, _ = strconv.Atoi(m[3])
	}
	return m[1], start - 1, end
}

func (f *fakeSheets) grow(tab string, n int) {
	for len(f.tabs[tab]) < n {
		f.tabs[tab] = append(f.tabs[tab], nil)
	}
}

func (f *fakeSheets) ReadRows(_ context.Context, _ string, rng string) ([][]string, error) {
	tab, start, end := f.parse(rng)
	rows := f.tabs[tab]
	if start >= len(rows) {
		return nil, nil
	}
	if end < 0 || end > len(rows) {
		end = len(rows)
	}
	var out [][]string
	for _, r := range rows[start:end] {
		out = append(out, append([]string(nil), r...))
	}
	return out, nil
}

func (f *fakeSheets) AppendRows(_ context.Context, _ string, rng string, rows [][]string) error {
	tab, _, _ := f.parse(rng)
	f.grow(tab, 1)
	f.tabs[tab] = append(f.tabs[tab], rows...)
	return nil
}

func (f *fakeSheets) UpdateRows(_ context.Context, _ string, rng string, rows [][]string) error {
	tab, start, _ := f.parse(rng)
	f.grow(tab, start+len(rows))
	for i, r := range rows {
		f.tabs[tab][start+i] = r
	}
	return nil
}

func (f *fakeSheets) ClearRange(_ context.Context, _ string, rng string) error {
	tab, start, _ := f.parse(rng)
	if start < len(f.tabs[tab]) {
		f.tabs[tab] = f.tabs[tab][:start]
	}
	return nil
}

func (f *fakeSheets) SheetIDs(context.Context, string) (map[string]int64, error) {
	out := map[string]int64{}
	for k, v := range f.ids {
		out[k] = v
	}
	return out, nil
}

func (f *fakeSheets) AddSheet(_ context.Context, _ string, title string) error {
	f.ids[title] = int64(len(f.ids) + 1)
	f.tabs[title] = nil
	return nil
}

func (f *fakeSheets) DeleteRow(_ context.Context, _ string, sheetID, row int64) error {
	for title, id := range f.ids {
		if id == sheetID {
			rows := f.tabs[title]
			f.tabs[title] = append(rows[:row], rows[row+1:]...)
			return nil
		}
	}
	return fmt.Errorf("sheet %d not found", sheetID)
}

func (f *fakeSheets) CreateSpreadsheet(ctx context.Context, _ string, tabs []string) (string, error) {
	for _, t := range tabs {
		_ = f.AddSheet(ctx, "", t)
	}
	return "sheet-1", nil
}

func TestRowMappingRoundTrip(t *testing.T) {
	loc, _ := time.LoadLocation("America/Sao_Paulo")
	now := time.Date(2026, 10, 23, 14, 5, 0, 0, loc)
	p := domain.Patient{ID: "P0001", Name: "Maria", Phone: "11 9", Address: "Rua", BirthDate: time.Date(1950, 2, 3, 0, 0, 0, 0, loc), Diagnosis: "dx", Notes: "n", CreatedAt: now, UpdatedAt: now}
	got := rowToPatient(patientToRow(p), loc)
	if got.ID != p.ID || got.Name != p.Name || !got.BirthDate.Equal(p.BirthDate) || !got.CreatedAt.Equal(now) || got.Notes != "n" {
		t.Fatalf("patient mismatch: %+v", got)
	}
	e := domain.Evolution{ID: "E1", PatientID: "P0001", Date: time.Date(2026, 1, 2, 0, 0, 0, 0, loc), Text: "t", CreatedAt: now}
	if ge := rowToEvolution(evolutionToRow(e), loc); ge != e {
		t.Fatalf("evolution mismatch: %+v vs %+v", ge, e)
	}
	a := domain.Appointment{ID: "ev", PatientID: "P0001", Title: "Fisio: Maria", Start: now, End: now.Add(time.Hour), Status: domain.StatusDone, Notes: "x", Location: "y", UpdatedAt: now}
	ga := rowToAppointment(appointmentToRow(a), loc)
	if ga.ID != a.ID || !ga.Start.Equal(a.Start) || !ga.End.Equal(a.End) || ga.Status != domain.StatusDone || ga.Location != "y" {
		t.Fatalf("appointment mismatch: %+v", ga)
	}
	if rowToPatient([]string{"P2", "Só nome"}, loc).Name != "Só nome" {
		t.Fatal("short rows must be tolerated")
	}
	if rowToPatient([]string{"P3", "BR", "", "", "25/12/1980"}, loc).BirthDate.Year() != 1980 {
		t.Fatal("dd/mm/yyyy typed by hand should parse")
	}
}

func TestBootstrapAndRepos(t *testing.T) {
	ctx := context.Background()
	loc := time.UTC
	api := newFakeSheets()
	id, err := Bootstrap(ctx, api, "")
	if err != nil || id != "sheet-1" {
		t.Fatalf("bootstrap: %v %q", err, id)
	}
	for _, tab := range AllTabs {
		if strings.Join(api.tabs[tab.Title][0], ",") != strings.Join(tab.Headers, ",") {
			t.Fatalf("header missing for %s: %v", tab.Title, api.tabs[tab.Title])
		}
	}
	// Second bootstrap on an existing sheet is a no-op.
	if id2, err := Bootstrap(ctx, api, id); err != nil || id2 != id || len(api.tabs[TabPatients.Title]) != 1 {
		t.Fatalf("re-bootstrap changed state: %v", err)
	}

	patients := NewPatientRepo(api, id, loc)
	now := time.Date(2026, 10, 23, 10, 0, 0, 0, loc)
	p := domain.Patient{ID: "P0001", Name: "Ana", Phone: "1", CreatedAt: now, UpdatedAt: now}
	if err := patients.Save(ctx, p); err != nil {
		t.Fatal(err)
	}
	if err := patients.Save(ctx, domain.Patient{ID: "P0002", Name: "Bruno", Phone: "2"}); err != nil {
		t.Fatal(err)
	}
	p.Name = "Ana Maria"
	if err := patients.Save(ctx, p); err != nil {
		t.Fatal(err)
	}
	all, _ := patients.List(ctx)
	if len(all) != 2 || all[0].Name != "Ana Maria" {
		t.Fatalf("upsert should update in place: %+v", all)
	}
	if got, err := patients.Get(ctx, "P0002"); err != nil || got.Name != "Bruno" {
		t.Fatalf("get: %v %+v", err, got)
	}
	if _, err := patients.Get(ctx, "nope"); !domain.IsNotFound(err) {
		t.Fatalf("want not found, got %v", err)
	}
	if res, _ := patients.Search(ctx, "bru"); len(res) != 1 {
		t.Fatalf("search: %+v", res)
	}

	evs := NewEvolutionRepo(api, id, loc)
	_ = evs.Add(ctx, domain.Evolution{ID: "E1", PatientID: "P0001", Date: now, Text: "a", CreatedAt: now})
	_ = evs.Add(ctx, domain.Evolution{ID: "E2", PatientID: "P0002", Date: now, Text: "b", CreatedAt: now})
	if l, _ := evs.ListByPatient(ctx, "P0001"); len(l) != 1 || l[0].Text != "a" {
		t.Fatalf("evolutions: %+v", l)
	}

	mirror := NewMirror(api, id)
	a := domain.Appointment{ID: "ev1", PatientID: "P0001", Title: "Fisio: Ana", Start: now, End: now.Add(time.Hour), Status: domain.StatusScheduled}
	_ = mirror.Upsert(ctx, a)
	a.Status = domain.StatusCancelled
	_ = mirror.Upsert(ctx, a)
	if rows := api.tabs[TabAppointments.Title]; len(rows) != 2 || rows[1][5] != "cancelado" {
		t.Fatalf("mirror upsert: %v", rows)
	}
	if err := mirror.Delete(ctx, "ev1"); err != nil || len(api.tabs[TabAppointments.Title]) != 1 {
		t.Fatalf("mirror delete: %v %v", err, api.tabs[TabAppointments.Title])
	}
	if err := mirror.ReplaceAll(ctx, []domain.Appointment{a, {ID: "ev2", Start: now, End: now}}); err != nil {
		t.Fatal(err)
	}
	if rows := api.tabs[TabAppointments.Title]; len(rows) != 3 || rows[0][0] != "EventoID" {
		t.Fatalf("replace all: %v", rows)
	}
	if err := mirror.ReplaceAll(ctx, nil); err != nil || len(api.tabs[TabAppointments.Title]) != 1 {
		t.Fatalf("replace all empty: %v %v", err, api.tabs[TabAppointments.Title])
	}
}
