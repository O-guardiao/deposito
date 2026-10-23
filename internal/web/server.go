// Package web is the HTTP/HTMX delivery layer.
package web

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/o-guardiao/deposito/internal/app"
)

// Services groups the use cases the handlers need. In Google mode it only
// exists after the user connects her account.
type Services struct {
	Patients   *app.PatientService
	Evolutions *app.EvolutionService
	Agenda     *app.AgendaService
	Sync       *app.SyncService
}

// GoogleLogin is the part of the OAuth flow the web layer drives.
type GoogleLogin interface {
	Connected() bool
	LoginURL(state string) string
	Exchange(ctx context.Context, code string) error
}

// ServicesFactory builds the use cases once a Google account is connected.
type ServicesFactory func(ctx context.Context) (*Services, error)

// Options configure the server.
type Options struct {
	Location *time.Location
	Demo     bool
	// Login and Factory are nil in demo mode.
	Login   GoogleLogin
	Factory ServicesFactory
	Logger  *slog.Logger
}

// Server routes requests to the use cases and renders HTML.
type Server struct {
	opts     Options
	views    *Renderer
	services atomic.Pointer[Services]
	log      *slog.Logger
}

// New creates a server. In demo mode svc must be provided; in Google mode it
// may be nil and will be built through opts.Factory after login.
func New(svc *Services, opts Options) (*Server, error) {
	if opts.Location == nil {
		opts.Location = time.UTC
	}
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}
	views, err := NewRenderer(opts.Location)
	if err != nil {
		return nil, err
	}
	s := &Server{opts: opts, views: views, log: opts.Logger}
	if svc != nil {
		s.services.Store(svc)
	}
	return s, nil
}

// Connected reports whether the use cases are available.
func (s *Server) Connected() bool { return s.services.Load() != nil }

// Handler returns the routed http.Handler.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("GET /static/", http.StripPrefix("/static/", cacheControl(http.FileServerFS(staticFS()))))
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("ok")) })
	mux.HandleFunc("GET /auth/login", s.handleLogin)
	mux.HandleFunc("GET /auth/callback", s.handleCallback)

	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/agenda", http.StatusSeeOther)
	})
	mux.HandleFunc("GET /agenda", s.handleAgendaDay)
	mux.HandleFunc("GET /agenda/semana", s.handleAgendaWeek)
	mux.HandleFunc("GET /agenda/novo", s.handleAppointmentNew)
	mux.HandleFunc("POST /agenda", s.handleAppointmentCreate)
	mux.HandleFunc("GET /agenda/{id}/editar", s.handleAppointmentEdit)
	mux.HandleFunc("POST /agenda/{id}", s.handleAppointmentUpdate)
	mux.HandleFunc("POST /agenda/{id}/cancelar", s.handleAppointmentCancel)
	mux.HandleFunc("POST /agenda/{id}/concluir", s.handleAppointmentDone)
	mux.HandleFunc("POST /agenda/{id}/vincular", s.handleAppointmentLink)
	mux.HandleFunc("POST /sincronizar", s.handleSync)

	mux.HandleFunc("GET /pacientes", s.handlePatientList)
	mux.HandleFunc("GET /pacientes/novo", s.handlePatientNew)
	mux.HandleFunc("POST /pacientes", s.handlePatientCreate)
	mux.HandleFunc("GET /pacientes/{id}", s.handlePatientRecord)
	mux.HandleFunc("GET /pacientes/{id}/editar", s.handlePatientEdit)
	mux.HandleFunc("POST /pacientes/{id}", s.handlePatientUpdate)
	mux.HandleFunc("POST /pacientes/{id}/evolucoes", s.handleEvolutionCreate)

	return s.requireConnection(s.logRequests(mux))
}

// requireConnection shows the "connect Google" page until the services exist.
func (s *Server) requireConnection(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Path
		open := p == "/healthz" || strings.HasPrefix(p, "/static/") || strings.HasPrefix(p, "/auth/")
		if !open && !s.Connected() {
			s.render(w, r, http.StatusOK, "connect", "", nil)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		s.log.Info("http", "method", r.Method, "path", r.URL.Path, "status", rec.status, "ms", time.Since(start).Milliseconds())
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

func cacheControl(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "public, max-age=86400")
		next.ServeHTTP(w, r)
	})
}

// svc returns the current services; handlers only run once connected.
func (s *Server) svc() *Services { return s.services.Load() }
