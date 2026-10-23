package memory

import (
	"context"
	"time"

	"github.com/o-guardiao/deposito/internal/domain"
)

// Store bundles the in-memory adapters so the demo can be wired in one call.
type Store struct {
	Patients   *PatientRepo
	Evolutions *EvolutionRepo
	Agenda     *Agenda
	Mirror     *Mirror
	IDs        *SequentialIDs
}

// NewStore creates empty in-memory adapters.
func NewStore() *Store {
	return &Store{
		Patients:   NewPatientRepo(),
		Evolutions: NewEvolutionRepo(),
		Agenda:     NewAgenda(),
		Mirror:     NewMirror(),
		IDs:        &SequentialIDs{},
	}
}

// SeedDemo fills the store with fictitious data around now (DEMO ONLY).
func (s *Store) SeedDemo(now time.Time, loc *time.Location) {
	ctx := context.Background()
	day := func(offset, hour, min int) time.Time {
		t := now.In(loc)
		return time.Date(t.Year(), t.Month(), t.Day()+offset, hour, min, 0, 0, loc)
	}
	type seedPatient struct {
		p    domain.Patient
		evs  []string
		apps [][3]int // dayOffset, hour, minute
	}
	seeds := []seedPatient{
		{domain.Patient{Name: "Maria Aparecida Souza", Phone: "11987654321", Address: "Rua das Flores, 120 - Vila Mariana", BirthDate: time.Date(1952, 3, 14, 0, 0, 0, 0, loc), Diagnosis: "Pós-operatório de artroplastia de joelho D", Notes: "Prefere atendimento pela manhã. Cachorro no portão."},
			[]string{"Amplitude de flexão 95°. Dor 4/10. Exercícios isométricos de quadríceps.", "Flexão 105°. Iniciada marcha com bengala no corredor."},
			[][3]int{{-7, 9, 0}, {-3, 9, 0}, {0, 9, 0}, {3, 9, 0}}},
		{domain.Patient{Name: "João Pedro Lima", Phone: "11912345678", Address: "Av. Paulista, 1500 ap 72", BirthDate: time.Date(1978, 11, 2, 0, 0, 0, 0, loc), Diagnosis: "Lombalgia crônica", Notes: ""},
			[]string{"Avaliação inicial. Teste de Lasègue negativo. Orientações posturais."},
			[][3]int{{0, 11, 0}, {2, 11, 0}}},
		{domain.Patient{Name: "Antônio Carlos Ferreira", Phone: "11955554444", Address: "Rua Harmonia, 88 - Vila Madalena", BirthDate: time.Date(1945, 7, 30, 0, 0, 0, 0, loc), Diagnosis: "AVC isquêmico - hemiparesia E", Notes: "Esposa acompanha a sessão."},
			[]string{"Treino de transferência cama-cadeira. Boa colaboração.", "Marcha assistida 10 m. Fadiga após 15 min."},
			[][3]int{{-1, 14, 0}, {1, 14, 0}, {4, 14, 0}}},
		{domain.Patient{Name: "Beatriz Oliveira", Phone: "11933332222", Address: "Rua Augusta, 900", BirthDate: time.Date(1990, 1, 25, 0, 0, 0, 0, loc), Diagnosis: "Tendinopatia do ombro D", Notes: ""},
			nil,
			[][3]int{{0, 16, 30}}},
		{domain.Patient{Name: "Sebastião Ramos", Phone: "11900001111", Address: "Rua do Bosque, 45 - Santana", BirthDate: time.Date(1960, 9, 9, 0, 0, 0, 0, loc), Diagnosis: "DPOC - reabilitação respiratória", Notes: "Oxigênio domiciliar."},
			[]string{"Exercícios respiratórios diafragmáticos. SpO2 94%."},
			[][3]int{{1, 9, 30}, {5, 9, 30}}},
	}
	for _, sd := range seeds {
		p := sd.p
		p.ID = s.IDs.NewPatientID()
		p.CreatedAt, p.UpdatedAt = now, now
		_ = s.Patients.Save(ctx, p)
		for i, txt := range sd.evs {
			_ = s.Evolutions.Add(ctx, domain.Evolution{
				ID: s.IDs.NewEvolutionID(), PatientID: p.ID,
				Date: day(-7+i*4, 0, 0), Text: txt, CreatedAt: now,
			})
		}
		for _, ap := range sd.apps {
			st := day(ap[0], ap[1], ap[2])
			status := domain.StatusScheduled
			if st.Before(now) {
				status = domain.StatusDone
			}
			a, _ := s.Agenda.Create(ctx, domain.Appointment{
				PatientID: p.ID, Title: p.SessionTitle(), Start: st, End: st.Add(time.Hour),
				Status: status, Location: p.Address, UpdatedAt: now,
			})
			_ = s.Mirror.Upsert(ctx, a)
		}
	}
	// An entry created directly on the phone, not yet linked to a patient.
	_, _ = s.Agenda.Create(ctx, domain.Appointment{
		Title: "Dona Lúcia - avaliação", Start: day(0, 13, 0), End: day(0, 14, 0),
		Status: domain.StatusScheduled, Location: "Rua Nova, 10", UpdatedAt: now,
	})
	_, _ = s.Agenda.Create(ctx, domain.Appointment{
		Title: "Dentista", Start: day(2, 8, 0), End: day(2, 9, 0),
		Status: domain.StatusScheduled, UpdatedAt: now,
	})
}
