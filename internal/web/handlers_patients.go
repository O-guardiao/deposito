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
	s.render(w, r, http.StatusOK, "patient_list", "patient-list", patientListData{Query: q, Patients: items, Now: time.Now()})
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

func patientFromInput(id string, in app.PatientInput) domain.Patient {
	return domain.Patient{ID: id, Name: in.Name, Phone: in.Phone, Address: in.Address, BirthDate: in.BirthDate, Diagnosis: in.Diagnosis, Notes: in.Notes}
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
		s.render(w, r, http.StatusUnprocessableEntity, "patient_form", "", patientFormData{Patient: patientFromInput("", in), Errors: fieldErrors(err)})
		return
	}
	s.redirectWithFlash(w, r, "/pacientes/"+p.ID, "Paciente cadastrado")
}

func (s *Server) handlePatientRecord(w http.ResponseWriter, r *http.Request) {
	rec, err := s.svc().Patients.Record(r.Context(), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.render(w, r, http.StatusOK, "patient_record", "", patientRecordData{PatientRecord: rec, Now: time.Now(), EvoDate: time.Now(), CurrentPath: r.URL.Path})
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
		s.render(w, r, http.StatusUnprocessableEntity, "patient_form", "", patientFormData{Patient: patientFromInput(id, in), Errors: fieldErrors(err), IsEdit: true})
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
	date := s.parseDate(r.PostForm.Get("data"))
	text := r.PostForm.Get("texto")
	_, err := s.svc().Evolutions.Add(r.Context(), id, date, text)
	if err != nil {
		if _, ok := domain.AsValidation(err); !ok {
			s.fail(w, r, err)
			return
		}
		rec, rerr := s.svc().Patients.Record(r.Context(), id)
		if rerr != nil {
			s.fail(w, r, rerr)
			return
		}
		data := patientRecordData{PatientRecord: rec, Now: time.Now(), EvoErrors: fieldErrors(err), EvoText: text, EvoDate: date, CurrentPath: r.URL.Path}
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
	rec, err := s.svc().Patients.Record(r.Context(), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	toast(w, "Evolução registrada")
	s.render(w, r, http.StatusOK, "patient_record", "evolutions", patientRecordData{PatientRecord: rec, Now: time.Now(), EvoDate: time.Now(), CurrentPath: "/pacientes/" + id})
}
