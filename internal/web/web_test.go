package web_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/o-guardiao/deposito/internal/adapters/memory"
	"github.com/o-guardiao/deposito/internal/app"
	"github.com/o-guardiao/deposito/internal/domain"
	"github.com/o-guardiao/deposito/internal/web"
)

var loc, _ = time.LoadLocation("America/Sao_Paulo")

func newServer(t *testing.T) (http.Handler, *memory.Store) {
	t.Helper()
	store := memory.NewStore()
	now := time.Now()
	store.SeedDemo(now, loc)
	clock := domain.ClockFunc(time.Now)
	svc := &web.Services{
		Patients:   app.NewPatientService(store.Patients, store.Evolutions, store.Agenda, store.IDs, clock),
		Evolutions: app.NewEvolutionService(store.Evolutions, store.Patients, store.IDs, clock),
		Agenda:     app.NewAgendaService(store.Agenda, store.Mirror, store.Patients, clock, loc),
		Sync:       app.NewSyncService(store.Agenda, store.Mirror, clock),
	}
	s, err := web.New(svc, web.Options{Location: loc, Demo: true})
	if err != nil {
		t.Fatal(err)
	}
	return s.Handler(), store
}

func do(t *testing.T, h http.Handler, method, path string, form url.Values, hx bool) *httptest.ResponseRecorder {
	t.Helper()
	var body io.Reader
	if form != nil {
		body = strings.NewReader(form.Encode())
	}
	req := httptest.NewRequest(method, path, body)
	if form != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	if hx {
		req.Header.Set("HX-Request", "true")
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestAgendaDayRendersSeededItems(t *testing.T) {
	h, _ := newServer(t)
	rec := do(t, h, "GET", "/agenda", nil, false)
	if rec.Code != 200 {
		t.Fatalf("status %d", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{"<!doctype html>", "MODO DEMO", "Fisio: Maria Aparecida Souza", "Dona Lúcia", "Vincular", "Novo atendimento", "/static/htmx.min.js"} {
		if !strings.Contains(body, want) {
			t.Errorf("day view missing %q", want)
		}
	}
	// htmx request gets only the partial.
	rec = do(t, h, "GET", "/agenda", nil, true)
	if b := rec.Body.String(); strings.Contains(b, "<!doctype") || !strings.Contains(b, `id="agenda-main"`) {
		t.Fatalf("partial expected, got: %.200s", b)
	}
	if rec := do(t, h, "GET", "/agenda/semana", nil, false); rec.Code != 200 || !strings.Contains(rec.Body.String(), "week-grid") {
		t.Fatalf("week: %d", rec.Code)
	}
	if rec := do(t, h, "GET", "/", nil, false); rec.Code != http.StatusSeeOther {
		t.Fatalf("root redirect: %d", rec.Code)
	}
}

func TestCreatePatientRedirectsAndValidates(t *testing.T) {
	h, store := newServer(t)
	rec := do(t, h, "POST", "/pacientes", url.Values{"nome": {"Nova Paciente"}, "telefone": {"11988887777"}, "nascimento": {"1980-05-06"}}, false)
	if rec.Code != http.StatusSeeOther || !strings.HasPrefix(rec.Header().Get("Location"), "/pacientes/P") {
		t.Fatalf("create: %d %s", rec.Code, rec.Header().Get("Location"))
	}
	id := strings.TrimPrefix(rec.Header().Get("Location"), "/pacientes/")
	p, err := store.Patients.Get(t.Context(), id)
	if err != nil || p.BirthDate.Year() != 1980 {
		t.Fatalf("saved patient: %v %+v", err, p)
	}
	// Flash cookie is shown on the next page.
	req := httptest.NewRequest("GET", "/pacientes/"+id, nil)
	for _, c := range rec.Result().Cookies() {
		req.AddCookie(c)
	}
	rec2 := httptest.NewRecorder()
	h.ServeHTTP(rec2, req)
	if !strings.Contains(rec2.Body.String(), `data-flash="Paciente cadastrado"`) {
		t.Fatal("flash not rendered")
	}
	// htmx create uses HX-Redirect.
	rec = do(t, h, "POST", "/pacientes", url.Values{"nome": {"Outra"}}, true)
	if rec.Code != 200 || rec.Header().Get("HX-Redirect") == "" {
		t.Fatalf("hx create: %d", rec.Code)
	}
	// Invalid.
	rec = do(t, h, "POST", "/pacientes", url.Values{"nome": {"  "}}, false)
	if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), "Informe o nome") {
		t.Fatalf("validation: %d", rec.Code)
	}
}

func TestPatientSearchAndRecordAnd404(t *testing.T) {
	h, _ := newServer(t)
	rec := do(t, h, "GET", "/pacientes?q=jo%C3%A3o", nil, true)
	body := rec.Body.String()
	if rec.Code != 200 || !strings.Contains(body, "João Pedro Lima") || strings.Contains(body, "Maria Aparecida") {
		t.Fatalf("search filter wrong: %d %.300s", rec.Code, body)
	}
	rec = do(t, h, "GET", "/pacientes/P0001", nil, false)
	if rec.Code != 200 {
		t.Fatalf("record: %d", rec.Code)
	}
	for _, want := range []string{"Maria Aparecida Souza", "Evolução", "Próximos atendimentos", "wa.me/5511987654321", "(11) 98765-4321"} {
		if !strings.Contains(rec.Body.String(), want) {
			t.Errorf("record missing %q", want)
		}
	}
	if rec := do(t, h, "GET", "/pacientes/nope", nil, false); rec.Code != http.StatusNotFound {
		t.Fatalf("404 expected, got %d", rec.Code)
	}
	if rec := do(t, h, "GET", "/pacientes/P0001/editar", nil, false); rec.Code != 200 || !strings.Contains(rec.Body.String(), `value="Maria Aparecida Souza"`) {
		t.Fatalf("edit form: %d", rec.Code)
	}
	rec = do(t, h, "POST", "/pacientes/P0001", url.Values{"nome": {"Maria A. Souza"}, "endereco": {"Rua Nova, 1"}}, false)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("update: %d", rec.Code)
	}
}

func TestScheduleFlowViaHTMX(t *testing.T) {
	h, store := newServer(t)
	rec := do(t, h, "GET", "/agenda/novo?data=2026-11-03&paciente=P0002", nil, true)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `value="P0002" selected`) || !strings.Contains(rec.Body.String(), `value="2026-11-03T09:00"`) {
		t.Fatalf("new form: %d %.400s", rec.Code, rec.Body.String())
	}
	rec = do(t, h, "POST", "/agenda", url.Values{"paciente": {"P0002"}, "inicio": {"2026-11-03T10:30"}, "duracao": {"45"}, "observacoes": {"interfone 72"}}, true)
	if rec.Code != 200 || rec.Header().Get("HX-Retarget") != "#agenda-main" || !strings.Contains(rec.Header().Get("HX-Trigger"), "Agendado para 03/11") {
		t.Fatalf("schedule: %d %v", rec.Code, rec.Header())
	}
	if !strings.Contains(rec.Body.String(), "interfone 72") || !strings.Contains(rec.Body.String(), "10:30") {
		t.Fatal("day view after scheduling should show the new item")
	}
	if store.Mirror.Len() == 0 {
		t.Fatal("mirror should be written")
	}
	// Validation error goes back into the modal.
	rec = do(t, h, "POST", "/agenda", url.Values{"paciente": {""}, "inicio": {"2026-11-03T10:30"}}, true)
	if rec.Code != http.StatusUnprocessableEntity || rec.Header().Get("HX-Retarget") != "#modal-body" || !strings.Contains(rec.Body.String(), "Escolha o paciente") {
		t.Fatalf("validation: %d %v", rec.Code, rec.Header())
	}
	// Non-htmx submit redirects to the day.
	rec = do(t, h, "POST", "/agenda", url.Values{"paciente": {"P0002"}, "inicio": {"2026-11-04T08:00"}}, false)
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/agenda?data=2026-11-04" {
		t.Fatalf("plain redirect: %d %s", rec.Code, rec.Header().Get("Location"))
	}
	// "voltar" from a patient record redirects there even for htmx.
	rec = do(t, h, "POST", "/agenda", url.Values{"paciente": {"P0002"}, "inicio": {"2026-11-05T08:00"}, "voltar": {"/pacientes/P0002"}}, true)
	if rec.Header().Get("HX-Redirect") != "/pacientes/P0002" {
		t.Fatalf("voltar: %v", rec.Header())
	}
}

