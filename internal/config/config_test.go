package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// writeConfig writes body to a temporary config file and returns its path.
func writeConfig(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.ini")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("writing config: %v", err)
	}
	return path
}

func TestDefaultIsReachableButRequiresSetup(t *testing.T) {
	cfg := Default()

	// Reachable from the network, because that is how this is normally used.
	if cfg.IsLoopbackBind() {
		t.Fatalf("default bind address %q should accept connections from the network", cfg.Server.BindAddress)
	}
	if cfg.Server.Password != "" {
		t.Fatal("no password should be set by default")
	}
	if cfg.Server.AllowInsecure {
		t.Fatal("password protection must not be skipped by default")
	}

	// Safety comes from setup, not from being unreachable.
	if !cfg.RequiresSetup() {
		t.Fatal("a fresh install reachable from the network must require setup")
	}
}

func TestMissingFileFallsBackToDefaults(t *testing.T) {
	cfg, err := Load(filepath.Join(t.TempDir(), "does-not-exist.ini"))
	if err != nil {
		t.Fatalf("a missing config file should not be an error: %v", err)
	}
	if !cfg.RequiresSetup() {
		t.Fatal("a missing config file should leave the setup requirement in place")
	}
}

func TestLoadServerSettings(t *testing.T) {
	path := writeConfig(t, `
[server]
port = 8080
bind_address = 0.0.0.0
password = hunter2
session_hours = 12
allow_insecure = true
`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if cfg.Server.Port != 8080 {
		t.Errorf("port = %d, want 8080", cfg.Server.Port)
	}
	if cfg.Server.BindAddress != "0.0.0.0" {
		t.Errorf("bind_address = %q, want 0.0.0.0", cfg.Server.BindAddress)
	}
	if cfg.Server.Password != "hunter2" {
		t.Errorf("password = %q, want hunter2", cfg.Server.Password)
	}
	if cfg.Server.SessionHours != 12 {
		t.Errorf("session_hours = %d, want 12", cfg.Server.SessionHours)
	}
	if !cfg.Server.AllowInsecure {
		t.Error("allow_insecure should be true")
	}
}

func TestIsLoopbackBind(t *testing.T) {
	tests := []struct {
		addr string
		want bool
	}{
		{"127.0.0.1", true},
		{"127.0.0.53", true},
		{"localhost", true},
		{"LOCALHOST", true},
		{"::1", true},
		{"[::1]", true},
		{"0.0.0.0", false},
		{"192.168.1.10", false},
		{"::", false},
		{"", false}, // empty means every interface
		{"not-an-address", false},
	}

	for _, tt := range tests {
		cfg := Default()
		cfg.Server.BindAddress = tt.addr
		if got := cfg.IsLoopbackBind(); got != tt.want {
			t.Errorf("IsLoopbackBind(%q) = %v, want %v", tt.addr, got, tt.want)
		}
	}
}

func TestRequiresSetupWhenReachableWithNoPassword(t *testing.T) {
	cfg := Default()
	cfg.Server.BindAddress = "0.0.0.0"

	if !cfg.RequiresSetup() {
		t.Fatal("a network-reachable install with no password must require setup")
	}
}

func TestRequiresSetupWithEmptyBindAddress(t *testing.T) {
	// An empty bind address listens on every interface, so it is just as
	// exposed as 0.0.0.0 and must be caught too.
	cfg := Default()
	cfg.Server.BindAddress = ""

	if !cfg.RequiresSetup() {
		t.Fatal("an empty bind address with no password must require setup")
	}
}

func TestNoSetupWhenPasswordAlreadySet(t *testing.T) {
	cfg := Default()
	cfg.Server.BindAddress = "0.0.0.0"
	cfg.Server.Password = "hunter2"

	if cfg.RequiresSetup() {
		t.Fatal("an existing password means there is nothing to set up")
	}
}

func TestNoSetupWhenLoopbackOnly(t *testing.T) {
	// Nobody else can reach it, so a password would be friction with no gain.
	cfg := Default()
	cfg.Server.BindAddress = "127.0.0.1"

	if cfg.RequiresSetup() {
		t.Fatal("a loopback-only install should not demand a password")
	}
}

func TestNoSetupWhenExplicitlyOptedOut(t *testing.T) {
	cfg := Default()
	cfg.Server.BindAddress = "0.0.0.0"
	cfg.Server.AllowInsecure = true

	if cfg.RequiresSetup() {
		t.Fatal("an explicit opt-out should be honoured")
	}
}

func TestPasswordFileLivesBesideTheData(t *testing.T) {
	cfg := Default()
	cfg.Storage.DataDir = "/tmp/example"

	if got, want := cfg.PasswordFile(), filepath.Join("/tmp/example", "auth"); got != want {
		t.Errorf("PasswordFile() = %q, want %q", got, want)
	}
}

func TestEnvOverridesConfigFile(t *testing.T) {
	path := writeConfig(t, `
[server]
port = 8080
bind_address = 127.0.0.1
password = from-file
`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	t.Setenv("ORANGUTAN_PORT", "9999")
	t.Setenv("ORANGUTAN_BIND_ADDRESS", "0.0.0.0")
	t.Setenv("ORANGUTAN_PASSWORD", "from-env")
	t.Setenv("ORANGUTAN_ALLOW_INSECURE", "true")
	cfg.ApplyEnv()

	if cfg.Server.Port != 9999 {
		t.Errorf("port = %d, want the environment value 9999", cfg.Server.Port)
	}
	if cfg.Server.BindAddress != "0.0.0.0" {
		t.Errorf("bind_address = %q, want the environment value", cfg.Server.BindAddress)
	}
	if cfg.Server.Password != "from-env" {
		t.Errorf("password = %q, want the environment value", cfg.Server.Password)
	}
	if !cfg.Server.AllowInsecure {
		t.Error("allow_insecure should come from the environment")
	}
}

func TestEnvLeavesUnsetValuesAlone(t *testing.T) {
	cfg := Default()
	original := cfg.Server.Port

	cfg.ApplyEnv() // nothing set

	if cfg.Server.Port != original {
		t.Errorf("port changed to %d with no environment variable set", cfg.Server.Port)
	}
	if cfg.Server.BindAddress != Default().Server.BindAddress {
		t.Error("bind address should be untouched with no environment variable set")
	}
}

func TestPasswordFileIsRead(t *testing.T) {
	path := filepath.Join(t.TempDir(), "secret")
	// Trailing newline is normal in a secret file and must be trimmed.
	if err := os.WriteFile(path, []byte("file-secret\n"), 0o600); err != nil {
		t.Fatalf("writing secret: %v", err)
	}

	cfg := Default()
	t.Setenv("ORANGUTAN_PASSWORD_FILE", path)
	cfg.ApplyEnv()

	if cfg.Server.Password != "file-secret" {
		t.Errorf("password = %q, want the trimmed file contents", cfg.Server.Password)
	}
}

func TestPasswordFileWinsOverPasswordVar(t *testing.T) {
	path := filepath.Join(t.TempDir(), "secret")
	if err := os.WriteFile(path, []byte("file-secret"), 0o600); err != nil {
		t.Fatalf("writing secret: %v", err)
	}

	cfg := Default()
	t.Setenv("ORANGUTAN_PASSWORD", "env-secret")
	t.Setenv("ORANGUTAN_PASSWORD_FILE", path)
	cfg.ApplyEnv()

	if cfg.Server.Password != "file-secret" {
		t.Errorf("password = %q, want the file to take precedence", cfg.Server.Password)
	}
}

func TestMissingPasswordFileIsIgnored(t *testing.T) {
	cfg := Default()
	t.Setenv("ORANGUTAN_PASSWORD", "env-secret")
	t.Setenv("ORANGUTAN_PASSWORD_FILE", filepath.Join(t.TempDir(), "nope"))
	cfg.ApplyEnv()

	// A missing file should not wipe out a password supplied another way.
	if cfg.Server.Password != "env-secret" {
		t.Errorf("password = %q, want the environment value kept", cfg.Server.Password)
	}
}

func TestSessionTTL(t *testing.T) {
	cfg := Default()
	if got := cfg.SessionTTL(); got != 7*24*time.Hour {
		t.Errorf("default SessionTTL = %v, want one week", got)
	}

	cfg.Server.SessionHours = 3
	if got := cfg.SessionTTL(); got != 3*time.Hour {
		t.Errorf("SessionTTL = %v, want 3h", got)
	}

	// Nonsense values fall back to the default rather than expiring instantly.
	cfg.Server.SessionHours = 0
	if got := cfg.SessionTTL(); got != 7*24*time.Hour {
		t.Errorf("SessionTTL with 0 hours = %v, want the default", got)
	}
	cfg.Server.SessionHours = -5
	if got := cfg.SessionTTL(); got != 7*24*time.Hour {
		t.Errorf("SessionTTL with a negative value = %v, want the default", got)
	}
}

func TestCommentsAndBlankLinesIgnored(t *testing.T) {
	path := writeConfig(t, `
# a comment
; another comment

[server]
port = 4242
`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Server.Port != 4242 {
		t.Errorf("port = %d, want 4242", cfg.Server.Port)
	}
}
