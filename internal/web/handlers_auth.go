package web

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"time"
)

const stateCookie = "fisio_oauth_state"

// setupTimeout bounds the spreadsheet bootstrap after login.
const setupTimeout = 60 * time.Second

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if s.opts.Login == nil {
		http.Redirect(w, r, "/agenda", http.StatusSeeOther)
		return
	}
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		s.fail(w, r, err)
		return
	}
	state := hex.EncodeToString(buf)
	http.SetCookie(w, &http.Cookie{Name: stateCookie, Value: state, Path: "/auth/", HttpOnly: true, SameSite: http.SameSiteLaxMode, MaxAge: 600})
	http.Redirect(w, r, s.opts.Login.LoginURL(state), http.StatusSeeOther)
}

func (s *Server) handleCallback(w http.ResponseWriter, r *http.Request) {
	if s.opts.Login == nil {
		http.Redirect(w, r, "/agenda", http.StatusSeeOther)
		return
	}
	c, err := r.Cookie(stateCookie)
	if err != nil || c.Value == "" || c.Value != r.URL.Query().Get("state") {
		http.Error(w, "estado inválido; comece o login novamente", http.StatusBadRequest)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: stateCookie, Value: "", Path: "/auth/", MaxAge: -1})
	if e := r.URL.Query().Get("error"); e != "" {
		s.render(w, r, http.StatusBadRequest, "connect", "", map[string]any{"Error": "O Google recusou o acesso: " + e})
		return
	}
	if err := s.opts.Login.Exchange(r.Context(), r.URL.Query().Get("code")); err != nil {
		s.log.Error("oauth exchange", "err", err)
		s.render(w, r, http.StatusBadGateway, "connect", "", map[string]any{"Error": "Não foi possível concluir o login. Tente novamente."})
		return
	}
	// The setup creates the spreadsheet on first run; it must not be cut
	// short because the browser closed the connection, or the ID is lost.
	ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), setupTimeout)
	defer cancel()
	if err := s.ConnectServices(ctx); err != nil {
		s.log.Error("connect services", "err", err)
		s.render(w, r, http.StatusBadGateway, "connect", "", map[string]any{"Error": "Login feito, mas a planilha/agenda não pôde ser preparada: " + err.Error()})
		return
	}
	s.redirectWithFlash(w, r, "/agenda", "Conta Google conectada")
}

// ConnectServices builds the use cases through the factory and activates
// them. ctx bounds the setup only; see ServicesFactory.
func (s *Server) ConnectServices(ctx context.Context) error {
	if s.opts.Factory == nil {
		return nil
	}
	svc, err := s.opts.Factory(ctx)
	if err != nil {
		return err
	}
	s.services.Store(svc)
	return nil
}
