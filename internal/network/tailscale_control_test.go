package network

import (
	"strings"
	"testing"
)

func TestLoginURLPattern(t *testing.T) {
	// The shape of the message tailscale prints when a machine must sign in.
	sample := `To authenticate, visit:

	https://login.tailscale.com/a/1234567890abcdef

Success.`
	got := loginURLPattern.FindString(sample)
	want := "https://login.tailscale.com/a/1234567890abcdef"
	if got != want {
		t.Errorf("extracted %q, want %q", got, want)
	}
}

func TestLoginURLPatternAbsentWhenConnected(t *testing.T) {
	// A normal reconnect prints no login URL.
	for _, s := range []string{"", "Success.", "some other output"} {
		if url := loginURLPattern.FindString(s); url != "" {
			t.Errorf("found a login URL in %q where there is none: %q", s, url)
		}
	}
}

func TestFirstLine(t *testing.T) {
	tests := []struct {
		output string
		want   string
	}{
		{"single line", "single line"},
		{"first\nsecond\nthird", "first"},
		{"  padded  \nnext", "padded"},
		{"\n\nleading blanks\nmore", "leading blanks"},
	}
	for _, tt := range tests {
		if got := firstLine(tt.output, nil); got != tt.want {
			t.Errorf("firstLine(%q) = %q, want %q", tt.output, got, tt.want)
		}
	}
}

func TestFirstLineFallsBackToError(t *testing.T) {
	if got := firstLine("", errStub("boom")); got != "boom" {
		t.Errorf("with empty output, want the error text, got %q", got)
	}
	if got := firstLine("", nil); got != "unknown error" {
		t.Errorf("with no output and no error, got %q", got)
	}
}

type errStub string

func (e errStub) Error() string { return string(e) }

func TestControlRefusesWhenTailscaleMissing(t *testing.T) {
	// With no tailscale binary present, both actions must fail cleanly rather
	// than trying to run something.
	if findTailscaleBinary() != "" {
		t.Skip("tailscale is installed on this machine; skipping the missing-binary check")
	}

	if _, err := ConnectTailscale(); err == nil || !strings.Contains(err.Error(), "not installed") {
		t.Errorf("ConnectTailscale should report tailscale is not installed, got %v", err)
	}
	if err := DisconnectTailscale(); err == nil || !strings.Contains(err.Error(), "not installed") {
		t.Errorf("DisconnectTailscale should report tailscale is not installed, got %v", err)
	}
}

func TestSyncBufferConcurrentWrites(t *testing.T) {
	// The buffer is written by exec's copy goroutines while another goroutine
	// reads it, so writing and reading at once must not race. Run with -race.
	var b syncBuffer
	done := make(chan struct{})
	go func() {
		for i := 0; i < 1000; i++ {
			b.Write([]byte("x"))
		}
		close(done)
	}()
	for i := 0; i < 1000; i++ {
		_ = b.String()
	}
	<-done
	if len(b.String()) != 1000 {
		t.Errorf("expected 1000 bytes written, got %d", len(b.String()))
	}
}