func TestAppointmentActions(t *testing.T) {
	h, store := newServer(t)
	ctx := t.Context()
	all, _ := store.Agenda.ListBetween(ctx, time.Now().Add(-24*time.Hour), time.Now().Add(30*24*time.Hour))
	var linked, loose domain.Appointment
	for _, a := range all {
		if a.Linked() && a.Status == domain.StatusScheduled && linked.ID == "" {
			linked = a
		}
		if !a.Linked() && loose.ID == "" {
			loose = a
		}
	}
	if linked.ID == "" || loose.ID == "" {
		t.Fatal("seed should contain linked and unlinked items")
	}
	rec := do(t, h, "POST", "/agenda/"+linked.ID+"/concluir", nil, true)
	if rec.Code != 200 || !strings.Contains(rec.Header().Get("HX-Trigger"), "concluído") {
		t.Fatalf("done: %d %v", rec.Code, rec.Header())
	}
	if a, _ := store.Agenda.Get(ctx, linked.ID); a.Status != domain.StatusDone {
		t.Fatalf("status not updated: %+v", a)
	}
	rec = do(t, h, "GET", "/agenda/"+linked.ID+"/editar", nil, true)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "Remarcar atendimento") {
		t.Fatalf("edit: %d", rec.Code)
	}
	rec = do(t, h, "POST", "/agenda/"+linked.ID, url.Values{"inicio": {"2026-12-01T15:00"}, "duracao": {"60"}}, true)
	if rec.Code != 200 || !strings.Contains(rec.Header().Get("HX-Trigger"), "Remarcado para 01/12") {
		t.Fatalf("reschedule: %d %v", rec.Code, rec.Header())
	}
	rec = do(t, h, "POST", "/agenda/"+loose.ID+"/vincular", url.Values{"paciente": {"P0003"}}, true)
	if rec.Code != 200 || !strings.Contains(rec.Header().Get("HX-Trigger"), "Vinculado a Fisio: Ant") {
		t.Fatalf("link: %d %v", rec.Code, rec.Header())
	}
	rec = do(t, h, "POST", "/agenda/"+linked.ID+"/cancelar", nil, false)
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/agenda?data=2026-12-01" {
		t.Fatalf("cancel: %d %s", rec.Code, rec.Header().Get("Location"))
	}
	if _, err := store.Agenda.Get(ctx, linked.ID); !domain.IsNotFound(err) {
		t.Fatal("cancelled item should be removed")
	}
	if rec := do(t, h, "POST", "/agenda/nope/concluir", nil, true); rec.Code != http.StatusNotFound {
		t.Fatalf("unknown appointment: %d", rec.Code)
	}
	rec = do(t, h, "POST", "/sincronizar", nil, true)
	if rec.Code != http.StatusNoContent || !strings.Contains(rec.Header().Get("HX-Trigger"), "sincronizada") {
		t.Fatalf("sync: %d %v", rec.Code, rec.Header())
	}
}

