// Package auth provides password protection for the web UI and API.
//
// A fresh install has no password. When the server is reachable from the
// network, the first visitor is sent to a setup page and must create one before
// anything else is shown. After that, a login form exchanges the password for a
// session cookie, and every other request must present that cookie.
//
// When the server is bound to loopback only, or the operator has explicitly
// opted out, no password is required at all.
package auth

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"
)

const (
	// SessionCookie is the name of the cookie holding the session token.
	SessionCookie = "orangutan_session"

	// These paths are reachable without a session.
	LoginPath  = "/login"
	LogoutPath = "/logout"
	SetupPath  = "/setup"

	// MinPasswordLength is the shortest password the setup page accepts.
	MinPasswordLength = 8

	// MaxPasswordBytes is bcrypt's own limit. Anything longer is rejected by
	// the library, so it is checked here in order to explain it in plain terms
	// rather than surfacing "bcrypt: password length exceeds 72 bytes".
	MaxPasswordBytes = 72

	// maxAttempts is how many failed logins an address may make within
	// attemptWindow before it is locked out for the rest of the window.
	maxAttempts    = 5
	attemptWindow  = 15 * time.Minute
	sessionIDBytes = 32
)

// Authenticator guards HTTP handlers with a password.
//
// The zero value is not usable; construct one with New.
type Authenticator struct {
	mu sync.Mutex

	// hash is the bcrypt hash of the current password. Empty means no password
	// has been set yet.
	hash []byte

	// setupRequired means that, with no password set, visitors should be sent
	// to the setup page rather than let straight through.
	setupRequired bool

	// sessionTTL is how long a session stays valid after login.
	sessionTTL time.Duration

	sessions map[string]time.Time // token -> expiry
	attempts map[string]*attemptRecord
}

type attemptRecord struct {
	count int
	// resets is when the count returns to zero.
	resets time.Time
}

// New creates an Authenticator.
//
// password may be either a plaintext password or an existing bcrypt hash, so
// callers can pass a stored hash straight through. An empty password leaves the
// Authenticator unconfigured, ready for either setup or open access.
func New(password string, sessionTTL time.Duration) (*Authenticator, error) {
	a := &Authenticator{
		sessionTTL: sessionTTL,
		sessions:   make(map[string]time.Time),
		attempts:   make(map[string]*attemptRecord),
	}

	if password == "" {
		return a, nil
	}

	hash, err := hashOrPassthrough(password)
	if err != nil {
		return nil, err
	}
	a.hash = hash
	return a, nil
}

// hashOrPassthrough returns password unchanged if it is already a bcrypt hash,
// and otherwise hashes it.
func hashOrPassthrough(password string) ([]byte, error) {
	if isBcryptHash(password) {
		return []byte(password), nil
	}
	return bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
}

// isBcryptHash reports whether s looks like a bcrypt hash rather than a
// plaintext password.
func isBcryptHash(s string) bool {
	return strings.HasPrefix(s, "$2a$") ||
		strings.HasPrefix(s, "$2b$") ||
		strings.HasPrefix(s, "$2y$")
}

// Enabled reports whether a password is currently set.
func (a *Authenticator) Enabled() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return len(a.hash) > 0
}

// SetSetupRequired controls what happens when no password is set: either
// visitors are sent to the setup page, or they are let through untouched.
func (a *Authenticator) SetSetupRequired(required bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.setupRequired = required
}

// NeedsSetup reports whether the user still has to create a password.
func (a *Authenticator) NeedsSetup() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.setupRequired && len(a.hash) == 0
}

// SetPassword establishes a new password and returns its hash so the caller can
// persist it. It is used both by first-run setup and by later password changes.
func (a *Authenticator) SetPassword(plaintext string) (string, error) {
	if err := ValidatePassword(plaintext); err != nil {
		return "", err
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(plaintext), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}

	a.mu.Lock()
	defer a.mu.Unlock()
	a.hash = hash
	// Any session issued while there was no password predates this one and
	// should not survive it.
	a.sessions = make(map[string]time.Time)

	return string(hash), nil
}

// ValidatePassword reports whether a proposed password is acceptable.
func ValidatePassword(password string) error {
	if len(password) < MinPasswordLength {
		return fmt.Errorf("password must be at least %d characters", MinPasswordLength)
	}
	// len() counts bytes, which is what bcrypt limits. Accented and non-Latin
	// characters take more than one byte each, so say so.
	if len(password) > MaxPasswordBytes {
		return fmt.Errorf(
			"password is too long: the limit is %d characters, and accented or non-Latin characters count as more than one",
			MaxPasswordBytes)
	}
	return nil
}

// Middleware wraps next so that requests without a valid session are turned
// away.
//
// The decision is made per request rather than once at startup, because a
// password can come into existence partway through the process's life, when the
// user completes setup.
func (a *Authenticator) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case a.NeedsSetup():
			a.reject(w, r, SetupPath, "setup required")

		case !a.Enabled():
			// No password, and none required. Open access.
			next.ServeHTTP(w, r)

		case a.authenticated(r):
			next.ServeHTTP(w, r)

		default:
			a.reject(w, r, LoginPath, "authentication required")
		}
	})
}

