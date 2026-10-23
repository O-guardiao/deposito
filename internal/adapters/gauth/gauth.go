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
	"google.golang.org/api/googleapi"
	"google.golang.org/api/sheets/v4"
)

// Scopes requested from Google: calendar read/write and spreadsheets
// read/write. Sheets has no narrower scope that still allows creating the
// spreadsheet on first run.
var Scopes = []string{calendar.CalendarScope, sheets.SpreadsheetsScope}

// ErrNotConnected is returned before the user completes the login.
var ErrNotConnected = errors.New("conta Google não conectada")

// Config tells the Authenticator where its files are and where Google must
// send the browser back.
type Config struct {
	CredentialsPath string // credentials.json from Google Cloud (Desktop or Web client)
	TokenPath       string // where the token is persisted (0600)
	RedirectURL     string // must be registered in Google Cloud
	// Context bounds the lifetime of the clients handed out by Client: token
	// refreshes run under it. Defaults to context.Background(), i.e. the
	// process lifetime. Never pass a request context here.
	Context context.Context
}

// Authenticator runs the login flow and hands out authenticated HTTP clients.
type Authenticator struct {
	cfg       *oauth2.Config
	tokenPath string
	baseCtx   context.Context
	mu        sync.Mutex
	token     *oauth2.Token
}

// New loads the OAuth client from credentials.json and any previously saved token.
func New(c Config) (*Authenticator, error) {
	raw, err := os.ReadFile(c.CredentialsPath)
	if err != nil {
		return nil, fmt.Errorf("ler credenciais %s: %w", c.CredentialsPath, err)
	}
	cfg, err := google.ConfigFromJSON(raw, Scopes...)
	if err != nil {
		return nil, fmt.Errorf("credenciais inválidas: %w", err)
	}
	cfg.RedirectURL = c.RedirectURL
	return newWithOAuth(cfg, c.TokenPath, c.Context), nil
}

// newWithOAuth is the constructor tests use to point at a fake token endpoint.
func newWithOAuth(cfg *oauth2.Config, tokenPath string, base context.Context) *Authenticator {
	if base == nil {
		base = context.Background()
	}
	a := &Authenticator{cfg: cfg, tokenPath: tokenPath, baseCtx: base}
	a.token, _ = loadToken(tokenPath) // absent token simply means "not connected yet"
	return a
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

// Client returns a long-lived HTTP client that refreshes the token
// automatically (under the Authenticator's base context, not the caller's)
// and persists refreshed tokens.
func (a *Authenticator) Client() (*http.Client, error) {
	a.mu.Lock()
	tok := a.token
	a.mu.Unlock()
	if tok == nil {
		return nil, ErrNotConnected
	}
	src := &persistingSource{auth: a, src: a.cfg.TokenSource(a.baseCtx, tok)}
	return oauth2.NewClient(a.baseCtx, src), nil
}

// NeedsReconnect reports whether err means Google no longer accepts the saved
// token (revoked, expired refresh token, invalid_grant): the user must log in
// again. Forget drops the token so the connect screen comes back.
func (a *Authenticator) NeedsReconnect(err error) bool {
	var re *oauth2.RetrieveError
	if errors.As(err, &re) {
		return re.Response == nil || re.Response.StatusCode == http.StatusBadRequest || re.Response.StatusCode == http.StatusUnauthorized
	}
	var ge *googleapi.Error
	return errors.As(err, &ge) && ge.Code == http.StatusUnauthorized
}

// Forget discards the saved token (memory and disk).
func (a *Authenticator) Forget() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.token = nil
	_ = os.Remove(a.tokenPath)
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
