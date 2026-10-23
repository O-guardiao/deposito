package app

import (
	"context"
	"fmt"
	"time"

	"github.com/o-guardiao/deposito/internal/domain"
)

// SyncService copies the live agenda into the spreadsheet mirror.
type SyncService struct {
	agenda domain.Agenda
	mirror domain.AppointmentMirror
	clock  domain.Clock
	// Past and Future bound the reconciliation window around now.
	Past   time.Duration
	Future time.Duration
}

// NewSyncService wires the dependencies.
func NewSyncService(a domain.Agenda, m domain.AppointmentMirror, c domain.Clock) *SyncService {
	return &SyncService{agenda: a, mirror: m, clock: c, Past: 90 * 24 * time.Hour, Future: 180 * 24 * time.Hour}
}

// Reconcile replaces the mirror contents with the linked agenda entries in the
// window and returns how many were copied.
func (s *SyncService) Reconcile(ctx context.Context) (int, error) {
	now := s.clock.Now()
	items, err := s.agenda.ListBetween(ctx, now.Add(-s.Past), now.Add(s.Future))
	if err != nil {
		return 0, fmt.Errorf("listar agenda: %w", err)
	}
	linked := items[:0:0]
	for _, a := range items {
		if a.Linked() {
			linked = append(linked, a)
		}
	}
	if err := s.mirror.ReplaceAll(ctx, linked); err != nil {
		return 0, fmt.Errorf("gravar planilha: %w", err)
	}
	return len(linked), nil
}
