package auth

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const testPassword = "correct-horse-battery-staple"

func newTestAuth(t *testing.T) *Authenticator {
	t.Helper()
	a, err := New(testPassword, time.Hour)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return a
}

// okHandler reports that the request reached the protected handler.
func okHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("protected"))
	})
}

func TestDisabledWithoutPassword(t *testing.T) {
	a, err := New("", time.Hour)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if a.Enabled() {
		t.Fatal("authentication should be disabled when no password is set")
	}

	// With no password, the middleware must not interfere at all.
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	a.Middleware(okHandler()).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected request to pass through, got status %d", rec.Code)
	}
}

func TestEnabledWithPassword(t *testing.T) {
	a := newTestAuth(t)
	if !a.Enabled() {
		t.Fatal("authentication should be enabled when a password is set")
	}
}

func TestLoginCorrectPassword(t *testing.T) {
	a := newTestAuth(t)

	token, ok := a.Login("192.168.1.5:5000", testPassword)
	if !ok {
		t.Fatal("login with the correct password should succeed")
	}
	if token == "" {
		t.Fatal("successful login should return a session token")
	}
}

func TestLoginWrongPassword(t *testing.T) {
	a := newTestAuth(t)

	if _, ok := a.Login("192.168.1.5:5000", "wrong"); ok {
		t.Fatal("login with the wrong password should fail")
	}
}

func TestLoginIssuesUniqueTokens(t *testing.T) {
	a := newTestAuth(t)

	first, ok := a.Login("192.168.1.5:5000", testPassword)
	if !ok {
		t.Fatal("first login failed")
	}
	second, ok := a.Login("192.168.1.6:5000", testPassword)
	if !ok {
		t.Fatal("second login failed")
	}
	if first == second {
		t.Fatal("each login must produce a distinct session token")
	}
}

func TestSessionGrantsAccess(t *testing.T) {
	a := newTestAuth(t)

	token, ok := a.Login("192.168.1.5:5000", testPassword)
	if !ok {
		t.Fatal("login failed")
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: SessionCookie, Value: token})
	rec := httptest.NewRecorder()

	a.Middleware(okHandler()).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("valid session should reach the handler, got status %d", rec.Code)
	}
}

func TestNoCookieRedirectsToLogin(t *testing.T) {
	a := newTestAuth(t)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	a.Middleware(okHandler()).ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("expected redirect to login, got status %d", rec.Code)
	}
	if got := rec.Header().Get("Location"); got != LoginPath {
		t.Fatalf("expected redirect to %q, got %q", LoginPath, got)
	}
}

func TestBogusCookieRejected(t *testing.T) {
	a := newTestAuth(t)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: SessionCookie, Value: "not-a-real-token"})
	rec := httptest.NewRecorder()

	a.Middleware(okHandler()).ServeHTTP(rec, req)

	if rec.Code == http.StatusOK {
		t.Fatal("a forged session token must not grant access")
	}
}