// reject turns away an unauthenticated request: JSON for the API, a redirect
// for anything a browser is displaying.
func (a *Authenticator) reject(w http.ResponseWriter, r *http.Request, redirectTo, reason string) {
	if strings.HasPrefix(r.URL.Path, "/api/") {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]any{
			"success": false,
			"error":   reason,
		})
		return
	}

	http.Redirect(w, r, redirectTo, http.StatusSeeOther)
}

// authenticated reports whether r carries a valid, unexpired session cookie.
func (a *Authenticator) authenticated(r *http.Request) bool {
	cookie, err := r.Cookie(SessionCookie)
	if err != nil || cookie.Value == "" {
		return false
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	expiry, ok := a.sessions[cookie.Value]
	if !ok {
		return false
	}
	if time.Now().After(expiry) {
		delete(a.sessions, cookie.Value)
		return false
	}
	return true
}

// Login verifies password and, on success, returns a new session token.
// The bool reports whether the attempt succeeded.
func (a *Authenticator) Login(remoteAddr, password string) (string, bool) {
	if !a.Enabled() {
		return "", false
	}

	key := clientKey(remoteAddr)
	if a.lockedOut(key) {
		return "", false
	}

	a.mu.Lock()
	hash := append([]byte(nil), a.hash...)
	a.mu.Unlock()

	if bcrypt.CompareHashAndPassword(hash, []byte(password)) != nil {
		a.recordFailure(key)
		return "", false
	}

	a.mu.Lock()
	defer a.mu.Unlock()
	delete(a.attempts, key)
	a.pruneSessionsLocked()

	token, err := newToken()
	if err != nil {
		return "", false
	}
	a.sessions[token] = time.Now().Add(a.sessionTTL)
	return token, true
}

// StartSession issues a session without checking a password. It exists so that
// completing setup signs the user straight in rather than bouncing them to a
// login form for the password they just chose.
func (a *Authenticator) StartSession() (string, bool) {
	token, err := newToken()
	if err != nil {
		return "", false
	}

	a.mu.Lock()
	defer a.mu.Unlock()
	a.pruneSessionsLocked()
	a.sessions[token] = time.Now().Add(a.sessionTTL)
	return token, true
}

// Logout invalidates the session carried by r, if any.
func (a *Authenticator) Logout(r *http.Request) {
	cookie, err := r.Cookie(SessionCookie)
	if err != nil || cookie.Value == "" {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	delete(a.sessions, cookie.Value)
}

// SetCookie writes the session cookie for token onto w.
func (a *Authenticator) SetCookie(w http.ResponseWriter, r *http.Request, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookie,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   r.TLS != nil,
		MaxAge:   int(a.sessionTTL.Seconds()),
	})
}

// ClearCookie expires the session cookie on w.
func (a *Authenticator) ClearCookie(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookie,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   r.TLS != nil,
		MaxAge:   -1,
	})
}

// LockedOut reports whether the given address has failed too many logins
// recently. Exposed so the login page can explain the wait.
func (a *Authenticator) LockedOut(remoteAddr string) bool {
	return a.lockedOut(clientKey(remoteAddr))
}

func (a *Authenticator) lockedOut(key string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()

	rec, ok := a.attempts[key]
	if !ok {
		return false
	}
	if time.Now().After(rec.resets) {
		delete(a.attempts, key)
		return false
	}
	return rec.count >= maxAttempts
}

func (a *Authenticator) recordFailure(key string) {
	a.mu.Lock()
	defer a.mu.Unlock()

	now := time.Now()
	rec, ok := a.attempts[key]
	if !ok || now.After(rec.resets) {
		a.attempts[key] = &attemptRecord{count: 1, resets: now.Add(attemptWindow)}
		return
	}
	rec.count++
}

// pruneSessionsLocked drops expired sessions so the map does not grow without
// bound on a long-running server. Callers must hold a.mu.
func (a *Authenticator) pruneSessionsLocked() {
	now := time.Now()
	for token, expiry := range a.sessions {
		if now.After(expiry) {
			delete(a.sessions, token)
		}
	}
}

// clientKey reduces a remote address to the host portion, so that a client
// cannot dodge the login rate limit by using a fresh source port.
func clientKey(remoteAddr string) string {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		return remoteAddr
	}
	return host
}

// newToken returns a cryptographically random session token.
func newToken() (string, error) {
	buf := make([]byte, sessionIDBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

// HashPassword returns a bcrypt hash of password, for users who would rather
// not store plaintext in their config file.
func HashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(hash), err
}

// LoadHash reads a stored password hash written by SaveHash.
//
// A missing file is not an error: it simply means no password has been set.
func LoadHash(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}

	hash := strings.TrimSpace(string(data))
	if !isBcryptHash(hash) {
		return ""
	}
	return hash
}

// SaveHash writes a password hash to disk, readable only by its owner.
func SaveHash(path, hash string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("could not create the directory for the password file: %w", err)
	}
	if err := os.WriteFile(path, []byte(hash+"\n"), 0o600); err != nil {
		return fmt.Errorf("could not save the password: %w", err)
	}
	return nil
}
