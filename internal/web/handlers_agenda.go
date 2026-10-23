package web

import (
	"net/http"
	"strconv"
	"time"

	"github.com/o-guardiao/deposito/internal/app"
	"github.com/o-guardiao/deposito/internal/domain"
)

// Views the appointment form can be opened from; the answer to a save is
// rendered for the same view.
const (
	viewDay  = "dia"
	viewWeek = "semana"
)

// agendaDayData feeds the day view (page and partial).
type agendaDayData struct {
	Day      app.DayView
	Now      time.Time
	Patients []domain.Patient // for the inline "vincular" select
	Notice   string           // e.g. mirror warning
}

// agendaWeekData feeds the week view.
type agendaWeekData struct {
	Days   []app.DayView
	Now    time.Time
	Notice string
}

// appointmentFormData feeds the create/edit form.
type appointmentFormData struct {
	Appointment domain.Appointment
	Patients    []domain.Patient
	Minutes     int
	Errors      map[string]string
	Return      string // where to go after saving, when opened from a patient record
	View        string // viewDay or viewWeek: which agenda view to refresh after saving
	IsEdit      bool
	// Modal is true when the form is rendered inside the modal (htmx); the
	// full page version submits as a plain form and must carry no hx-target.
	Modal bool
}

func (s *Server) dayData(r *http.Request, date time.Time, notice string) (agendaDayData, error) {
	day, err := s.svc().Agenda.Day(r.Context(), date)
	if err != nil {
		return agendaDayData{}, err
	}
	patients, err := s.svc().Patients.List(r.Context())
	if err != nil {
		return agendaDayData{}, err
	}
	return agendaDayData{Day: day, Now: s.now(), Patients: patients, Notice: notice}, nil
}

func (s *Server) weekData(r *http.Request, start time.Time, notice string) (agendaWeekData, error) {
	days, err := s.svc().Agenda.Week(r.Context(), start)
	if err != nil {
		return agendaWeekData{}, err
	}
	return agendaWeekData{Days: days, Now: s.now(), Notice: notice}, nil
}

func (s *Server) handleAgendaDay(w http.ResponseWriter, r *http.Request) {
	data, err := s.dayData(r, s.parseDateParam(r, "data"), "")
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.render(w, r, http.StatusOK, "agenda_day", "agenda-day", data)
}

func (s *Server) handleAgendaWeek(w http.ResponseWriter, r *http.Request) {
	start := s.parseDateParam(r, "inicio")
	if r.URL.Query().Get("inicio") == "" {
		start = mondayOf(start, s.opts.Location)
	}
	data, err := s.weekData(r, start, "")
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.render(w, r, http.StatusOK, "agenda_week", "agenda-week", data)
}

func (s *Server) defaultMinutes() int { return int(s.svc().Agenda.DefaultDuration.Minutes()) }

func viewOf(v string) string {
	if v == viewWeek {
		return viewWeek
	}
	return viewDay
}

func (s *Server) handleAppointmentNew(w http.ResponseWriter, r *http.Request) {
	patients, err := s.svc().Patients.List(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	q := r.URL.Query()
	date := s.parseDateParam(r, "data")
	start := time.Date(date.Year(), date.Month(), date.Day(), 9, 0, 0, 0, s.opts.Location)
	if q.Get("data") == "" || domain.SameDay(date, s.now(), s.opts.Location) {
		n := s.now()
		start = time.Date(n.Year(), n.Month(), n.Day(), n.Hour()+1, 0, 0, 0, s.opts.Location)
	}
	data := appointmentFormData{
		Appointment: domain.Appointment{PatientID: q.Get("paciente"), Start: start},
		Patients:    patients, Minutes: s.defaultMinutes(),
		Return: localPath(q.Get("voltar")), View: viewOf(q.Get("vista")), Modal: isHX(r),
	}
	s.render(w, r, http.StatusOK, "appointment_form", "appointment-form", data)
}

func (s *Server) handleAppointmentCreate(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "formulário inválido", http.StatusBadRequest)
		return
	}
	in := app.ScheduleInput{
		PatientID: r.PostForm.Get("paciente"),
		Start:     s.parseLocalDateTime(r.PostForm.Get("inicio")),
		Duration:  parseMinutes(r.PostForm.Get("duracao"), s.svc().Agenda.DefaultDuration),
		Notes:     r.PostForm.Get("observacoes"),
	}
	res, err := s.svc().Agenda.Schedule(r.Context(), in)
	if err != nil {
		if _, ok := domain.AsValidation(err); !ok {
			s.fail(w, r, err)
			return
		}
		patients, _ := s.svc().Patients.List(r.Context())
		data := appointmentFormData{
			Appointment: domain.Appointment{PatientID: in.PatientID, Start: in.Start, Notes: in.Notes},
			Patients:    patients, Minutes: int(in.Duration.Minutes()), Errors: fieldErrors(err),
			Return: localPath(r.PostForm.Get("voltar")), View: viewOf(r.PostForm.Get("vista")), Modal: isHX(r),
		}
		s.renderForm(w, r, data)
		return
	}
	s.afterAppointmentWrite(w, r, res, "Agendado para "+res.Appointment.Start.In(s.opts.Location).Format("02/01 às 15:04"))
}

