// Command fisio serves the physiotherapist's agenda and patient records.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	_ "time/tzdata"

	"google.golang.org/api/calendar/v3"
	"google.golang.org/api/option"
	"google.golang.org/api/sheets/v4"

	"github.com/o-guardiao/deposito/internal/adapters/gauth"
	"github.com/o-guardiao/deposito/internal/adapters/gcalendar"
	"github.com/o-guardiao/deposito/internal/adapters/gsheets"
	"github.com/o-guardiao/deposito/internal/adapters/ids"
	"github.com/o-guardiao/deposito/internal/adapters/memory"
	"github.com/o-guardiao/deposito/internal/app"
	"github.com/o-guardiao/deposito/internal/domain"
	"github.com/o-guardiao/deposito/internal/web"
)

type config struct {
	addr          string
	demo          bool
	dataDir       string
	credentials   string
	tokenPath     string
	spreadsheetID string
	calendarID    string
	timezone      string
	publicURL     string
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func parseConfig() config {
	var c config
	flag.StringVar(&c.addr, "addr", envOr("FISIO_ADDR", "127.0.0.1:8080"), "endereço HTTP (só a máquina dela; 0.0.0.0:8080 expõe na rede)")
	flag.BoolVar(&c.demo, "demo", os.Getenv("FISIO_DEMO") == "1", "modo demo: dados em memória, sem Google")
	flag.StringVar(&c.dataDir, "data", envOr("FISIO_DATA", "./data"), "pasta para token e configuração")
	flag.StringVar(&c.credentials, "credentials", envOr("FISIO_CREDENTIALS", "./credentials.json"), "credentials.json do OAuth (Google Cloud)")
	flag.StringVar(&c.tokenPath, "token", envOr("FISIO_TOKEN", ""), "arquivo do token OAuth (padrão: <data>/token.json)")
	flag.StringVar(&c.spreadsheetID, "spreadsheet", envOr("FISIO_SPREADSHEET_ID", ""), "ID da planilha (vazio = criar/usar a salva)")
	flag.StringVar(&c.calendarID, "calendar", envOr("FISIO_CALENDAR_ID", "primary"), "ID da agenda do Google")
	flag.StringVar(&c.timezone, "tz", envOr("FISIO_TZ", "America/Sao_Paulo"), "fuso horário")
	flag.StringVar(&c.publicURL, "public-url", envOr("FISIO_PUBLIC_URL", ""), "URL base vista pelo navegador (padrão: http://localhost:<porta>)")
	flag.Parse()
	if c.tokenPath == "" {
		c.tokenPath = filepath.Join(c.dataDir, "token.json")
	}
	if c.publicURL == "" {
		_, port, err := net.SplitHostPort(c.addr)
		if err != nil || port == "" {
			port = "8080"
		}
		c.publicURL = "http://localhost:" + port
	}
	c.publicURL = strings.TrimRight(c.publicURL, "/")
	return c
}

// settings persists IDs discovered at runtime (the spreadsheet created on first run).
type settings struct {
	SpreadsheetID string `json:"spreadsheet_id"`
}

func loadSettings(path string) settings {
	var s settings
	raw, err := os.ReadFile(path)
	if err == nil {
		_ = json.Unmarshal(raw, &s)
	}
	return s
}

func saveSettings(path string, s settings) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	raw, _ := json.MarshalIndent(s, "", "  ")
	return os.WriteFile(path, raw, 0o600)
}

