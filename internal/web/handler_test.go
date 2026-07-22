package web

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/291-Group/LAN-Orangutan/internal/auth"
	"github.com/291-Group/LAN-Orangutan/internal/config"
	"github.com/291-Group/LAN-Orangutan/internal/storage"
)

const testPassword = "test-password"

// newTestHandler builds a web handler backed by throwaway storage.
func newTestHandler(t *testing.T, password string) (*Handler, *auth.Authenticator) {
	t.Helper()

	dir := t.TempDir()
	store, err := storage.New(filepath.Join(dir, "devices.json"), filepath.Join(dir, "state.json"))
	if err != nil {
		t.Fatalf("storage.New: %v", err)
	}

	cfg := config.Default()
	cfg.Server.Password = password

	authn, err := auth.New(password, time.Hour)
	if err != nil {
		t.Fatalf("auth.New: %v", err)
	}

	return NewHandler(store, cfg, authn, "test"), authn
}

func TestLoginPageRenders(t *testing.T) {
	h, _ := newTestHandler(t, testPassword)

	rec := httptest.NewRecorder()
	h.HandleLogin(rec, httptest.NewRequest(http.MethodGet, auth.LoginPath, nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("login page status = %d, want 200", rec.Code)
	}

	body := rec.Body.String()
	if !strings.Contains(body, `name="password"`) {
		t.Error("login page should contain a password field")
	}
	if !strings.Contains(body, `type="password"`) {
		t.Error("the password field should mask what is typed")
	}
}

func TestLoginPageRedirectsWhenNoPasswordConfigured(t *testing.T) {
	h, _ := newTestHandler(t, "")

	rec := httptest.NewRecorder()
	h.HandleLogin(rec, httptest.NewRequest(http.MethodGet, auth.LoginPath, nil))

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want a redirect when there is nothing to sign in to", rec.Code)
	}
}

// postLogin submits the login form with the given password.
func postLogin(t *testing.T, h *Handler, password string) *httptest.ResponseRecorder {
	t.Helper()

	form := url.Values{"password": {password}}
	req := httptest.NewRequest(http.MethodPost, auth.LoginPath, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.RemoteAddr = "192.168.1.9:5000"

	rec := httptest.NewRecorder()
	h.HandleLogin(rec, req)
	return rec
}

func TestSuccessfulLoginSetsCookieAndRedirects(t *testing.T) {
	h, _ := newTestHandler(t, testPassword)

	rec := postLogin(t, h, testPassword)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want a redirect after signing in", rec.Code)
	}
	if got := rec.Header().Get("Location"); got != "/" {
		t.Errorf("redirected to %q, want the dashboard", got)
	}

	cookies := rec.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("a successful login should set a session cookie")
	}
	if cookies[0].Name != auth.SessionCookie {
		t.Errorf("cookie name = %q, want %q", cookies[0].Name, auth.SessionCookie)
	}
	if cookies[0].Value == "" {
		t.Error("the session cookie should carry a token")
	}
}

func TestFailedLoginShowsErrorAndSetsNoCookie(t *testing.T) {
	h, _ := newTestHandler(t, testPassword)

	rec := postLogin(t, h, "wrong-password")

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401 for a rejected password", rec.Code)
	}
	if len(rec.Result().Cookies()) != 0 {
		t.Fatal("a failed login must not set a session cookie")
	}
	if !strings.Contains(rec.Body.String(), "Incorrect password") {
		t.Error("the page should tell the user the password was wrong")
	}
}

func TestFailedLoginDoesNotEchoTheAttempt(t *testing.T) {
	h, _ := newTestHandler(t, testPassword)

	const attempt = "my-secret-guess"
	body := postLogin(t, h, attempt).Body.String()

	// Reflecting the attempted password back into the page would leak it into
	// browser history and any shared screenshot.
	if strings.Contains(body, attempt) {
		t.Error("the failed attempt must not be echoed back into the page")
	}
}

func TestLockoutMessageAfterRepeatedFailures(t *testing.T) {
	h, _ := newTestHandler(t, testPassword)

	var last *httptest.ResponseRecorder
	for i := 0; i < 6; i++ {
		last = postLogin(t, h, "wrong-password")
	}

	if !strings.Contains(last.Body.String(), "Too many failed attempts") {
		t.Error("after repeated failures the page should explain the lockout")
	}
}

func TestLogoutClearsCookieAndRedirects(t *testing.T) {
	h, authn := newTestHandler(t, testPassword)

	token, ok := authn.Login("192.168.1.9:5000", testPassword)
	if !ok {
		t.Fatal("login failed")
	}

	req := httptest.NewRequest(http.MethodGet, auth.LogoutPath, nil)
	req.AddCookie(&http.Cookie{Name: auth.SessionCookie, Value: token})
	rec := httptest.NewRecorder()

	h.HandleLogout(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want a redirect after signing out", rec.Code)
	}
	if got := rec.Header().Get("Location"); got != auth.LoginPath {
		t.Errorf("redirected to %q, want the login page", got)
	}

	cookies := rec.Result().Cookies()
	if len(cookies) == 0 || cookies[0].MaxAge >= 0 {
		t.Error("signing out should expire the session cookie")
	}

	// The session must be dead server side, not just cleared in the browser.
	after := httptest.NewRequest(http.MethodGet, "/", nil)
	after.AddCookie(&http.Cookie{Name: auth.SessionCookie, Value: token})
	rec2 := httptest.NewRecorder()
	authn.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})).ServeHTTP(rec2, after)

	if rec2.Code == http.StatusOK {
		t.Error("the old token should be rejected after signing out")
	}
}