func TestEvolutionFlow(t *testing.T) {
	h, store := newServer(t)
	rec := do(t, h, "POST", "/pacientes/P0004/evolucoes", url.Values{"data": {"2026-10-20"}, "texto": {"Primeira sessão: avaliação"}}, true)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "Primeira sessão: avaliação") || !strings.Contains(rec.Body.String(), `id="evolutions"`) {
		t.Fatalf("add evolution: %d", rec.Code)
	}
	if evs, _ := store.Evolutions.ListByPatient(t.Context(), "P0004"); len(evs) != 1 || evs[0].Date.Day() != 20 {
		t.Fatalf("stored: %+v", evs)
	}
	rec = do(t, h, "POST", "/pacientes/P0004/evolucoes", url.Values{"data": {"2026-10-20"}, "texto": {"  "}}, true)
	if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), "Descreva a evolução") || rec.Header().Get("HX-Retarget") != "#evolutions" {
		t.Fatalf("validation: %d %v", rec.Code, rec.Header())
	}
	if rec := do(t, h, "POST", "/pacientes/nope/evolucoes", url.Values{"texto": {"x"}}, false); rec.Code != http.StatusNotFound {
		t.Fatalf("unknown patient: %d", rec.Code)
	}
}

var errTest = errors.New("boom")

