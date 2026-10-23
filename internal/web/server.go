// Package web is the HTTP/HTMX delivery layer.
package web

import (
	"context"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"time"

	"github.com/o-guardiao/deposito/internal/app"
	"github.com/o-guardiao/deposito/internal/domain"
)

// GoogleLogin is the part of the OAuth flow the web layer drives.
type GoogleLogin interface {
	Connected() bool
	LoginURL(state string) string
	Exchange(ctx context.Context, code string) error
	// NeedsReconnect tells whether an error from the backend means the saved
	// credentials were revoked and the user has to log in again.
	NeedsReconnect(err error) bool
	// Forget drops the saved credentials.
	Forget()
}

// ServicesFactory builds the use cases once a Google account is connected.
// ctx bounds only the setup calls (spreadsheet bootstrap); the services it
// returns must keep working after ctx is done.
type ServicesFactory func(ctx context.Context) (*app.Services, error)

// Options configure the server.
type Options struct {
	Location *time.Location
	Clock    domain.Clock
	Demo     bool
	// Login and Factory are nil in demo mode.
	Login   GoogleLogin
	Factory ServicesFactory
	Logger  *slog.Logger
	// MaxFormBytes caps the size of a POST body (default 64 KiB).
	MaxFormBytes int64
}

// Server routes requests to the use cases and renders HTML.
type Server struct {
	opts     Options
	views    *Renderer
	services atomic.Pointer[app.Services]
	log      *slog.Logger
	clock    domain.Clock
}

// New creates a server. In demo mode svc must be provided; in Google mode it
// may be nil and will be built through opts.Factory after login.
func New(svc *app.Services, opts Options) (*Server, error) {
	if opts.Location == nil {
		opts.Location = time.UTC
	}
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}
	if opts.Clock == nil {
		opts.Clock = domain.ClockFunc(time.Now)
	}
	if opts.MaxFormBytes <= 0 {
		opts.MaxFormBytes = 64 << 10
	}
	views, err := NewRenderer(opts.Location)
	if err != nil {
		return nil, err
	}
	s := &Server{opts: opts, views: views, log: opts.Logger, clock: opts.Clock}
	if svc != nil {
		s.services.Store(svc)
	}
	return s, nil
}

// Connected reports whether the use cases are available.
func (s *Server) Connected() bool { return s.services.Load() != nil }

// Handler returns the routed http.Handler. Routes that work before login are
// registered on the outer mux; everything else sits behind requireConnection.
func (s *Server) Handler() http.Handler {
	appMux := http.NewServeMux()
	appMux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/agenda", http.StatusSeeOther)
	})
	appMux.HandleFunc("GET /agenda", s.handleAgendaDay)
	appMux.HandleFunc("GET /agenda/semana", s.handleAgendaWeek)
	appMux.HandleFunc("GET /agenda/novo", s.handleAppointmentNew)
	appMux.HandleFunc("POST /agenda", s.handleAppointmentCreate)
	appMux.HandleFunc("GET /agenda/{id}/editar", s.handleAppointmentEdit)
	appMux.HandleFunc("POST /agenda/{id}", s.handleAppointmentUpdate)
	appMux.HandleFunc("POST /agenda/{id}/cancelar", s.handleAppointmentCancel)
	appMux.HandleFunc("POST /agenda/{id}/concluir", s.handleAppointmentDone)
	appMux.HandleFunc("POST /agenda/{id}/vincular", s.handleAppointmentLink)
	appMux.HandleFunc("POST /sincronizar", s.handleSync)

	appMux.HandleFunc("GET /pacientes", s.handlePatientList)
	appMux.HandleFunc("GET /pacientes/novo", s.handlePatientNew)
	appMux.HandleFunc("POST /pacientes", s.handlePatientCreate)
	appMux.HandleFunc("GET /pacientes/{id}", s.handlePatientRecord)
	appMux.HandleFunc("GET /pacientes/{id}/editar", s.handlePatientEdit)
	appMux.HandleFunc("POST /pacientes/{id}", s.handlePatientUpdate)
	appMux.HandleFunc("POST /pacientes/{id}/evolucoes", s.handleEvolutionCreate)
	// Unknown GET paths get the styled 404; unknown methods still get 405.
	appMux.HandleFunc("GET /{path...}", func(w http.ResponseWriter, r *http.Request) { s.fail(w, r, domain.ErrNotFound) })

	mux := http.NewServeMux()
	mux.Handle("GET /static/", http.StripPrefix("/static/", staticCache(http.FileServerFS(staticFS()))))
	mux.HandleFunc("GET /ajuda", s.handleHelp) // public: explains the app before connecting
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("ok")) })
	mux.HandleFunc("GET /auth/login", s.handleLogin)
	mux.HandleFunc("GET /auth/callback", s.handleCallback)
	mux.Handle("/", s.requireConnection(appMux))

	return s.logRequests(s.sameOrigin(s.limitBody(mux)))
}

// requireConnection shows the "connect Google" page until the services exist.
// Non-GET requests are redirected so a stale tab never swaps the connect page
// into a fragment.
func (s *Server) requireConnection(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.Connected() {
			next.ServeHTTP(w, r)
			return
		}
		if r.Method != http.MethodGet {
			s.redirectWithFlash(w, r, "/", "Conecte sua conta Google para continuar")
			return
		}
		s.render(w, r, http.StatusOK, "connect", "", nil)
	})
}

// sameOrigin rejects state-changing requests that come from another site
// (CSRF): when the browser says where the request originated, it must be us.
func (s *Server) sameOrigin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead && !sameOriginRequest(r) {
			http.Error(w, "requisição de outra origem recusada", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func sameOriginRequest(r *http.Request) bool {
	if site := r.Header.Get("Sec-Fetch-Site"); site != "" && site != "same-origin" && site != "none" {
		return false
	}
	origin := r.Header.Get("Origin")
	if origin == "" || origin == "null" {
		return origin == "" // "null" origin is never ours
	}
	u, err := url.Parse(origin)
	if err != nil {
		return false
	}
	return strings.EqualFold(u.Host, r.Host)
}

// limitBody caps POST bodies so a stray upload cannot exhaust memory.
func (s *Server) limitBody(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Body != nil && r.Method != http.MethodGet {
			r.Body = http.MaxBytesReader(w, r.Body, s.opts.MaxFormBytes)
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

// staticCache lets versioned asset URLs (?v=<hash>) be cached for a year and
// everything else be revalidated, so upgrades show up immediately.
func staticCache(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("v") != "" {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			w.Header().Set("Cache-Control", "no-cache")
		}
		next.ServeHTTP(w, r)
	})
}

// svc returns the current services; handlers only run once connected.
func (s *Server) svc() *app.Services { return s.services.Load() }
