package auth

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestNewAPITokenShape(t *testing.T) {
	token, hash, err := NewAPIToken()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.HasPrefix(token, TokenPrefix) {
		t.Errorf("token %q does not carry the %q prefix", token, TokenPrefix)
	}
	if HashAPIToken(token) != hash {
		t.Error("returned hash does not match the token")
	}
	if len(hash) != 64 {
		t.Errorf("expected a 64 character SHA-256 hex hash, got %d", len(hash))
	}

	// Two tokens in a row must not collide, or the randomness is not random.
	other, _, _ := NewAPIToken()
	if other == token {
		t.Error("two generated tokens were identical")
	}
}

func TestBearerToken(t *testing.T) {
	cases := []struct{ header, want string }{
		{"Bearer lano_abc", "lano_abc"},
		{"bearer lano_abc", "lano_abc"}, // scheme is case insensitive
		{"BEARER lano_abc", "lano_abc"},
		{"lano_abc", "lano_abc"}, // bare token, the common first mistake
		{"  Bearer   lano_abc  ", "lano_abc"},
		{"", ""},
		{"Basic dXNlcjpwYXNz", ""}, // a different scheme is not a token
	}
	for _, c := range cases {
		r := httptest.NewRequest(http.MethodGet, "/api/devices", nil)
		if c.header != "" {
			r.Header.Set("Authorization", c.header)
		}
		if got := bearerToken(r); got != c.want {
			t.Errorf("header %q: got %q, want %q", c.header, got, c.want)
		}
	}
}

func TestAPITokenAuthenticates(t *testing.T) {
	a, err := New("correct-horse-battery-staple", time.Hour)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	token, hash, _ := NewAPIToken()
	a.SetAPITokenHash(hash)

	if !a.HasAPIToken() {
		t.Error("expected HasAPIToken to be true once a hash is set")
	}

	r := httptest.NewRequest(http.MethodGet, "/api/devices", nil)
	r.Header.Set("Authorization", "Bearer "+token)
	if !a.authenticated(r) {
		t.Error("expected the correct token to authenticate")
	}

	wrong := httptest.NewRequest(http.MethodGet, "/api/devices", nil)
	wrong.Header.Set("Authorization", "Bearer lano_not-the-real-token")
	if a.authenticated(wrong) {
		t.Error("expected a wrong token to be rejected")
	}

	none := httptest.NewRequest(http.MethodGet, "/api/devices", nil)
	if a.authenticated(none) {
		t.Error("expected a request with no credential to be rejected")
	}
}

// enable_api = false means scripts may not use the API at all, which is
// applied by refusing the token while leaving sessions alone.
func TestTokenAccessCanBeSwitchedOff(t *testing.T) {
	a, err := New("correct-horse-battery-staple", time.Hour)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	token, hash, _ := NewAPIToken()
	a.SetAPITokenHash(hash)
	if !a.TokenAccessEnabled() {
		t.Fatal("token access should be on by default")
	}

	r := httptest.NewRequest(http.MethodGet, "/api/devices", nil)
	r.Header.Set("Authorization", "Bearer "+token)

	a.SetTokenAccess(false)
	if a.TokenAccessEnabled() {
		t.Error("TokenAccessEnabled should report the switch")
	}
	if a.authenticated(r) || a.TokenAuthenticated(r) {
		t.Error("a valid token must be refused while token access is off")
	}
	if !a.HasAPIToken() {
		t.Error("switching access off does not revoke the token itself")
	}

	a.SetTokenAccess(true)
	if !a.authenticated(r) {
		t.Error("the token should work again once access is back on")
	}
}

// A token is for programs. Honouring it on a browser page would invite people
// to put a long-lived credential in a URL or a bookmark, and the page itself
// would be useless anyway because every link from it would bounce to login.
func TestAPITokenOnlyWorksOnAPIPaths(t *testing.T) {
	a, _ := New("correct-horse-battery-staple", time.Hour)
	token, hash, _ := NewAPIToken()
	a.SetAPITokenHash(hash)

	for _, path := range []string{"/", "/settings", "/login", "/static/app.js"} {
		r := httptest.NewRequest(http.MethodGet, path, nil)
		r.Header.Set("Authorization", "Bearer "+token)
		if a.authenticated(r) {
			t.Errorf("token unexpectedly accepted on a non-API path: %s", path)
		}
	}

	r := httptest.NewRequest(http.MethodGet, "/api/stats", nil)
	r.Header.Set("Authorization", "Bearer "+token)
	if !a.authenticated(r) {
		t.Error("token should still work on an API path")
	}
}

