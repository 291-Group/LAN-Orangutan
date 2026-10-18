package cli

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/291-Group/LAN-Orangutan/internal/auth"
	"github.com/291-Group/LAN-Orangutan/internal/config"
)

func TestHostIsLoopback(t *testing.T) {
	cases := []struct {
		host string
		want bool
	}{
		{"localhost", true},
		{"localhost:291", true},
		{"127.0.0.1", true},
		{"127.0.0.1:291", true},
		{"127.0.0.5:8080", true}, // all of 127.0.0.0/8 is loopback
		{"[::1]", true},
		{"[::1]:291", true},
		{"LocalHost:291", true}, // case-insensitive
		{"evil.com", false},
		{"evil.com:291", false},
		{"192.168.1.50:291", false}, // a LAN address is not loopback
		{"attacker.example:80", false},
		{"", false},
	}
	for _, c := range cases {
		if got := hostIsLoopback(c.host); got != c.want {
			t.Errorf("hostIsLoopback(%q) = %v, want %v", c.host, got, c.want)
		}
	}
}

func TestGuardHost_LoopbackBind(t *testing.T) {
	reached := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reached = true
		w.WriteHeader(http.StatusOK)
	})
	guard := guardHost(true, next)

	// A loopback Host is allowed through.
	reached = false
	r := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:291/api/devices", nil)
	r.Host = "127.0.0.1:291"
	w := httptest.NewRecorder()
	guard.ServeHTTP(w, r)
	if !reached || w.Code != http.StatusOK {
		t.Fatalf("loopback Host should pass: reached=%v code=%d", reached, w.Code)
	}

	// A rebound attacker hostname is rejected before reaching the handler.
	reached = false
	r = httptest.NewRequest(http.MethodGet, "http://127.0.0.1:291/api/devices", nil)
	r.Host = "evil.example:291" // browser sends the attacker's domain
	w = httptest.NewRecorder()
	guard.ServeHTTP(w, r)
	if reached {
		t.Fatal("rebound Host reached the handler; DNS rebinding not blocked")
	}
	if w.Code != http.StatusForbidden {
		t.Fatalf("rebound Host should be 403, got %d", w.Code)
	}
}

func TestGuardHost_NetworkBindIsPassthrough(t *testing.T) {
	// On a network bind the Host is not checked: any hostname must pass, since
	// the server is legitimately reached by its LAN address or name.
	reached := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reached = true
		w.WriteHeader(http.StatusOK)
	})
	guard := guardHost(false, next)

	r := httptest.NewRequest(http.MethodGet, "http://myserver.lan:291/", nil)
	r.Host = "myserver.lan:291"
	w := httptest.NewRecorder()
	guard.ServeHTTP(w, r)
	if !reached || w.Code != http.StatusOK {
		t.Fatalf("network bind should pass any Host: reached=%v code=%d", reached, w.Code)
	}
}

// authPassword is the one place that decides whether a password gates the
// dashboard, so these pin down the precedence, and in particular that
// allow_insecure wins over a password stored by the setup page (GitHub #45).

func authTestConfig(t *testing.T) *config.Config {
	t.Helper()
	cfg := config.Default()
	cfg.Storage.DataDir = t.TempDir()
	return cfg
}

func storeSetupPassword(t *testing.T, cfg *config.Config) {
	t.Helper()
	// A real bcrypt hash of "correct horse", so LoadHash accepts it.
	const hash = "$2a$10$7EqJtq98hPqEX7fNZaFWoOa5aL1ScuGUHyvFuVvYQWD/2lmGhDHPa"
	if err := auth.SaveHash(cfg.PasswordFile(), hash); err != nil {
		t.Fatalf("SaveHash: %v", err)
	}
}

func TestAuthPasswordFallsBackToSetupPagePassword(t *testing.T) {
	cfg := authTestConfig(t)
	storeSetupPassword(t, cfg)

	got, ignored := authPassword(cfg)
	if got == "" {
		t.Fatal("with no configured password, the stored setup-page password should be used")
	}
	if ignored != "" {
		t.Errorf("nothing should be reported as ignored, got %q", ignored)
	}
}

func TestAuthPasswordConfiguredWinsOverStored(t *testing.T) {
	cfg := authTestConfig(t)
	storeSetupPassword(t, cfg)
	cfg.Server.Password = "from-config"

	got, _ := authPassword(cfg)
	if got != "from-config" {
		t.Fatalf("configured password should win over the stored one, got %q", got)
	}
}

// The #45 case: a password was created on first visit, then allow_insecure was
// set. The stored hash must not gate the dashboard any more.
func TestAllowInsecureIgnoresSetupPagePassword(t *testing.T) {
	cfg := authTestConfig(t)
	storeSetupPassword(t, cfg)
	cfg.Server.AllowInsecure = true

	got, ignored := authPassword(cfg)
	if got != "" {
		t.Fatalf("allow_insecure must disable the stored password, got %q", got)
	}
	if ignored == "" {
		t.Error("the banner should be told a stored password was ignored")
	}
}

func TestAllowInsecureIgnoresConfiguredPassword(t *testing.T) {
	cfg := authTestConfig(t)
	cfg.Server.Password = "from-config"
	cfg.Server.AllowInsecure = true

	got, ignored := authPassword(cfg)
	if got != "" {
		t.Fatalf("allow_insecure must disable the configured password, got %q", got)
	}
	if ignored == "" {
		t.Error("the banner should be told a configured password was ignored")
	}
}

func TestAllowInsecureWithNoPasswordReportsNothingIgnored(t *testing.T) {
	cfg := authTestConfig(t)
	cfg.Server.AllowInsecure = true

	if got, ignored := authPassword(cfg); got != "" || ignored != "" {
		t.Fatalf("got password %q, ignored %q; want both empty", got, ignored)
	}
}