func TestLoginRejectsUnsupportedMethod(t *testing.T) {
	h, _ := newTestHandler(t, testPassword)

	rec := httptest.NewRecorder()
	h.HandleLogin(rec, httptest.NewRequest(http.MethodDelete, auth.LoginPath, nil))

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", rec.Code)
	}
}

// TestDashboardIsProtected checks the whole route wiring the way the server
// assembles it, rather than testing the middleware in isolation.
func TestDashboardIsProtected(t *testing.T) {
	h, authn := newTestHandler(t, testPassword)

	mux := http.NewServeMux()
	mux.Handle("/", authn.Middleware(reached()))
	mux.Handle("/static/", h.StaticHandler())
	mux.HandleFunc(auth.LoginPath, h.HandleLogin)

	tests := []struct {
		name       string
		path       string
		wantStatus int
	}{
		{"dashboard requires a session", "/", http.StatusSeeOther},
		{"settings requires a session", "/settings", http.StatusSeeOther},
		{"login page stays reachable", auth.LoginPath, http.StatusOK},
		{"stylesheet stays reachable", "/static/style.css", http.StatusOK},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, tt.path, nil))

			if rec.Code != tt.wantStatus {
				t.Errorf("GET %s = %d, want %d", tt.path, rec.Code, tt.wantStatus)
			}
		})
	}
}

// reached stands in for the dashboard when the test cares about whether a
// request got through the middleware, not about what the page renders.
// Rendering the real dashboard shells out to network tools, which does not
// belong in a test of the authentication wiring.
func reached() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
}

func TestDashboardReachableAfterSignIn(t *testing.T) {
	_, authn := newTestHandler(t, testPassword)

	mux := http.NewServeMux()
	mux.Handle("/", authn.Middleware(reached()))

	token, ok := authn.Login("192.168.1.9:5000", testPassword)
	if !ok {
		t.Fatal("login failed")
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: auth.SessionCookie, Value: token})
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("dashboard status = %d, want 200 once signed in", rec.Code)
	}
}

func TestDashboardOpenWhenNoPasswordSet(t *testing.T) {
	_, authn := newTestHandler(t, "")

	mux := http.NewServeMux()
	mux.Handle("/", authn.Middleware(reached()))

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("dashboard status = %d, want 200 when no password is configured", rec.Code)
	}
}

// --- First run setup ---------------------------------------------------

// newSetupHandler builds a handler in the first run state, plus a pointer to
// wherever a saved password hash ends up.
func newSetupHandler(t *testing.T) (*Handler, *auth.Authenticator, *string) {
	t.Helper()

	h, authn := newTestHandler(t, "")
	authn.SetSetupRequired(true)

	var saved string
	return h, authn, &saved
}

func setupHandler(h *Handler, saved *string) http.HandlerFunc {
	return h.HandleSetup(func(hash string) error {
		*saved = hash
		return nil
	})
}

// postSetup submits the setup form.
func postSetup(h *Handler, saved *string, password, confirm string) *httptest.ResponseRecorder {
	form := url.Values{"password": {password}, "confirm": {confirm}}
	req := httptest.NewRequest(http.MethodPost, auth.SetupPath, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.RemoteAddr = "192.168.1.9:5000"

	rec := httptest.NewRecorder()
	setupHandler(h, saved)(rec, req)
	return rec
}

func TestSetupPageShownOnFirstRun(t *testing.T) {
	h, _, saved := newSetupHandler(t)

	rec := httptest.NewRecorder()
	setupHandler(h, saved)(rec, httptest.NewRequest(http.MethodGet, auth.SetupPath, nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("setup page status = %d, want 200", rec.Code)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "Create a password") {
		t.Error("the setup page should ask the user to create a password")
	}
	if !strings.Contains(body, `name="confirm"`) {
		t.Error("the setup page should ask for confirmation")
	}
}

func TestEverythingRedirectsToSetupOnFirstRun(t *testing.T) {
	h, authn, saved := newSetupHandler(t)

	mux := http.NewServeMux()
	mux.Handle("/", authn.Middleware(reached()))
	mux.Handle("/static/", h.StaticHandler())
	mux.HandleFunc(auth.SetupPath, setupHandler(h, saved))
	mux.HandleFunc(auth.LoginPath, h.HandleLogin)

	tests := []struct {
		name       string
		path       string
		wantStatus int
		wantLoc    string
	}{
		{"dashboard goes to setup", "/", http.StatusSeeOther, auth.SetupPath},
		{"settings goes to setup", "/settings", http.StatusSeeOther, auth.SetupPath},
		{"login goes to setup", auth.LoginPath, http.StatusSeeOther, auth.SetupPath},
		{"setup itself is reachable", auth.SetupPath, http.StatusOK, ""},
		{"stylesheet is reachable", "/static/style.css", http.StatusOK, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, tt.path, nil))

			if rec.Code != tt.wantStatus {
				t.Errorf("GET %s = %d, want %d", tt.path, rec.Code, tt.wantStatus)
			}
			if tt.wantLoc != "" {
				if got := rec.Header().Get("Location"); got != tt.wantLoc {
					t.Errorf("GET %s redirected to %q, want %q", tt.path, got, tt.wantLoc)
				}
			}
		})
	}
}

