package cli

import (
	"net/http"
	"net/http/httptest"
	"testing"
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
