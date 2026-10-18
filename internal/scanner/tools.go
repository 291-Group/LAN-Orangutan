package scanner

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// ErrNmapNotFound reports that no nmap executable could be located, as opposed
// to one that was found and then failed to run. The two need different advice,
// so callers distinguish them with errors.Is.
var ErrNmapNotFound = errors.New("nmap not found")

// ArpScanAvailable reports whether arp-scan can exist on this platform at all.
//
// arp-scan has no Windows build. Treating it as a universal fallback is what
// made every failed Windows scan report "arp-scan not found", sending users
// after a tool they cannot install no matter what the real problem was.
func ArpScanAvailable() bool {
	return runtime.GOOS != "windows"
}

// NmapPath locates the nmap executable.
//
// exec.LookPath searches PATH and nothing else. On Windows the installer does
// add its directory to PATH, but a shell or a service that was already running
// when nmap was installed keeps the environment it started with, so a perfectly
// good install looks missing until the session is restarted. Checking the
// standard install locations turns that into a scan that simply works. The same
// applies to a macOS Homebrew install invoked from a launchd service with a
// minimal PATH.
func NmapPath() (string, error) {
	if path, err := exec.LookPath("nmap"); err == nil {
		return path, nil
	}
	for _, candidate := range nmapFallbackPaths() {
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate, nil
		}
	}
	return "", ErrNmapNotFound
}

// nmapFallbackPaths lists the usual install locations to try when nmap is not
// on PATH, most likely first.
func nmapFallbackPaths() []string {
	switch runtime.GOOS {
	case "windows":
		var paths []string
		// The 32-bit tree first: the Nmap installer is 32-bit and lands in
		// "Program Files (x86)" on every 64-bit Windows.
		for _, env := range []string{"ProgramFiles(x86)", "ProgramFiles", "ProgramW6432"} {
			if base := os.Getenv(env); base != "" {
				paths = append(paths, filepath.Join(base, "Nmap", "nmap.exe"))
			}
		}
		return paths
	case "darwin":
		return []string{
			"/opt/homebrew/bin/nmap", // Apple silicon Homebrew
			"/usr/local/bin/nmap",    // Intel Homebrew
			"/opt/local/bin/nmap",    // MacPorts
		}
	default:
		return []string{
			"/usr/bin/nmap",
			"/usr/local/bin/nmap",
			"/usr/sbin/nmap",
		}
	}
}

// ScanFailureMessage explains why a scan could not run.
//
// Only the last error used to survive, so on Windows every failure read
// "arp-scan not found" regardless of the actual cause. nmap is the scanner we
// actually want, so its error leads, arp-scan's is appended only where arp-scan
// is a real option, and a platform-specific hint says what to do about it.
func ScanFailureMessage(nmapErr, arpErr error) string {
	var b strings.Builder
	b.WriteString(nmapErr.Error())
	if arpErr != nil {
		fmt.Fprintf(&b, "; %s", arpErr.Error())
	}
	if hint := nmapHint(nmapErr); hint != "" {
		fmt.Fprintf(&b, ". %s", hint)
	}
	return b.String()
}

// nmapHint returns the actionable advice for an nmap failure on this platform.
func nmapHint(nmapErr error) string {
	if errors.Is(nmapErr, ErrNmapNotFound) {
		switch runtime.GOOS {
		case "windows":
			return "Install nmap from nmap.org, including Npcap, then open a NEW Administrator " +
				"Command Prompt so it picks up the updated PATH. Run \"orangutan status\" to see " +
				"which tools LAN Orangutan can find"
		case "darwin":
			return "Install it with: brew install nmap"
		default:
			return "Install it with your package manager, for example: sudo apt install nmap"
		}
	}
	// nmap was found but did not complete.
	switch runtime.GOOS {
	case "windows":
		return "nmap is installed but did not complete. Check that Npcap was installed alongside " +
			"it, and run LAN Orangutan as Administrator"
	default:
		return "nmap is installed but did not complete. Try running as root, which nmap needs for " +
			"MAC addresses and vendor details"
	}
}
