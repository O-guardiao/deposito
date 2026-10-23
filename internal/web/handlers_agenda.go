package web

import (
	"errors"
	"net/http"
	"time"

	"github.com/o-guardiao/deposito/internal/app"
	"github.com/o-guardiao/deposito/internal/domain"
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
	Days []app.DayView
	Now  time.Time
}

// appointmentFormData feeds the create/edit form.
type appointmentFormData struct {
	Appointment domain.Appointment
	Patients    []domain.Patient
	Minutes     int
	Errors      map[string]string
	Return      string // where to go after saving, when opened from a patient record
	IsEdit      bool
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
	return agendaDayData{Day: day, Now: time.Now(), Patients: patients, Notice: notice}, nil
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
		// Default to the Monday of the current week.
		offset := (int(start.Weekday()) + 6) % 7
		start = start.AddDate(0, 0, -offset)
	}
	days, err := s.svc().Agenda.Week(r.Context(), start)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.render(w, r, http.StatusOK, "agenda_week", "agenda-week", agendaWeekData{Days: days, Now: time.Now()})
}

func (s *Server) handleAppointmentNew(w http.ResponseWriter, r *http.Request) {
	patients, err := s.svc().Patients.List(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	date := s.parseDateParam(r, "data")
	start := time.Date(date.Year(), date.Month(), date.Day(), 9, 0, 0, 0, s.opts.Location)
	if date.Hour() != 0 { // "today" default: next full hour
		n := time.Now().In(s.opts.Location)
		start = time.Date(n.Year(), n.Month(), n.Day(), n.Hour()+1, 0, 0, 0, s.opts.Location)
	}
	data := appointmentFormData{
		Appointment: domain.Appointment{PatientID: r.URL.Query().Get("paciente"), Start: start},
		Patients:    patients, Minutes: 60, Return: r.URL.Query().Get("voltar"),
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
		Duration:  parseMinutes(r.PostForm.Get("duracao"), time.Hour),
		Notes:     r.PostForm.Get("observacoes"),
	}
	a, err := s.svc().Agenda.Schedule(r.Context(), in)
	if err != nil && !errors.Is(err, app.ErrMirror) {
		if _, ok := domain.AsValidation(err); !ok {
			s.fail(w, r, err)
			return
		}
		patients, _ := s.svc().Patients.List(r.Context())
		data := appointmentFormData{
			Appointment: domain.Appointment{PatientID: in.PatientID, Start: in.Start, Notes: in.Notes},
			Patients:    patients, Minutes: int(in.Duration.Minutes()), Errors: fieldErrors(err),
			Return: r.PostForm.Get("voltar"),
		}
		s.renderForm(w, r, "appointment_form", "appointment-form", data)
		return
	}
	s.afterAppointmentWrite(w, r, a, err, "Agendado para "+a.Start.In(s.opts.Location).Format("02/01 às 15:04"), r.PostForm.Get("voltar"))
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
		mins = 60
	}
	data := appointmentFormData{Appointment: a, Patients: patients, Minutes: mins, IsEdit: true, Return: r.URL.Query().Get("voltar")}
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
	a, err := s.svc().Agenda.Reschedule(r.Context(), id, in)
	if err != nil && !errors.Is(err, app.ErrMirror) {
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
		data := appointmentFormData{Appointment: cur, Patients: patients, Minutes: int(in.Duration.Minutes()), Errors: fieldErrors(err), IsEdit: true, Return: r.PostForm.Get("voltar")}
		s.renderForm(w, r, "appointment_form", "appointment-form", data)
		return
	}
	s.afterAppointmentWrite(w, r, a, err, "Remarcado para "+a.Start.In(s.opts.Location).Format("02/01 às 15:04"), r.PostForm.Get("voltar"))
}

func (s *Server) handleAppointmentCancel(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	a, err := s.svc().Agenda.Get(r.Context(), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	err = s.svc().Agenda.Cancel(r.Context(), id)
	if err != nil && !errors.Is(err, app.ErrMirror) {
		s.fail(w, r, err)
		return
	}
	s.afterAppointmentWrite(w, r, a, err, "Atendimento cancelado", r.FormValue("voltar"))
}

func (s *Server) handleAppointmentDone(w http.ResponseWriter, r *http.Request) {
	a, err := s.svc().Agenda.MarkDone(r.Context(), r.PathValue("id"))
	if err != nil && !errors.Is(err, app.ErrMirror) {
		s.fail(w, r, err)
		return
	}
	s.afterAppointmentWrite(w, r, a, err, "Atendimento concluído", r.FormValue("voltar"))
}

func (s *Server) handleAppointmentLink(w http.ResponseWriter, r *http.Request) {
	a, err := s.svc().Agenda.Link(r.Context(), r.PathValue("id"), r.FormValue("paciente"))
	if err != nil && !errors.Is(err, app.ErrMirror) {
		s.fail(w, r, err)
		return
	}
	s.afterAppointmentWrite(w, r, a, err, "Vinculado a "+a.Title, r.FormValue("voltar"))
}

// afterAppointmentWrite responds to a successful (or mirror-warned) write:
// htmx callers get the refreshed day view, others are redirected to it.
func (s *Server) afterAppointmentWrite(w http.ResponseWriter, r *http.Request, a domain.Appointment, mirrorErr error, msg, back string) {
	notice := ""
	if mirrorErr != nil {
		notice = mirrorErr.Error()
		s.log.Warn("mirror", "err", mirrorErr)
	}
	target := "/agenda?data=" + a.Start.In(s.opts.Location).Format("2006-01-02")
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
	data, err := s.dayData(r, a.Start, notice)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	toast(w, msg)
	w.Header().Set("HX-Retarget", "#agenda-main")
	w.Header().Set("HX-Reswap", "outerHTML")
	s.render(w, r, http.StatusOK, "agenda_day", "agenda-day", data)
}

// renderForm re-renders a form with validation errors (422). For htmx the
// swap is redirected into the modal body so the error replaces the form.
func (s *Server) renderForm(w http.ResponseWriter, r *http.Request, page, partial string, data any) {
	if isHX(r) {
		w.Header().Set("HX-Retarget", "#modal-body")
		w.Header().Set("HX-Reswap", "innerHTML")
	}
	s.render(w, r, http.StatusUnprocessableEntity, page, partial, data)
}

func (s *Server) handleSync(w http.ResponseWriter, r *http.Request) {
	n, err := s.svc().Sync.Reconcile(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	msg := "Planilha sincronizada: " + itoa(n) + " atendimento(s)"
	if isHX(r) {
		toast(w, msg)
		w.WriteHeader(http.StatusNoContent)
		return
	}
	s.redirectWithFlash(w, r, "/agenda", msg)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}
