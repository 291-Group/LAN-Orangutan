package network

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"time"
)

// tailscaleUpTimeout bounds how long we wait for `tailscale up`.
//
// A machine that is already signed in reconnects within a few seconds. One that
// needs to sign in prints a login URL almost immediately and then blocks
// waiting for the user to visit it, so this is only a safety net: the URL is
// detected and returned as soon as it appears, well before this fires.
const tailscaleUpTimeout = 20 * time.Second

// loginURLPattern matches the sign-in URL that `tailscale up` prints when the
// machine is not yet authenticated to a tailnet.
var loginURLPattern = regexp.MustCompile(`https://login\.tailscale\.com/\S+`)

// TailscaleActionResult describes the outcome of a connect attempt.
type TailscaleActionResult struct {
	// Connected is true when the machine is up on the tailnet.
	Connected bool `json:"connected"`
	// LoginURL is set when the machine must sign in before it can connect. The
	// user opens it themselves; the app does not automate browser auth.
	LoginURL string `json:"login_url,omitempty"`
}

// ConnectTailscale runs `tailscale up`.
//
// If the machine is already authenticated it simply reconnects. If it needs to
// sign in, Tailscale prints a login URL and then waits; that URL is captured
// and returned so the request does not block, and the user can open it and
// finish signing in.
func ConnectTailscale() (TailscaleActionResult, error) {
	bin := findTailscaleBinary()
	if bin == "" {
		return TailscaleActionResult{}, fmt.Errorf("tailscale is not installed")
	}

	ctx, cancel := context.WithTimeout(context.Background(), tailscaleUpTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, bin, "up")
	var out syncBuffer
	cmd.Stdout = &out
	cmd.Stderr = &out

	if err := cmd.Start(); err != nil {
		return TailscaleActionResult{}, fmt.Errorf("could not run tailscale: %w", err)
	}

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	// Poll the output. The login URL appears almost immediately when sign-in is
	// needed, so return it right away rather than waiting for the blocked
	// process to be killed at the timeout.
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case err := <-done:
			text := out.String()
			if url := loginURLPattern.FindString(text); url != "" {
				return TailscaleActionResult{LoginURL: url}, nil
			}
			if err != nil {
				return TailscaleActionResult{}, fmt.Errorf("tailscale up failed: %s", firstLine(text, err))
			}
			return TailscaleActionResult{Connected: IsTailscaleConnected()}, nil

		case <-ticker.C:
			if url := loginURLPattern.FindString(out.String()); url != "" {
				cancel() // stop waiting; the user must sign in via the URL
				<-done   // reap the process
				return TailscaleActionResult{LoginURL: url}, nil
			}
		}
	}
}

// DisconnectTailscale runs `tailscale down`, disconnecting the machine from its
// tailnet.
func DisconnectTailscale() error {
	bin := findTailscaleBinary()
	if bin == "" {
		return fmt.Errorf("tailscale is not installed")
	}

	ctx, cancel := context.WithTimeout(context.Background(), commandTimeout)
	defer cancel()

	out, err := exec.CommandContext(ctx, bin, "down").CombinedOutput()
	if err != nil {
		return fmt.Errorf("tailscale down failed: %s", firstLine(string(out), err))
	}
	return nil
}

// firstLine reduces command output to a single line for an error message,
// falling back to the error itself when there is no output.
func firstLine(output string, err error) string {
	output = strings.TrimSpace(output)
	if output == "" {
		if err != nil {
			return err.Error()
		}
		return "unknown error"
	}
	if i := strings.IndexByte(output, '\n'); i >= 0 {
		return strings.TrimSpace(output[:i])
	}
	return output
}

// syncBuffer is an io.Writer safe for the concurrent writes that os/exec makes
// from its output-copying goroutines while another goroutine reads it.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.Write(p)
}

func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.String()
}
