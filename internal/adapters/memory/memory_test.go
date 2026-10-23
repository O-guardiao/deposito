package memory

import (
	"context"
	"testing"
	"time"

	"github.com/o-guardiao/deposito/internal/domain"
	"github.com/o-guardiao/deposito/internal/domain/domaintest"
)

func TestAgendaContract(t *testing.T) {
	loc, _ := time.LoadLocation("America/Sao_Paulo")
	domaintest.RunAgendaContract(t, NewAgenda(), loc)
}

func TestMirrorReconcileKeepsCancelled(t *testing.T) {
	ctx := context.Background()
	m := NewMirror()
	_ = m.Upsert(ctx, domain.Appointment{ID: "old", Status: domain.StatusCancelled})
	_ = m.Upsert(ctx, domain.Appointment{ID: "gone", Status: domain.StatusScheduled})
	_ = m.Upsert(ctx, domain.Appointment{ID: "live", Status: domain.StatusScheduled, Notes: "stale"})
	if err := m.Reconcile(ctx, []domain.Appointment{{ID: "live", Status: domain.StatusDone}}); err != nil {
		t.Fatal(err)
	}
	if m.Len() != 2 {
		t.Fatalf("want cancelled + live, got %d", m.Len())
	}
	if a, ok := m.Get("live"); !ok || a.Status != domain.StatusDone {
		t.Fatalf("live row not replaced: %+v", a)
	}
	if _, ok := m.Get("gone"); ok {
		t.Fatal("scheduled row absent from the agenda must be dropped")
	}
}