func TestAPIRequestGets401JSON(t *testing.T) {
	a := newTestAuth(t)

	req := httptest.NewRequest(http.MethodGet, "/api/devices", nil)
	rec := httptest.NewRecorder()

	a.Middleware(okHandler()).ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("API should answer 401, got %d", rec.Code)
	}

	var body struct {
		Success bool   `json:"success"`
		Error   string `json:"error"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("API 401 should carry a JSON body: %v", err)
	}
	if body.Success {
		t.Fatal("API 401 body should report success=false")
	}
}

func TestSessionExpires(t *testing.T) {
	a, err := New(testPassword, 20*time.Millisecond)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	token, ok := a.Login("192.168.1.5:5000", testPassword)
	if !ok {
		t.Fatal("login failed")
	}

	time.Sleep(40 * time.Millisecond)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: SessionCookie, Value: token})
	rec := httptest.NewRecorder()

	a.Middleware(okHandler()).ServeHTTP(rec, req)

	if rec.Code == http.StatusOK {
		t.Fatal("an expired session must not grant access")
	}
}

func TestLogoutInvalidatesSession(t *testing.T) {
	a := newTestAuth(t)

	token, ok := a.Login("192.168.1.5:5000", testPassword)
	if !ok {
		t.Fatal("login failed")
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: SessionCookie, Value: token})
	a.Logout(req)

	rec := httptest.NewRecorder()
	a.Middleware(okHandler()).ServeHTTP(rec, req)

	if rec.Code == http.StatusOK {
		t.Fatal("a logged-out session must not grant access")
	}
}

func TestLockoutAfterRepeatedFailures(t *testing.T) {
	a := newTestAuth(t)
	const addr = "192.168.1.50:5000"

	for i := 0; i < maxAttempts; i++ {
		if _, ok := a.Login(addr, "wrong"); ok {
			t.Fatal("wrong password should never succeed")
		}
	}

	if !a.LockedOut(addr) {
		t.Fatalf("address should be locked out after %d failures", maxAttempts)
	}

	// Even the correct password is refused while locked out.
	if _, ok := a.Login(addr, testPassword); ok {
		t.Fatal("login should be refused while locked out")
	}
}

func TestLockoutIsPerAddressNotPerPort(t *testing.T) {
	a := newTestAuth(t)

	// Same host, different source ports, as a real client would produce.
	for i := 0; i < maxAttempts; i++ {
		a.Login("192.168.1.50:600"+string(rune('0'+i)), "wrong")
	}

	if !a.LockedOut("192.168.1.50:9999") {
		t.Fatal("lockout must key on host so a new source port does not reset it")
	}
}

func TestLockoutDoesNotAffectOtherAddresses(t *testing.T) {
	a := newTestAuth(t)

	for i := 0; i < maxAttempts; i++ {
		a.Login("192.168.1.50:5000", "wrong")
	}

	if a.LockedOut("192.168.1.51:5000") {
		t.Fatal("one client's lockout must not lock out a different client")
	}
	if _, ok := a.Login("192.168.1.51:5000", testPassword); !ok {
		t.Fatal("an unrelated client should still be able to log in")
	}
}

func TestSuccessfulLoginClearsFailureCount(t *testing.T) {
	a := newTestAuth(t)
	const addr = "192.168.1.50:5000"

	// Stay one below the limit, then succeed.
	for i := 0; i < maxAttempts-1; i++ {
		a.Login(addr, "wrong")
	}
	if _, ok := a.Login(addr, testPassword); !ok {
		t.Fatal("login should succeed before the limit is reached")
	}

	// The counter should have reset, so a fresh run of failures is needed.
	for i := 0; i < maxAttempts-1; i++ {
		a.Login(addr, "wrong")
	}
	if a.LockedOut(addr) {
		t.Fatal("a successful login should reset the failure count")
	}
}

func TestAcceptsPreHashedPassword(t *testing.T) {
	hash, err := HashPassword(testPassword)
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}

	a, err := New(hash, time.Hour)
	if err != nil {
		t.Fatalf("New with hash: %v", err)
	}
	if !a.Enabled() {
		t.Fatal("a pre-hashed password should enable authentication")
	}
	if _, ok := a.Login("192.168.1.5:5000", testPassword); !ok {
		t.Fatal("the original password should work against a stored hash")
	}
	if _, ok := a.Login("192.168.1.6:5000", hash); ok {
		t.Fatal("the hash itself must not be accepted as the password")
	}
}

func TestPlaintextPasswordIsNotStored(t *testing.T) {
	a := newTestAuth(t)
	if string(a.hash) == testPassword {
		t.Fatal("the password must be hashed, not stored as given")
	}
}

func TestSetAndClearCookie(t *testing.T) {
	a := newTestAuth(t)
	req := httptest.NewRequest(http.MethodGet, "/", nil)

	rec := httptest.NewRecorder()
	a.SetCookie(rec, req, "token-value")
	cookie := rec.Result().Cookies()[0]

	if cookie.Value != "token-value" {
		t.Fatalf("unexpected cookie value %q", cookie.Value)
	}
	if !cookie.HttpOnly {
		t.Fatal("session cookie must be HttpOnly so scripts cannot read it")
	}
	if cookie.SameSite != http.SameSiteLaxMode {
		t.Fatal("session cookie should set SameSite to limit cross-site use")
	}

	rec = httptest.NewRecorder()
	a.ClearCookie(rec, req)
	cleared := rec.Result().Cookies()[0]
	if cleared.MaxAge >= 0 {
		t.Fatal("clearing the cookie should expire it")
	}
}

func TestExpiredSessionsArePruned(t *testing.T) {
	a, err := New(testPassword, 10*time.Millisecond)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	if _, ok := a.Login("192.168.1.5:5000", testPassword); !ok {
		t.Fatal("login failed")
	}
	time.Sleep(30 * time.Millisecond)

	// A later login prunes anything that has expired in the meantime.
	if _, ok := a.Login("192.168.1.6:5000", testPassword); !ok {
		t.Fatal("second login failed")
	}

	a.mu.Lock()
	count := len(a.sessions)
	a.mu.Unlock()

	if count != 1 {
		t.Fatalf("expected only the live session to remain, found %d", count)
	}
}

func TestPasswordLengthLimits(t *testing.T) {
	tests := []struct {
		name     string
		password string
		wantErr  bool
	}{
		{"too short", strings.Repeat("a", MinPasswordLength-1), true},
		{"exactly the minimum", strings.Repeat("a", MinPasswordLength), false},
		{"a normal passphrase", "correct horse battery staple", false},
		{"exactly bcrypt's limit", strings.Repeat("a", MaxPasswordBytes), false},
		{"one byte over the limit", strings.Repeat("a", MaxPasswordBytes+1), true},
		// Accented characters take two bytes each, so this is 74 bytes.
		{"non-Latin over the byte limit", strings.Repeat("é", 37), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePassword(tt.password)
			if tt.wantErr && err == nil {
				t.Error("expected this password to be rejected")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("expected this password to be accepted, got: %v", err)
			}
			// Whatever we say, it must not be library jargon.
			if err != nil && strings.Contains(err.Error(), "bcrypt") {
				t.Errorf("error text leaks implementation detail: %v", err)
			}
		})
	}
}

func TestSetPasswordRejectsOverlongInput(t *testing.T) {
	a, err := New("", time.Hour)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := a.SetPassword(strings.Repeat("a", MaxPasswordBytes+1)); err == nil {
		t.Fatal("SetPassword should refuse a password bcrypt cannot hash")
	}
	if a.Enabled() {
		t.Error("a refused password must not become the active one")
	}
}
