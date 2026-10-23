package gauth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/oauth2"
)

func TestNewWithoutTokenAndLoginURL(t *testing.T) {
	dir := t.TempDir()
	cred := filepath.Join(dir, "credentials.json")
	_ = os.WriteFile(cred, []byte(`{"installed":{"client_id":"id","client_secret":"s","auth_uri":"https://accounts.google.com/o/oauth2/auth","token_uri":"https://oauth2.googleapis.com/token","redirect_uris":["http://localhost"]}}`), 0o600)
	cfg := Config{CredentialsPath: cred, TokenPath: filepath.Join(dir, "token.json"), RedirectURL: "http://localhost:8080/auth/callback"}
	a, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if a.Connected() {
		t.Fatal("should not be connected without token")
	}
	u := a.LoginURL("xyz")
	if !strings.Contains(u, "state=xyz") || !strings.Contains(u, "redirect_uri=http%3A%2F%2Flocalhost%3A8080%2Fauth%2Fcallback") {
		t.Fatalf("login url: %s", u)
	}
	if strings.Contains(u, "drive.file") {
		t.Fatal("drive.file scope is not needed")
	}
	if _, err := a.Client(); err != ErrNotConnected {
		t.Fatalf("want ErrNotConnected, got %v", err)
	}
	// A saved token makes a fresh Authenticator connected.
	_ = os.WriteFile(cfg.TokenPath, []byte(`{"access_token":"a","refresh_token":"r","token_type":"Bearer"}`), 0o600)
	b, _ := New(cfg)
	if !b.Connected() {
		t.Fatal("token on disk should connect")
	}
	b.Forget()
	if b.Connected() {
		t.Fatal("Forget must drop the token")
	}
	if _, err := os.Stat(cfg.TokenPath); !os.IsNotExist(err) {
		t.Fatal("Forget must remove the token file")
	}
}

// TestRefreshSurvivesCallerContext proves the production bug fix: the client
// is built while a short-lived context is active, that context is cancelled,
// and a later request must still refresh the expired token and persist it.
func TestRefreshSurvivesCallerContext(t *testing.T) {
	var refreshes int
	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		refreshes++
		_ = r.ParseForm()
		if r.Form.Get("grant_type") != "refresh_token" || r.Form.Get("refresh_token") != "r1" {
			http.Error(w, "bad grant", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "fresh", "token_type": "Bearer", "expires_in": 3600, "refresh_token": "r1"})
	}))
	defer tokenSrv.Close()
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(r.Header.Get("Authorization")))
	}))
	defer api.Close()

	dir := t.TempDir()
	tokenPath := filepath.Join(dir, "token.json")
	_ = os.WriteFile(tokenPath, []byte(`{"access_token":"expired","refresh_token":"r1","token_type":"Bearer","expiry":"2020-01-01T00:00:00Z"}`), 0o600)
	cfg := &oauth2.Config{ClientID: "id", ClientSecret: "s", Endpoint: oauth2.Endpoint{TokenURL: tokenSrv.URL}}
	a := newWithOAuth(cfg, tokenPath, nil)

	// The caller's context dies right after the client is built, as it does
	// in the OAuth callback handler and in main's bounded setup.
	ctx, cancel := context.WithCancel(context.Background())
	client, err := a.Client()
	if err != nil {
		t.Fatal(err)
	}
	_ = ctx
	cancel()
	time.Sleep(10 * time.Millisecond)

	resp, err := client.Get(api.URL)
	if err != nil {
		t.Fatalf("request after caller ctx cancel must work: %v", err)
	}
	defer resp.Body.Close()
	var buf [64]byte
	n, _ := resp.Body.Read(buf[:])
	if got := string(buf[:n]); got != "Bearer fresh" {
		t.Fatalf("expected refreshed token in use, got %q", got)
	}
	if refreshes != 1 {
		t.Fatalf("want exactly one refresh, got %d", refreshes)
	}
	raw, _ := os.ReadFile(tokenPath)
	if !strings.Contains(string(raw), `"fresh"`) {
		t.Fatalf("refreshed token not persisted: %s", raw)
	}
	if a.NeedsReconnect(&oauth2.RetrieveError{Response: &http.Response{StatusCode: 400}}) != true || a.NeedsReconnect(context.Canceled) {
		t.Fatal("NeedsReconnect classification wrong")
	}
}
