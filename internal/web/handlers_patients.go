package web

import (
	"net/http"
	"time"

	"github.com/o-guardiao/deposito/internal/app"
	"github.com/o-guardiao/deposito/internal/domain"
)

type patientListData struct {
	Query    string
	Patients []domain.Patient
	Now      time.Time
}

type patientFormData struct {
	Patient domain.Patient
	Errors  map[string]string
	IsEdit  bool
}

type patientRecordData struct {
	app.PatientRecord
	Now         time.Time
	EvoErrors   map[string]string
	EvoText     string
	EvoDate     time.Time
	CurrentPath string
}

func (s *Server) handlePatientList(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	items, err := s.svc().Patients.Search(r.Context(), q)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.render(w, r, http.StatusOK, "patient_list", "patient-list", patientListData{Query: q, Patients: items, Now: s.now()})
}

func (s *Server) handlePatientNew(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, http.StatusOK, "patient_form", "", patientFormData{})
}

func (s *Server) patientInput(r *http.Request) app.PatientInput {
	return app.PatientInput{
		Name:      r.PostForm.Get("nome"),
		Phone:     r.PostForm.Get("telefone"),
		Address:   r.PostForm.Get("endereco"),
		BirthDate: s.parseDate(r.PostForm.Get("nascimento")),
		Diagnosis: r.PostForm.Get("diagnostico"),
		Notes:     r.PostForm.Get("observacoes"),
	}
}

// invalidPatientForm re-renders the form with what the user typed.
func (s *Server) invalidPatientForm(w http.ResponseWriter, r *http.Request, id string, in app.PatientInput, err error) {
	p := domain.Patient{ID: id}
	in.Apply(&p, s.now())
	s.render(w, r, http.StatusUnprocessableEntity, "patient_form", "", patientFormData{Patient: p, Errors: fieldErrors(err), IsEdit: id != ""})
}

func (s *Server) handlePatientCreate(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "formulário inválido", http.StatusBadRequest)
		return
	}
	in := s.patientInput(r)
	p, err := s.svc().Patients.Create(r.Context(), in)
	if err != nil {
		if _, ok := domain.AsValidation(err); !ok {
			s.fail(w, r, err)
			return
		}
		s.invalidPatientForm(w, r, "", in, err)
		return
	}
	s.redirectWithFlash(w, r, "/pacientes/"+p.ID, "Paciente cadastrado")
}

// recordData assembles the ficha view model for a patient.
func (s *Server) recordData(r *http.Request, id string) (patientRecordData, error) {
	rec, err := s.svc().Patients.GetRecord(r.Context(), id)
	if err != nil {
		return patientRecordData{}, err
	}
	now := s.now()
	return patientRecordData{PatientRecord: rec, Now: now, EvoDate: now, CurrentPath: "/pacientes/" + id}, nil
}

func (s *Server) handlePatientRecord(w http.ResponseWriter, r *http.Request) {
	data, err := s.recordData(r, r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.render(w, r, http.StatusOK, "patient_record", "", data)
}

func (s *Server) handlePatientEdit(w http.ResponseWriter, r *http.Request) {
	p, err := s.svc().Patients.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.render(w, r, http.StatusOK, "patient_form", "", patientFormData{Patient: p, IsEdit: true})
}

func (s *Server) handlePatientUpdate(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "formulário inválido", http.StatusBadRequest)
		return
	}
	id := r.PathValue("id")
	in := s.patientInput(r)
	p, err := s.svc().Patients.Update(r.Context(), id, in)
	if err != nil {
		if _, ok := domain.AsValidation(err); !ok {
			s.fail(w, r, err)
			return
		}
		s.invalidPatientForm(w, r, id, in, err)
		return
	}
	s.redirectWithFlash(w, r, "/pacientes/"+p.ID, "Ficha atualizada")
}

func (s *Server) handleEvolutionCreate(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "formulário inválido", http.StatusBadRequest)
		return
	}
	id := r.PathValue("id")
	in := app.EvolutionInput{PatientID: id, Date: s.parseDate(r.PostForm.Get("data")), Text: r.PostForm.Get("texto")}
	_, err := s.svc().Evolutions.Add(r.Context(), in)
	if err != nil {
		if _, ok := domain.AsValidation(err); !ok {
			s.fail(w, r, err)
			return
		}
		data, rerr := s.recordData(r, id)
		if rerr != nil {
			s.fail(w, r, rerr)
			return
		}
		data.EvoErrors, data.EvoText, data.EvoDate = fieldErrors(err), in.Text, in.Date
		if isHX(r) {
			w.Header().Set("HX-Retarget", "#evolutions")
			w.Header().Set("HX-Reswap", "outerHTML")
		}
		s.render(w, r, http.StatusUnprocessableEntity, "patient_record", "evolutions", data)
		return
	}
	if !isHX(r) {
		s.redirectWithFlash(w, r, "/pacientes/"+id, "Evolução registrada")
		return
	}
	data, err := s.recordData(r, id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.notify(w, "Evolução registrada")
	s.render(w, r, http.StatusOK, "patient_record", "evolutions", data)
}
