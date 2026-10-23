package gauth

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewWithoutTokenAndLoginURL(t *testing.T) {
	dir := t.TempDir()
	cred := filepath.Join(dir, "credentials.json")
	_ = os.WriteFile(cred, []byte(`{"installed":{"client_id":"id","client_secret":"s","auth_uri":"https://accounts.google.com/o/oauth2/auth","token_uri":"https://oauth2.googleapis.com/token","redirect_uris":["http://localhost"]}}`), 0o600)
	a, err := New(cred, filepath.Join(dir, "token.json"), "http://localhost:8080/auth/callback")
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
	if _, err := a.Client(t.Context()); err != ErrNotConnected {
		t.Fatalf("want ErrNotConnected, got %v", err)
	}
	// A saved token makes a fresh Authenticator connected.
	_ = os.WriteFile(filepath.Join(dir, "token.json"), []byte(`{"access_token":"a","refresh_token":"r","token_type":"Bearer"}`), 0o600)
	b, _ := New(cred, filepath.Join(dir, "token.json"), "http://localhost:8080/auth/callback")
	if !b.Connected() {
		t.Fatal("token on disk should connect")
	}
}
