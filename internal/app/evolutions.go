package app

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/o-guardiao/deposito/internal/domain"
)

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
func (s *EvolutionService) Add(ctx context.Context, patientID string, date time.Time, text string) (domain.Evolution, error) {
	if _, err := s.patients.Get(ctx, patientID); err != nil {
		return domain.Evolution{}, err
	}
	now := s.clock.Now()
	if date.IsZero() {
		date = now
	}
	e := domain.Evolution{
		ID:        s.ids.NewEvolutionID(),
		PatientID: patientID,
		Date:      date,
		Text:      strings.TrimSpace(text),
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
