// Package domaintest holds contract tests every adapter of a domain port must
// pass, so backends behave identically where the use cases rely on it.
package domaintest

import (
	"context"
	"testing"
	"time"

	"github.com/o-guardiao/deposito/internal/domain"
)

// RunAgendaContract checks the ListBetween semantics of domain.Agenda:
// appointments whose Start lies in [from, to), sorted by Start, regardless of
// where they end. The agenda must start empty.
func RunAgendaContract(t *testing.T, agenda domain.Agenda, loc *time.Location) {
	t.Helper()
	ctx := context.Background()
	day := time.Date(2026, 10, 23, 0, 0, 0, 0, loc)
	next := day.AddDate(0, 0, 1)
	mk := func(name string, start time.Time, dur time.Duration) domain.Appointment {
		a, err := agenda.Create(ctx, domain.Appointment{Title: name, Start: start, End: start.Add(dur), Status: domain.StatusScheduled})
		if err != nil {
			t.Fatalf("create %s: %v", name, err)
		}
		return a
	}
	mk("yesterday-overlapping", day.Add(-30*time.Minute), 2*time.Hour)    // ends inside the day: excluded
	late := mk("late", day.Add(23*time.Hour+30*time.Minute), 2*time.Hour) // ends tomorrow: included
	early := mk("early", day, time.Hour)                                  // exactly at from: included
	mk("tomorrow", next, time.Hour)                                       // exactly at to: excluded

	got, err := agenda.ListBetween(ctx, day, next)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].ID != early.ID || got[1].ID != late.ID {
		names := make([]string, 0, len(got))
		for _, a := range got {
			names = append(names, a.Title)
		}
		t.Fatalf("ListBetween must return Start in [from,to) sorted; got %v", names)
	}
}
