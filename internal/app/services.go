package app

import (
	"time"

	"github.com/o-guardiao/deposito/internal/domain"
)

// Window bounds how far behind and ahead of "now" the agenda is read.
type Window struct {
	Behind time.Duration
	Ahead  time.Duration
}

// Deps are the ports every use case is built from.
type Deps struct {
	Patients   domain.PatientRepository
	Evolutions domain.EvolutionRepository
	Agenda     domain.Agenda
	Mirror     domain.AppointmentMirror
	IDs        domain.IDGenerator
	Clock      domain.Clock
	Location   *time.Location
}

// Services bundles the use cases so the composition root and the delivery
// layer share one definition of "everything the app can do".
type Services struct {
	Patients   *PatientService
	Evolutions *EvolutionService
	Agenda     *AgendaService
	Sync       *SyncService
}

// NewServices wires every use case from the same dependencies.
func NewServices(d Deps) *Services {
	if d.Location == nil {
		d.Location = time.UTC
	}
	if d.Clock == nil {
		d.Clock = domain.ClockFunc(time.Now)
	}
	return &Services{
		Patients:   NewPatientService(d.Patients, d.Evolutions, d.Agenda, d.IDs, d.Clock),
		Evolutions: NewEvolutionService(d.Evolutions, d.Patients, d.IDs, d.Clock),
		Agenda:     NewAgendaService(d.Agenda, d.Mirror, d.Patients, d.Clock, d.Location),
		Sync:       NewSyncService(d.Agenda, d.Mirror, d.Clock),
	}
}