func main() {
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	slog.SetDefault(log)
	cfg := parseConfig()

	loc, err := time.LoadLocation(cfg.timezone)
	if err != nil {
		log.Warn("fuso horário inválido, usando UTC", "tz", cfg.timezone, "err", err)
		loc = time.UTC
	}
	clock := domain.ClockFunc(time.Now)

	var (
		services *app.Services
		opts     = web.Options{Location: loc, Clock: clock, Demo: cfg.demo, Logger: log}
	)
	if cfg.demo {
		store := memory.NewStore()
		store.SeedDemo(clock.Now(), loc)
		services = app.NewServices(app.Deps{Patients: store.Patients, Evolutions: store.Evolutions, Agenda: store.Agenda, Mirror: store.Mirror, IDs: store.IDs, Clock: clock, Location: loc})
		log.Warn("MODO DEMO: dados fictícios em memória; nada é gravado no Google")
	} else {
		auth, err := gauth.New(gauth.Config{CredentialsPath: cfg.credentials, TokenPath: cfg.tokenPath, RedirectURL: cfg.publicURL + "/auth/callback"})
		if err != nil {
			log.Error("não foi possível carregar as credenciais do Google", "err", err,
				"dica", "crie um OAuth client no Google Cloud e salve como credentials.json (veja README)")
			os.Exit(1)
		}
		opts.Login = auth
		opts.Factory = googleFactory(cfg, auth, clock, loc, log)
		if auth.Connected() {
			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			services, err = opts.Factory(ctx)
			cancel()
			if err != nil {
				log.Warn("token salvo, mas a preparação falhou; será pedido novo login", "err", err)
				services = nil
			}
		}
	}

	srv, err := web.New(services, opts)
	if err != nil {
		log.Error("iniciar servidor", "err", err)
		os.Exit(1)
	}
	httpServer := &http.Server{
		Addr: cfg.addr, Handler: srv.Handler(),
		ReadHeaderTimeout: 10 * time.Second, ReadTimeout: 30 * time.Second,
		WriteTimeout: 120 * time.Second, IdleTimeout: 2 * time.Minute,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		log.Info("fisio no ar", "url", cfg.publicURL, "addr", cfg.addr, "demo", cfg.demo, "tz", loc.String())
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("servidor parou", "err", err)
			stop()
		}
	}()
	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = httpServer.Shutdown(shutdownCtx)
	log.Info("até logo")
}

// googleFactory builds the Google-backed services once a token exists,
// creating the spreadsheet on first use and remembering its ID. ctx bounds
// the setup calls only: the HTTP client refreshes tokens under the
// Authenticator's own lifetime context.
func googleFactory(cfg config, auth *gauth.Authenticator, clock domain.Clock, loc *time.Location, log *slog.Logger) web.ServicesFactory {
	settingsPath := filepath.Join(cfg.dataDir, "fisio.json")
	return func(ctx context.Context) (*app.Services, error) {
		client, err := auth.Client()
		if err != nil {
			return nil, err
		}
		sheetsSvc, err := sheets.NewService(ctx, option.WithHTTPClient(client))
		if err != nil {
			return nil, fmt.Errorf("sheets: %w", err)
		}
		calSvc, err := calendar.NewService(ctx, option.WithHTTPClient(client))
		if err != nil {
			return nil, fmt.Errorf("calendar: %w", err)
		}
		sheetsAPI := gsheets.NewServiceAPI(sheetsSvc)

		st := loadSettings(settingsPath)
		id := cfg.spreadsheetID
		if id == "" {
			id = st.SpreadsheetID
		}
		id, err = gsheets.EnsureSpreadsheet(ctx, sheetsAPI, id, loc)
		if err != nil {
			return nil, err
		}
		if id != st.SpreadsheetID {
			if err := saveSettings(settingsPath, settings{SpreadsheetID: id}); err != nil {
				log.Warn("não foi possível salvar fisio.json", "err", err)
			}
			log.Info("planilha pronta", "url", "https://docs.google.com/spreadsheets/d/"+id)
		}
		agenda := gcalendar.NewAgenda(gcalendar.NewServiceAPI(calSvc), cfg.calendarID, loc, gcalendar.DefaultReminders())
		agenda.Logger = log
		return app.NewServices(app.Deps{
			Patients:   gsheets.NewPatientRepo(sheetsAPI, id, loc),
			Evolutions: gsheets.NewEvolutionRepo(sheetsAPI, id, loc),
			Agenda:     agenda,
			Mirror:     gsheets.NewMirror(sheetsAPI, id),
			IDs:        ids.Random{Clock: clock},
			Clock:      clock, Location: loc,
		}), nil
	}
}
