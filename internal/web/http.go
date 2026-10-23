package web

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/o-guardiao/deposito/internal/domain"
)

const flashCookie = "fisio_flash"

func isHX(r *http.Request) bool { return r.Header.Get("HX-Request") == "true" }

// pageMeta is the title and navigation section of each full page.
type pageMeta struct{ Title, Section string }

var pages = map[string]pageMeta{
	"agenda_day":       {"Agenda", "agenda"},
	"agenda_week":      {"Agenda", "agenda"},
	"appointment_form": {"Agenda", "agenda"},
	"patient_list":     {"Pacientes", "pacientes"},
	"patient_form":     {"Pacientes", "pacientes"},
	"patient_record":   {"Pacientes", "pacientes"},
	"connect":          {"Conectar", "agenda"},
	"error":            {"Fisio", "agenda"},
	"help":             {"Ajuda", "ajuda"},
}

// render writes a full page, or just the partial when the request came from htmx.
func (s *Server) render(w http.ResponseWriter, r *http.Request, status int, page, partial string, data any) {
	var err error
	if partial != "" && isHX(r) {
		err = s.views.Partial(w, status, partial, data)
	} else {
		meta, ok := pages[page]
		if !ok {
			s.log.Error("render", "page", page, "err", "página sem metadados")
			http.Error(w, "erro ao montar a página", http.StatusInternalServerError)
			return
		}
		lay := Layout{Title: meta.Title, Section: meta.Section, Demo: s.opts.Demo, Data: data}
		lay.Flash = s.popFlash(w, r)
		err = s.views.Page(w, status, page, lay)
	}
	if err != nil {
		s.log.Error("render", "page", page, "partial", partial, "err", err)
		http.Error(w, "erro ao montar a página", http.StatusInternalServerError)
	}
}

// notify asks the browser to show a toast after the swap. The modal, if
// open, is closed only after the swap (HX-Trigger-After-Swap), because
// closing it earlier would detach the form htmx is about to swap from.
func (s *Server) notify(w http.ResponseWriter, msg string) {
	w.Header().Set("HX-Trigger", asciiJSON(map[string]string{"toast": msg}))
	w.Header().Set("HX-Trigger-After-Swap", `{"closeModal":true}`)
}

// asciiJSON encodes v as JSON with every non-ASCII rune escaped, because HTTP
// headers are read as Latin-1 by browsers and would garble UTF-8.
func asciiJSON(v any) string {
	raw, err := json.Marshal(v)
	if err != nil {
		return "{}"
	}
	var b strings.Builder
	for len(raw) > 0 {
		r, size := utf8.DecodeRune(raw)
		if r < utf8.RuneSelf {
			b.WriteByte(raw[0])
		} else {
			for _, u := range utf16Encode(r) {
				fmt.Fprintf(&b, `\u%04x`, u)
			}
		}
		raw = raw[size:]
	}
	return b.String()
}

func utf16Encode(r rune) []uint16 {
	if r < 0x10000 {
		return []uint16{uint16(r)}
	}
	r -= 0x10000
	return []uint16{uint16(0xD800 + (r >> 10)), uint16(0xDC00 + (r & 0x3FF))}
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

// fail maps errors to status codes and renders the error page. When Google
// says the saved credentials are no longer valid, the services are dropped so
// the connect screen comes back instead of an endless 500.
func (s *Server) fail(w http.ResponseWriter, r *http.Request, err error) {
	status := http.StatusInternalServerError
	msg := "Algo deu errado. Tente novamente."
	switch {
	case domain.IsNotFound(err):
		status, msg = http.StatusNotFound, "Não encontramos o que você procurava."
	case s.opts.Login != nil && s.opts.Login.NeedsReconnect(err):
		s.log.Warn("credenciais recusadas; pedindo novo login", "err", err)
		s.opts.Login.Forget()
		s.services.Store(nil)
		s.redirectWithFlash(w, r, "/", "O Google pediu um novo login")
		return
	}
	if status >= 500 {
		s.log.Error("request failed", "path", r.URL.Path, "err", err)
	}
	s.render(w, r, status, "error", "error", map[string]any{"Message": msg, "Status": status})
}

// Form helpers -------------------------------------------------------------

// localPath accepts only same-site paths for "voltar" redirects.
func localPath(v string) string {
	if strings.HasPrefix(v, "/") && !strings.HasPrefix(v, "//") && !strings.ContainsAny(v, "\\\r\n") {
		return v
	}
	return ""
}

func (s *Server) now() time.Time { return s.clock.Now().In(s.opts.Location) }

func (s *Server) parseDateParam(r *http.Request, name string) time.Time {
	v := r.URL.Query().Get(name)
	if v == "" {
		return s.now()
	}
	t, err := time.ParseInLocation("2006-01-02", v, s.opts.Location)
	if err != nil {
		return s.now()
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
	n, err := strconv.Atoi(strings.TrimSpace(v))
	if err != nil || n <= 0 || n > 24*60 {
		return def
	}
	return time.Duration(n) * time.Minute
}

// mondayOf returns the Monday 00:00 of the week containing t.
func mondayOf(t time.Time, loc *time.Location) time.Time {
	d := t.In(loc)
	offset := (int(d.Weekday()) + 6) % 7
	return domain.DayStart(d.Year(), d.Month(), d.Day()-offset, loc)
}

// fieldErrors extracts per-field messages from a validation error.
func fieldErrors(err error) map[string]string {
	if ve, ok := domain.AsValidation(err); ok {
		return ve.Fields
	}
	return nil
}
