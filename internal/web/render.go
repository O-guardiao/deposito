package web

import (
	"bytes"
	"embed"
	"fmt"
	"html/template"
	"io/fs"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/o-guardiao/deposito/internal/domain"
)

//go:embed templates/*.html templates/partials/*.html templates/pages/*.html
var templateFS embed.FS

//go:embed static
var staticFiles embed.FS

func staticFS() fs.FS {
	sub, _ := fs.Sub(staticFiles, "static")
	return sub
}

// Renderer executes page and partial templates with pt-BR helpers.
type Renderer struct {
	pages    map[string]*template.Template
	partials *template.Template
	loc      *time.Location
}

// NewRenderer parses every template under templates/.
func NewRenderer(loc *time.Location) (*Renderer, error) {
	funcs := templateFuncs(loc)
	r := &Renderer{pages: map[string]*template.Template{}, loc: loc}
	base, err := template.New("").Funcs(funcs).ParseFS(templateFS, "templates/layout.html", "templates/partials/*.html")
	if err != nil {
		return nil, fmt.Errorf("templates base: %w", err)
	}
	r.partials = base
	pages, err := fs.Glob(templateFS, "templates/pages/*.html")
	if err != nil {
		return nil, err
	}
	for _, p := range pages {
		name := strings.TrimSuffix(path.Base(p), ".html")
		t, err := template.Must(base.Clone()).ParseFS(templateFS, p)
		if err != nil {
			return nil, fmt.Errorf("template %s: %w", name, err)
		}
		r.pages[name] = t
	}
	return r, nil
}

// Layout is the data every full page receives.
type Layout struct {
	Title   string
	Section string // "agenda" | "pacientes"
	Demo    bool
	Flash   string
	Data    any
}

// Page renders a full page inside the layout.
func (r *Renderer) Page(w http.ResponseWriter, status int, name string, data Layout) error {
	t, ok := r.pages[name]
	if !ok {
		return fmt.Errorf("página %q não existe", name)
	}
	return r.exec(w, status, t, "layout", data)
}

// Partial renders a named fragment.
func (r *Renderer) Partial(w http.ResponseWriter, status int, name string, data any) error {
	return r.exec(w, status, r.partials, name, data)
}

func (r *Renderer) exec(w http.ResponseWriter, status int, t *template.Template, name string, data any) error {
	var buf bytes.Buffer
	if err := t.ExecuteTemplate(&buf, name, data); err != nil {
		return err
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_, err := buf.WriteTo(w)
	return err
}

var (
	weekdaysPT = [...]string{"domingo", "segunda", "terça", "quarta", "quinta", "sexta", "sábado"}
	monthsPT   = [...]string{"", "janeiro", "fevereiro", "março", "abril", "maio", "junho", "julho", "agosto", "setembro", "outubro", "novembro", "dezembro"}
)

func templateFuncs(loc *time.Location) template.FuncMap {
	in := func(t time.Time) time.Time { return t.In(loc) }
	return template.FuncMap{
		"isoDate": func(t time.Time) string {
			if t.IsZero() {
				return ""
			}
			return in(t).Format("2006-01-02")
		},
		"isoLocal": func(t time.Time) string {
			if t.IsZero() {
				return ""
			}
			return in(t).Format("2006-01-02T15:04")
		},
		"fmtTime": func(t time.Time) string { return in(t).Format("15:04") },
		"fmtDate": func(t time.Time) string { return in(t).Format("02/01/2006") },
		"fmtDateShort": func(t time.Time) string {
			t = in(t)
			return fmt.Sprintf("%02d/%02d", t.Day(), int(t.Month()))
		},
		"weekday":      func(t time.Time) string { return weekdaysPT[in(t).Weekday()] },
		"weekdayShort": func(t time.Time) string { return strings.ToUpper(weekdaysPT[in(t).Weekday()][:3]) },
		"longDate": func(t time.Time) string {
			t = in(t)
			return fmt.Sprintf("%s, %d de %s de %d", weekdaysPT[t.Weekday()], t.Day(), monthsPT[t.Month()], t.Year())
		},
		"monthYear": func(t time.Time) string {
			t = in(t)
			return fmt.Sprintf("%s de %d", monthsPT[t.Month()], t.Year())
		},
		"addDays": func(t time.Time, n int) time.Time { return t.AddDate(0, 0, n) },
		"isToday": func(t time.Time, now time.Time) bool {
			return in(t).Format("2006-01-02") == in(now).Format("2006-01-02")
		},
		"fmtPhone": formatPhone,
		"initial": func(s string) string {
			for _, r := range strings.TrimSpace(s) {
				return strings.ToUpper(string(r))
			}
			return "?"
		},
		"durationOptions": func() []int { return []int{30, 45, 60, 90, 120} },
		"minutes":         func(d time.Duration) int { return int(d.Minutes()) },
		"statusLabel": func(st domain.AppointmentStatus) string {
			switch st {
			case domain.StatusDone:
				return "Concluído"
			case domain.StatusCancelled:
				return "Cancelado"
			default:
				return "Agendado"
			}
		},
		"age": func(p domain.Patient, now time.Time) int { return p.Age(now) },
		"dict": func(kv ...any) map[string]any {
			m := make(map[string]any, len(kv)/2)
			for i := 0; i+1 < len(kv); i += 2 {
				m[fmt.Sprint(kv[i])] = kv[i+1]
			}
			return m
		},
		"waLink": func(phone string) string {
			digits := onlyDigits(phone)
			if digits == "" {
				return ""
			}
			if !strings.HasPrefix(digits, "55") && (len(digits) == 10 || len(digits) == 11) {
				digits = "55" + digits
			}
			return "https://wa.me/" + digits
		},
	}
}

func onlyDigits(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// formatPhone renders Brazilian numbers as (11) 98765-4321; others unchanged.
func formatPhone(s string) string {
	d := onlyDigits(s)
	switch len(d) {
	case 11:
		return fmt.Sprintf("(%s) %s-%s", d[:2], d[2:7], d[7:])
	case 10:
		return fmt.Sprintf("(%s) %s-%s", d[:2], d[2:6], d[6:])
	default:
		return s
	}
}
