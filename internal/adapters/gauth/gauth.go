// Package gauth handles the Google OAuth2 login of the single user and keeps
// the token on disk so the app can run unattended afterwards.
package gauth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sync"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/calendar/v3"
	"google.golang.org/api/sheets/v4"
)

// Scopes requested from Google: calendar read/write, spreadsheets read/write
// and creation of files owned by the app.
var Scopes = []string{calendar.CalendarScope, sheets.SpreadsheetsScope, "https://www.googleapis.com/auth/drive.file"}

// ErrNotConnected is returned before the user completes the login.
var ErrNotConnected = errors.New("conta Google não conectada")

// Authenticator runs the login flow and hands out authenticated HTTP clients.
type Authenticator struct {
	cfg       *oauth2.Config
	tokenPath string
	mu        sync.Mutex
	token     *oauth2.Token
}

// New loads the OAuth client from credentials.json (Desktop or Web type) and
// any previously saved token. redirectURL must be registered in Google Cloud.
func New(credentialsPath, tokenPath, redirectURL string) (*Authenticator, error) {
	raw, err := os.ReadFile(credentialsPath)
	if err != nil {
		return nil, fmt.Errorf("ler credenciais %s: %w", credentialsPath, err)
	}
	cfg, err := google.ConfigFromJSON(raw, Scopes...)
	if err != nil {
		return nil, fmt.Errorf("credenciais inválidas: %w", err)
	}
	cfg.RedirectURL = redirectURL
	a := &Authenticator{cfg: cfg, tokenPath: tokenPath}
	a.token, _ = loadToken(tokenPath) // absent token simply means "not connected yet"
	return a, nil
}

// Connected reports whether a token is available.
func (a *Authenticator) Connected() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.token != nil
}

// LoginURL is where the browser must be sent to start the consent flow.
func (a *Authenticator) LoginURL(state string) string {
	return a.cfg.AuthCodeURL(state, oauth2.AccessTypeOffline, oauth2.ApprovalForce)
}

// Exchange turns the callback code into a token and persists it.
func (a *Authenticator) Exchange(ctx context.Context, code string) error {
	tok, err := a.cfg.Exchange(ctx, code)
	if err != nil {
		return fmt.Errorf("trocar código: %w", err)
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.token = tok
	return saveToken(a.tokenPath, tok)
}

// Client returns an HTTP client that refreshes the token automatically and
// persists refreshed tokens.
func (a *Authenticator) Client(ctx context.Context) (*http.Client, error) {
	a.mu.Lock()
	tok := a.token
	a.mu.Unlock()
	if tok == nil {
		return nil, ErrNotConnected
	}
	src := &persistingSource{auth: a, src: a.cfg.TokenSource(ctx, tok)}
	return oauth2.NewClient(ctx, src), nil
}

// persistingSource writes refreshed tokens back to disk.
type persistingSource struct {
	auth *Authenticator
	src  oauth2.TokenSource
}

func (p *persistingSource) Token() (*oauth2.Token, error) {
	tok, err := p.src.Token()
	if err != nil {
		return nil, err
	}
	p.auth.mu.Lock()
	defer p.auth.mu.Unlock()
	if p.auth.token == nil || tok.AccessToken != p.auth.token.AccessToken {
		p.auth.token = tok
		_ = saveToken(p.auth.tokenPath, tok)
	}
	return tok, nil
}

func loadToken(path string) (*oauth2.Token, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var tok oauth2.Token
	if err := json.Unmarshal(raw, &tok); err != nil {
		return nil, err
	}
	if tok.RefreshToken == "" && tok.AccessToken == "" {
		return nil, errors.New("token vazio")
	}
	return &tok, nil
}

func saveToken(path string, tok *oauth2.Token) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(tok, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, raw, 0o600)
}
