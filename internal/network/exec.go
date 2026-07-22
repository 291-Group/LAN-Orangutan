package network

import (
	"context"
	"os/exec"
	"time"
)

// commandTimeout bounds how long any helper process may run.
//
// These commands feed the dashboard, which renders synchronously. Without a
// bound, a wedged helper (a hung `tailscale status`, an unresponsive
// `resolvectl`) would hang the page rather than degrade it.
const commandTimeout = 5 * time.Second

// runCommand executes name with args and returns its standard output, giving up
// after commandTimeout.
//
// A timeout is reported as an error like any other failure, so callers keep
// their existing "no data available" behaviour instead of blocking.
func runCommand(name string, args ...string) ([]byte, error) {
	return runCommandTimeout(commandTimeout, name, args...)
}

// runCommandTimeout is runCommand with an explicit deadline, for callers that
// need to allow more or less time than the default.
func runCommandTimeout(timeout time.Duration, name string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	return exec.CommandContext(ctx, name, args...).Output()
}