func (s *Server) handleAppointmentEdit(w http.ResponseWriter, r *http.Request) {
	a, err := s.svc().Agenda.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	patients, err := s.svc().Patients.List(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	mins := int(a.Duration().Minutes())
	if mins <= 0 {
		mins = s.defaultMinutes()
	}
	q := r.URL.Query()
	data := appointmentFormData{Appointment: a, Patients: patients, Minutes: mins, IsEdit: true,
		Return: localPath(q.Get("voltar")), View: viewOf(q.Get("vista")), Modal: isHX(r)}
	s.render(w, r, http.StatusOK, "appointment_form", "appointment-form", data)
}

func (s *Server) handleAppointmentUpdate(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "formulário inválido", http.StatusBadRequest)
		return
	}
	id := r.PathValue("id")
	in := app.RescheduleInput{
		Start:    s.parseLocalDateTime(r.PostForm.Get("inicio")),
		Duration: parseMinutes(r.PostForm.Get("duracao"), 0),
		Notes:    r.PostForm.Get("observacoes"),
	}
	res, err := s.svc().Agenda.Reschedule(r.Context(), id, in)
	if err != nil {
		if _, ok := domain.AsValidation(err); !ok {
			s.fail(w, r, err)
			return
		}
		cur, gerr := s.svc().Agenda.Get(r.Context(), id)
		if gerr != nil {
			s.fail(w, r, gerr)
			return
		}
		cur.Start, cur.Notes = in.Start, in.Notes
		patients, _ := s.svc().Patients.List(r.Context())
		data := appointmentFormData{Appointment: cur, Patients: patients, Minutes: int(in.Duration.Minutes()), Errors: fieldErrors(err), IsEdit: true,
			Return: localPath(r.PostForm.Get("voltar")), View: viewOf(r.PostForm.Get("vista")), Modal: isHX(r)}
		s.renderForm(w, r, data)
		return
	}
	s.afterAppointmentWrite(w, r, res, "Remarcado para "+res.Appointment.Start.In(s.opts.Location).Format("02/01 às 15:04"))
}

func (s *Server) handleAppointmentCancel(w http.ResponseWriter, r *http.Request) {
	res, err := s.svc().Agenda.Cancel(r.Context(), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.afterAppointmentWrite(w, r, res, "Atendimento cancelado")
}

func (s *Server) handleAppointmentDone(w http.ResponseWriter, r *http.Request) {
	res, err := s.svc().Agenda.MarkDone(r.Context(), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.afterAppointmentWrite(w, r, res, "Atendimento concluído")
}

func (s *Server) handleAppointmentLink(w http.ResponseWriter, r *http.Request) {
	res, err := s.svc().Agenda.Link(r.Context(), r.PathValue("id"), r.FormValue("paciente"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.afterAppointmentWrite(w, r, res, "Vinculado a "+res.Appointment.Title)
}

// afterAppointmentWrite answers a successful write. Plain submits and
// "voltar" callers are redirected; htmx callers receive the refreshed agenda
// (day or week, matching the view the form came from) retargeted onto
// #agenda-main, with the toast and the modal close queued after the swap.
func (s *Server) afterAppointmentWrite(w http.ResponseWriter, r *http.Request, res app.WriteResult, msg string) {
	a := res.Appointment
	notice := ""
	if res.MirrorWarning != nil {
		notice = res.MirrorWarning.Error()
		s.log.Warn("mirror", "err", res.MirrorWarning)
	}
	back := localPath(r.FormValue("voltar"))
	view := viewOf(r.FormValue("vista"))
	target := "/agenda?data=" + a.Start.In(s.opts.Location).Format("2006-01-02")
	if view == viewWeek {
		target = "/agenda/semana?inicio=" + mondayOf(a.Start, s.opts.Location).Format("2006-01-02")
	}
	if back != "" {
		target = back
	}
	if !isHX(r) || back != "" {
		if notice != "" {
			msg += " — atenção: " + notice
		}
		s.redirectWithFlash(w, r, target, msg)
		return
	}
	s.notify(w, msg)
	w.Header().Set("HX-Retarget", "#agenda-main")
	w.Header().Set("HX-Reswap", "outerHTML")
	w.Header().Set("HX-Push-Url", target)
	if view == viewWeek {
		data, err := s.weekData(r, mondayOf(a.Start, s.opts.Location), notice)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		s.render(w, r, http.StatusOK, "agenda_week", "agenda-week", data)
		return
	}
	data, err := s.dayData(r, a.Start, notice)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.render(w, r, http.StatusOK, "agenda_day", "agenda-day", data)
}

// renderForm re-renders the appointment form with validation errors (422).
// For htmx the swap goes into the modal body so the error replaces the form.
func (s *Server) renderForm(w http.ResponseWriter, r *http.Request, data appointmentFormData) {
	if isHX(r) {
		w.Header().Set("HX-Retarget", "#modal-body")
		w.Header().Set("HX-Reswap", "innerHTML")
	}
	s.render(w, r, http.StatusUnprocessableEntity, "appointment_form", "appointment-form", data)
}

func (s *Server) handleSync(w http.ResponseWriter, r *http.Request) {
	n, err := s.svc().Sync.Reconcile(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	msg := "Planilha sincronizada: " + strconv.Itoa(n) + " atendimento(s)"
	if isHX(r) {
		s.notify(w, msg)
		w.WriteHeader(http.StatusNoContent)
		return
	}
	s.redirectWithFlash(w, r, "/agenda", msg)
}