func TestAPIRefusedDuringSetup(t *testing.T) {
	_, authn, _ := newSetupHandler(t)

	mux := http.NewServeMux()
	mux.Handle("/api/", authn.Middleware(reached()))

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/devices", nil))

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("API during setup = %d, want 401", rec.Code)
	}
}

func TestSetupCreatesPasswordAndSignsIn(t *testing.T) {
	h, authn, saved := newSetupHandler(t)

	rec := postSetup(h, saved, "a-good-password", "a-good-password")

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want a redirect after setup", rec.Code)
	}
	if got := rec.Header().Get("Location"); got != "/" {
		t.Errorf("redirected to %q, want the dashboard", got)
	}

	if !authn.Enabled() {
		t.Error("a password should exist after setup")
	}
	if authn.NeedsSetup() {
		t.Error("setup should be complete")
	}

	// Signed straight in rather than bounced to a login form.
	cookies := rec.Result().Cookies()
	if len(cookies) == 0 || cookies[0].Name != auth.SessionCookie {
		t.Error("completing setup should sign the user in")
	}
}

func TestSetupPersistsTheHash(t *testing.T) {
	h, _, saved := newSetupHandler(t)

	postSetup(h, saved, "a-good-password", "a-good-password")

	if *saved == "" {
		t.Fatal("the new password should have been handed over for saving")
	}
	if strings.Contains(*saved, "a-good-password") {
		t.Error("what gets saved must be a hash, not the password itself")
	}
	if !strings.HasPrefix(*saved, "$2") {
		t.Errorf("saved value %q does not look like a bcrypt hash", *saved)
	}
}

func TestSetupRejectsMismatchedConfirmation(t *testing.T) {
	h, authn, saved := newSetupHandler(t)

	rec := postSetup(h, saved, "a-good-password", "a-different-password")

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "do not match") {
		t.Error("the page should say the passwords do not match")
	}
	if authn.Enabled() {
		t.Error("no password should have been set")
	}
	if *saved != "" {
		t.Error("nothing should have been saved")
	}
}

func TestSetupRejectsShortPassword(t *testing.T) {
	h, authn, saved := newSetupHandler(t)

	short := strings.Repeat("a", auth.MinPasswordLength-1)
	rec := postSetup(h, saved, short, short)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
	if authn.Enabled() {
		t.Error("a too-short password should not be accepted")
	}
}

func TestSetupCannotBeUsedTwice(t *testing.T) {
	h, authn, saved := newSetupHandler(t)

	postSetup(h, saved, "first-password", "first-password")
	firstHash := *saved

	// A second attempt must not be able to take the account over.
	rec := postSetup(h, saved, "attacker-password", "attacker-password")

	if rec.Code != http.StatusSeeOther {
		t.Errorf("status = %d, want a redirect away from setup", rec.Code)
	}
	if *saved != firstHash {
		t.Error("the stored password must not be overwritten by a second setup attempt")
	}
	if _, ok := authn.Login("192.168.1.9:5000", "attacker-password"); ok {
		t.Fatal("the second password must not work")
	}
	if _, ok := authn.Login("192.168.1.9:5000", "first-password"); !ok {
		t.Error("the original password should still work")
	}
}

func TestSetupReportsSaveFailure(t *testing.T) {
	h, _, _ := newSetupHandler(t)

	failing := h.HandleSetup(func(string) error {
		return fmt.Errorf("disk is full")
	})

	form := url.Values{"password": {"a-good-password"}, "confirm": {"a-good-password"}}
	req := httptest.NewRequest(http.MethodPost, auth.SetupPath, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	failing(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500 when the password cannot be saved", rec.Code)
	}
	// Silently carrying on would lose the password at the next restart.
	if !strings.Contains(rec.Body.String(), "disk is full") {
		t.Error("the user should be told why the password could not be saved")
	}
}

func TestNoSetupWhenPasswordAlreadyConfigured(t *testing.T) {
	h, _ := newTestHandler(t, testPassword)

	var saved string
	rec := httptest.NewRecorder()
	setupHandler(h, &saved)(rec, httptest.NewRequest(http.MethodGet, auth.SetupPath, nil))

	if rec.Code != http.StatusSeeOther {
		t.Errorf("status = %d, want a redirect when a password already exists", rec.Code)
	}
}
