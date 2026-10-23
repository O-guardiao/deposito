package web

import (
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/o-guardiao/deposito/internal/app"
	"github.com/o-guardiao/deposito/internal/domain"
)

const flashCookie = "fisio_flash"

func isHX(r *http.Request) bool { return r.Header.Get("HX-Request") == "true" }

// render writes a full page, or just the partial when the request came from htmx.
func (s *Server) render(w http.ResponseWriter, r *http.Request, status int, page, partial string, data any) {
	var err error
	if partial != "" && isHX(r) {
		err = s.views.Partial(w, status, partial, data)
	} else {
		lay := Layout{Title: pageTitle(page), Section: sectionOf(page), Demo: s.opts.Demo, Data: data}
		lay.Flash = s.popFlash(w, r)
		err = s.views.Page(w, status, page, lay)
	}
	if err != nil {
		s.log.Error("render", "page", page, "partial", partial, "err", err)
		http.Error(w, "erro ao montar a página", http.StatusInternalServerError)
	}
}

func pageTitle(page string) string {
	switch {
	case strings.HasPrefix(page, "agenda"):
		return "Agenda"
	case strings.HasPrefix(page, "patient"):
		return "Pacientes"
	case page == "connect":
		return "Conectar"
	default:
		return "Fisio"
	}
}

func sectionOf(page string) string {
	if strings.HasPrefix(page, "patient") {
		return "pacientes"
	}
	return "agenda"
}

// toast asks the browser to show a message after an htmx swap.
func toast(w http.ResponseWriter, msg string) {
	w.Header().Set("HX-Trigger", `{"toast":`+jsonString(msg)+`,"closeModal":true}`)
}

func jsonString(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`)
	return `"` + r.Replace(s) + `"`
}

// redirectWithFlash sends the browser to a new page and queues a toast.
func (s *Server) redirectWithFlash(w http.ResponseWriter, r *http.Request, to, msg string) {
	if msg != "" {
		http.SetCookie(w, &http.Cookie{Name: flashCookie, Value: url.QueryEscape(msg), Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode, MaxAge: 60})
	}
	if isHX(r) {
		w.Header().Set("HX-Redirect", to)
		w.WriteHeader(http.StatusOK)
		return
	}
	http.Redirect(w, r, to, http.StatusSeeOther)
}

func (s *Server) popFlash(w http.ResponseWriter, r *http.Request) string {
	c, err := r.Cookie(flashCookie)
	if err != nil || c.Value == "" {
		return ""
	}
	http.SetCookie(w, &http.Cookie{Name: flashCookie, Value: "", Path: "/", MaxAge: -1})
	v, _ := url.QueryUnescape(c.Value)
	return v
}

// fail maps domain errors to status codes and renders the error page.
func (s *Server) fail(w http.ResponseWriter, r *http.Request, err error) {
	status := http.StatusInternalServerError
	msg := "Algo deu errado. Tente novamente."
	switch {
	case domain.IsNotFound(err):
		status, msg = http.StatusNotFound, "Não encontramos o que você procurava."
	case errors.Is(err, app.ErrMirror):
		status, msg = http.StatusBadGateway, err.Error()
	}
	if status >= 500 {
		s.log.Error("request failed", "path", r.URL.Path, "err", err)
	}
	s.render(w, r, status, "error", "error", map[string]any{"Message": msg, "Status": status})
}

// Form helpers -------------------------------------------------------------

func (s *Server) parseDateParam(r *http.Request, name string) time.Time {
	now := time.Now().In(s.opts.Location)
	v := r.URL.Query().Get(name)
	if v == "" {
		return now
	}
	t, err := time.ParseInLocation("2006-01-02", v, s.opts.Location)
	if err != nil {
		return now
	}
	return t
}

func (s *Server) parseLocalDateTime(v string) time.Time {
	for _, layout := range []string{"2006-01-02T15:04", "2006-01-02T15:04:05", "2006-01-02 15:04"} {
		if t, err := time.ParseInLocation(layout, strings.TrimSpace(v), s.opts.Location); err == nil {
			return t
		}
	}
	return time.Time{}
}

func (s *Server) parseDate(v string) time.Time {
	for _, layout := range []string{"2006-01-02", "02/01/2006"} {
		if t, err := time.ParseInLocation(layout, strings.TrimSpace(v), s.opts.Location); err == nil {
			return t
		}
	}
	return time.Time{}
}

func parseMinutes(v string, def time.Duration) time.Duration {
	v = strings.TrimSpace(v)
	if v == "" {
		return def
	}
	var n int
	for _, c := range v {
		if c < '0' || c > '9' {
			return def
		}
		n = n*10 + int(c-'0')
	}
	if n <= 0 || n > 24*60 {
		return def
	}
	return time.Duration(n) * time.Minute
}

// fieldErrors extracts per-field messages or a general one.
func fieldErrors(err error) map[string]string {
	if ve, ok := domain.AsValidation(err); ok {
		return ve.Fields
	}
	return map[string]string{"_": err.Error()}
}
