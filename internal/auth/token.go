package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	// TokenPrefix marks a string as one of our API tokens. It is not a secret
	// and carries no entropy; it exists so that a token found in a script, a
	// log or a pasted config is immediately recognisable as belonging to this
	// application, and so that automated secret scanners can match on it.
	TokenPrefix = "lano_"

	// apiTokenBytes is the amount of randomness behind a token. 256 bits puts
	// guessing one permanently out of reach, which is what lets the stored form
	// be a plain SHA-256 rather than a slow password hash.
	apiTokenBytes = 32
)

// NewAPIToken returns a fresh API token and the hash to store for it.
//
// The token itself is never written anywhere. Only the hash is kept, so a copy
// of the data directory does not hand over API access, and the value is shown
// to the user once at the moment it is created.
func NewAPIToken() (token, hash string, err error) {
	buf := make([]byte, apiTokenBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", "", fmt.Errorf("could not generate an API token: %w", err)
	}
	token = TokenPrefix + base64.RawURLEncoding.EncodeToString(buf)
	return token, HashAPIToken(token), nil
}

// HashAPIToken returns the stored form of a token.
//
// SHA-256 rather than bcrypt on purpose. bcrypt is deliberately slow because a
// password is short and guessable, and that cost is worth paying once per
// login. A token carries 256 bits of randomness, so there is nothing to guess,
// and the same cost paid on every API request would be a self-inflicted denial
// of service on anything polling the endpoints.
func HashAPIToken(token string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(token)))
	return hex.EncodeToString(sum[:])
}

// SetTokenAccess turns API token authentication on or off. Off means a valid
// token is refused as if it did not exist; sessions are unaffected. It is how
// the enable_api setting is applied: the API routes stay up for the browser,
// but no script can call them.
func (a *Authenticator) SetTokenAccess(on bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.tokenAccessOff = !on
}

// TokenAccessEnabled reports whether API tokens are honoured.
func (a *Authenticator) TokenAccessEnabled() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return !a.tokenAccessOff
}

// SetAPITokenHash records the hash of the active API token. An empty hash turns
// token access off.
func (a *Authenticator) SetAPITokenHash(hash string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.apiTokenHash = strings.TrimSpace(hash)
}

// tokenFileStamp is the modification time and size of the token file, enough
// to notice that another process wrote or removed it.
type tokenFileStamp struct {
	exists bool
	mod    time.Time
	size   int64
}

// WatchAPITokenFile loads the token hash from path and keeps it in step with
// the file from then on, picking up a token created or revoked by another
// process (the CLI) without a restart.
func (a *Authenticator) WatchAPITokenFile(path string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.apiTokenFile = path
	a.apiTokenStamp = tokenFileStamp{}
	a.apiTokenHash = LoadAPITokenHash(path)
	a.apiTokenStamp = statTokenFile(path)
}

func statTokenFile(path string) tokenFileStamp {
	fi, err := os.Stat(path)
	if err != nil {
		return tokenFileStamp{}
	}
	return tokenFileStamp{exists: true, mod: fi.ModTime(), size: fi.Size()}
}

// refreshAPITokenLocked re-reads the watched token file if it has changed. A
// removed file means the token was revoked. Callers hold a.mu.
func (a *Authenticator) refreshAPITokenLocked() {
	if a.apiTokenFile == "" {
		return
	}
	st := statTokenFile(a.apiTokenFile)
	if st == a.apiTokenStamp {
		return
	}
	a.apiTokenStamp = st
	if !st.exists {
		a.apiTokenHash = ""
		return
	}
	a.apiTokenHash = LoadAPITokenHash(a.apiTokenFile)
}

// HasAPIToken reports whether an API token is currently active.
func (a *Authenticator) HasAPIToken() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.refreshAPITokenLocked()
	return a.apiTokenHash != ""
}

// apiTokenAuthenticated reports whether r presents the active API token.
//
// Only the API accepts a token. A browser page reached with a token in a header
// would have no session, so every link from it would bounce to the login form;
// worse, it would invite people to put a long-lived credential in a URL or a
// bookmark. Tokens are for programs, cookies are for browsers.
func (a *Authenticator) apiTokenAuthenticated(r *http.Request) bool {
	if !strings.HasPrefix(r.URL.Path, "/api/") {
		return false
	}

	presented := bearerToken(r)
	if presented == "" {
		return false
	}

	a.mu.Lock()
	a.refreshAPITokenLocked()
	stored := a.apiTokenHash
	off := a.tokenAccessOff
	a.mu.Unlock()
	if stored == "" || off {
		return false
	}

	// Constant time, so that a wrong token cannot be narrowed down by timing
	// how long the comparison took.
	return subtle.ConstantTimeCompare([]byte(HashAPIToken(presented)), []byte(stored)) == 1
}

// bearerToken pulls the credential out of an Authorization header.
//
// "Authorization: Bearer <token>" is the standard spelling and the one the
// documentation uses. A bare "Authorization: <token>" is also accepted, because
// it is the mistake everyone makes first and rejecting it teaches nothing.
func bearerToken(r *http.Request) string {
	header := strings.TrimSpace(r.Header.Get("Authorization"))
	if header == "" {
		return ""
	}
	if len(header) >= 7 && strings.EqualFold(header[:7], "bearer ") {
		return strings.TrimSpace(header[7:])
	}
	if strings.ContainsAny(header, " \t") {
		return ""
	}
	return header
}

// LoadAPITokenHash reads a stored token hash written by SaveAPITokenHash.
//
// A missing file is not an error: it means no token has been created.
func LoadAPITokenHash(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	hash := strings.TrimSpace(string(data))
	if len(hash) != sha256.Size*2 {
		return ""
	}
	if _, err := hex.DecodeString(hash); err != nil {
		return ""
	}
	return hash
}

// SaveAPITokenHash writes a token hash to disk, readable only by its owner. An
// empty hash removes the file, which is how a token is revoked.
func SaveAPITokenHash(path, hash string) error {
	if hash == "" {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("could not remove the API token: %w", err)
		}
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("could not create the directory for the API token: %w", err)
	}
	if err := os.WriteFile(path, []byte(hash+"\n"), 0o600); err != nil {
		return fmt.Errorf("could not save the API token: %w", err)
	}
	return nil
}
