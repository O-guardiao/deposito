package app

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/o-guardiao/deposito/internal/domain"
)

// EvolutionInput is a session note to record.
type EvolutionInput struct {
	PatientID string
	Date      time.Time // zero means "now"
	Text      string
}

// EvolutionService records session notes.
type EvolutionService struct {
	evolutions domain.EvolutionRepository
	patients   domain.PatientRepository
	ids        domain.IDGenerator
	clock      domain.Clock
}

// NewEvolutionService wires the dependencies.
func NewEvolutionService(e domain.EvolutionRepository, p domain.PatientRepository, ids domain.IDGenerator, c domain.Clock) *EvolutionService {
	return &EvolutionService{evolutions: e, patients: p, ids: ids, clock: c}
}

// Add validates and stores a note for an existing patient.
func (s *EvolutionService) Add(ctx context.Context, in EvolutionInput) (domain.Evolution, error) {
	if _, err := s.patients.Get(ctx, in.PatientID); err != nil {
		return domain.Evolution{}, err
	}
	now := s.clock.Now()
	date := in.Date
	if date.IsZero() {
		date = now
	}
	e := domain.Evolution{
		ID:        s.ids.NewEvolutionID(),
		PatientID: in.PatientID,
		Date:      date,
		Text:      strings.TrimSpace(in.Text),
		CreatedAt: now,
	}
	if err := e.Validate(); err != nil {
		return domain.Evolution{}, err
	}
	if err := s.evolutions.Add(ctx, e); err != nil {
		return domain.Evolution{}, fmt.Errorf("salvar evolução: %w", err)
	}
	return e, nil
}
