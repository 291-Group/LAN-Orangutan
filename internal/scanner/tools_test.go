package scanner

import (
	"errors"
	"runtime"
	"strings"
	"testing"
)

func TestArpScanAvailableIsFalseOnWindows(t *testing.T) {
	if got, want := ArpScanAvailable(), runtime.GOOS != "windows"; got != want {
		t.Fatalf("ArpScanAvailable() = %v, want %v on %s", got, want, runtime.GOOS)
	}
}

// The reported reason must be nmap's, not arp-scan's. Reporting only the second
// error is what made every Windows failure read "arp-scan not found".
func TestScanFailureMessageLeadsWithNmapError(t *testing.T) {
	msg := ScanFailureMessage(ErrNmapNotFound, errors.New("arp-scan not found"))
	if !strings.HasPrefix(msg, "nmap not found") {
		t.Fatalf("message should lead with the nmap error, got %q", msg)
	}
	if !strings.Contains(msg, "arp-scan not found") {
		t.Fatalf("message should still mention arp-scan when it was tried, got %q", msg)
	}
}

// On Windows arp-scan is never attempted, so its error must not appear at all.
func TestScanFailureMessageOmitsArpScanWhenNotTried(t *testing.T) {
	msg := ScanFailureMessage(ErrNmapNotFound, nil)
	if strings.Contains(msg, "arp-scan") {
		t.Fatalf("message must not mention arp-scan when it was not tried, got %q", msg)
	}
	if !strings.Contains(msg, "nmap not found") {
		t.Fatalf("message should name the real problem, got %q", msg)
	}
}

// A missing nmap and a broken nmap need different advice.
func TestScanFailureMessageHintDistinguishesMissingFromFailed(t *testing.T) {
	missing := ScanFailureMessage(ErrNmapNotFound, nil)
	failed := ScanFailureMessage(errors.New("nmap failed: exit status 1"), nil)
	if missing == failed {
		t.Fatal("a missing nmap and a failed nmap should not give the same advice")
	}
	if !strings.Contains(missing, "Install") {
		t.Fatalf("a missing nmap should say how to install it, got %q", missing)
	}
	if !strings.Contains(failed, "did not complete") {
		t.Fatalf("a failed nmap should say it ran but did not finish, got %q", failed)
	}
}

func TestNmapNotFoundIsIdentifiable(t *testing.T) {
	if !errors.Is(ErrNmapNotFound, ErrNmapNotFound) {
		t.Fatal("ErrNmapNotFound should match itself under errors.Is")
	}
}

// Every candidate must be an absolute path to a file named nmap, so a lookup
// can never be satisfied by something relative to the working directory.
func TestNmapFallbackPathsAreAbsolute(t *testing.T) {
	for _, p := range nmapFallbackPaths() {
		if !strings.HasPrefix(p, "/") && !strings.Contains(p, `:\`) {
			t.Errorf("fallback path %q is not absolute", p)
		}
		if !strings.Contains(strings.ToLower(p), "nmap") {
			t.Errorf("fallback path %q does not point at nmap", p)
		}
	}
}