// With no token created, presenting anything at all must fail. Otherwise an
// empty stored hash could be matched by an empty presented one.
func TestNoTokenMeansNoTokenAccess(t *testing.T) {
	a, _ := New("correct-horse-battery-staple", time.Hour)
	if a.HasAPIToken() {
		t.Fatal("a fresh authenticator should have no API token")
	}
	for _, header := range []string{"Bearer ", "Bearer x", ""} {
		r := httptest.NewRequest(http.MethodGet, "/api/devices", nil)
		r.Header.Set("Authorization", header)
		if a.authenticated(r) {
			t.Errorf("header %q authenticated against an empty token", header)
		}
	}
}

// The end-to-end path: a request with a valid token must get through the
// middleware that guards every handler.
func TestMiddlewareAcceptsAPIToken(t *testing.T) {
	a, _ := New("correct-horse-battery-staple", time.Hour)
	token, hash, _ := NewAPIToken()
	a.SetAPITokenHash(hash)

	var reached bool
	handler := a.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reached = true
		w.WriteHeader(http.StatusOK)
	}))

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/devices", nil)
	r.Header.Set("Authorization", "Bearer "+token)
	handler.ServeHTTP(w, r)

	if !reached {
		t.Fatalf("handler was not reached; middleware returned %d", w.Code)
	}

	// And without it, the API answers 401 as JSON rather than redirecting.
	reached = false
	w2 := httptest.NewRecorder()
	handler.ServeHTTP(w2, httptest.NewRequest(http.MethodGet, "/api/devices", nil))
	if reached {
		t.Error("handler was reached without a credential")
	}
	if w2.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for the API, got %d", w2.Code)
	}
}

func TestSaveAndLoadAPITokenHash(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "api-token")

	if got := LoadAPITokenHash(path); got != "" {
		t.Errorf("expected no token before one is saved, got %q", got)
	}

	_, hash, _ := NewAPIToken()
	if err := SaveAPITokenHash(path, hash); err != nil {
		t.Fatalf("SaveAPITokenHash: %v", err)
	}
	if got := LoadAPITokenHash(path); got != hash {
		t.Errorf("round trip failed: got %q, want %q", got, hash)
	}

	// The file holds the hash and never the token, and only its owner may read it.
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("expected mode 0600 on the token file, got %o", perm)
	}

	// Revoking removes it, and revoking again is not an error.
	if err := SaveAPITokenHash(path, ""); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	if got := LoadAPITokenHash(path); got != "" {
		t.Errorf("expected the token to be gone after revoking, got %q", got)
	}
	if err := SaveAPITokenHash(path, ""); err != nil {
		t.Errorf("revoking twice should be harmless, got %v", err)
	}
}

// A file that has been hand-edited into something that is not a hash must be
// ignored rather than loaded, or a truncated file could become a short secret.
func TestLoadAPITokenHashRejectsGarbage(t *testing.T) {
	dir := t.TempDir()
	for i, content := range []string{"", "not-a-hash", "abc123", strings.Repeat("z", 64)} {
		path := filepath.Join(dir, "t"+string(rune('a'+i)))
		if err := os.WriteFile(path, []byte(content+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		if got := LoadAPITokenHash(path); got != "" {
			t.Errorf("content %q was accepted as a hash: %q", content, got)
		}
	}
}

// A token created or revoked by another process (the CLI) must take effect in a
// running server without a restart. A revoke that waited for a restart would
// leave a credential working that the user believes is dead.
func TestWatchedTokenFileFollowsOtherProcess(t *testing.T) {
	a, _ := New("correct-horse-battery-staple", time.Hour)
	path := filepath.Join(t.TempDir(), "api-token")
	a.WatchAPITokenFile(path)

	token, hash, err := NewAPIToken()
	if err != nil {
		t.Fatal(err)
	}
	req := func() bool {
		r := httptest.NewRequest(http.MethodGet, "/api/devices", nil)
		r.Header.Set("Authorization", "Bearer "+token)
		return a.apiTokenAuthenticated(r)
	}

	if req() || a.HasAPIToken() {
		t.Fatal("no token file yet, nothing should authenticate")
	}

	// Another process creates the token.
	if err := SaveAPITokenHash(path, hash); err != nil {
		t.Fatal(err)
	}
	if !req() {
		t.Fatal("token written by another process was not picked up")
	}

	// Another process revokes it.
	if err := SaveAPITokenHash(path, ""); err != nil {
		t.Fatal(err)
	}
	if req() || a.HasAPIToken() {
		t.Fatal("token revoked by another process still authenticates")
	}
}