type fakeLogin struct{ connected bool }

func (f *fakeLogin) Connected() bool { return f.connected }
func (f *fakeLogin) LoginURL(state string) string {
	return "https://accounts.example/auth?state=" + state
}
func (f *fakeLogin) Exchange(_ context.Context, code string) error {
	if code != "good" {
		return errTest
	}
	f.connected = true
	return nil
}

func TestConnectGateAndOAuthCallback(t *testing.T) {
	login := &fakeLogin{}
	built := false
	s, err := web.New(nil, web.Options{Location: loc, Login: login, Factory: func(context.Context) (*web.Services, error) {
		built = true
		store := memory.NewStore()
		clock := domain.ClockFunc(time.Now)
		return &web.Services{
			Patients:   app.NewPatientService(store.Patients, store.Evolutions, store.Agenda, store.IDs, clock),
			Evolutions: app.NewEvolutionService(store.Evolutions, store.Patients, store.IDs, clock),
			Agenda:     app.NewAgendaService(store.Agenda, store.Mirror, store.Patients, clock, loc),
			Sync:       app.NewSyncService(store.Agenda, store.Mirror, clock),
		}, nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	h := s.Handler()
	rec := do(t, h, "GET", "/agenda", nil, false)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "Conectar com Google") {
		t.Fatalf("gate: %d", rec.Code)
	}
	if rec := do(t, h, "GET", "/healthz", nil, false); rec.Code != 200 || rec.Body.String() != "ok" {
		t.Fatal("healthz must bypass the gate")
	}
	rec = do(t, h, "GET", "/auth/login", nil, false)
	if rec.Code != http.StatusSeeOther || !strings.HasPrefix(rec.Header().Get("Location"), "https://accounts.example/auth?state=") {
		t.Fatalf("login: %d %s", rec.Code, rec.Header().Get("Location"))
	}
	state := strings.TrimPrefix(rec.Header().Get("Location"), "https://accounts.example/auth?state=")
	cookies := rec.Result().Cookies()
	// Wrong state is rejected.
	req := httptest.NewRequest("GET", "/auth/callback?state=bad&code=good", nil)
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rec2 := httptest.NewRecorder()
	h.ServeHTTP(rec2, req)
	if rec2.Code != http.StatusBadRequest {
		t.Fatalf("bad state: %d", rec2.Code)
	}
	req = httptest.NewRequest("GET", "/auth/callback?state="+state+"&code=good", nil)
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rec3 := httptest.NewRecorder()
	h.ServeHTTP(rec3, req)
	if rec3.Code != http.StatusSeeOther || !built || !s.Connected() {
		t.Fatalf("callback: %d built=%v connected=%v", rec3.Code, built, s.Connected())
	}
	if rec := do(t, h, "GET", "/agenda", nil, false); rec.Code != 200 || strings.Contains(rec.Body.String(), "Conectar com Google") {
		t.Fatalf("after connect: %d", rec.Code)
	}
}
