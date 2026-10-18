package cli

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/spf13/cobra"

	"github.com/291-Group/LAN-Orangutan/internal/network"
	"github.com/291-Group/LAN-Orangutan/internal/scanner"
	"github.com/291-Group/LAN-Orangutan/internal/storage"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show system and service status",
	RunE:  runStatus,
}

func runStatus(cmd *cobra.Command, args []string) error {
	fmt.Println("=== LAN Orangutan Status ===")
	fmt.Println()

	// System info
	fmt.Println("System:")
	fmt.Printf("  Go Version: %s\n", runtime.Version())
	fmt.Printf("  OS/Arch: %s/%s\n", runtime.GOOS, runtime.GOARCH)

	// Check tools
	fmt.Println()
	fmt.Println("Tools:")
	checkTool("nmap")
	// arp-scan has no Windows build, so listing it there only ever reports a
	// tool the user cannot install as missing.
	if scanner.ArpScanAvailable() {
		checkTool("arp-scan")
	}
	checkTool("tailscale")

	// Storage
	fmt.Println()
	fmt.Println("Storage:")
	store, err := storage.New(cfg.DevicesFile(), cfg.StateFile())
	if err != nil {
		fmt.Printf("  Error: %v\n", err)
	} else {
		stats := store.GetStats()
		fmt.Printf("  Devices: %d total (%d online, %d offline)\n", stats.Total, stats.Online, stats.Offline)
		fmt.Printf("  Data directory: %s\n", cfg.Storage.DataDir)
	}

	// Networks
	fmt.Println()
	fmt.Println("Networks:")
	networks, err := network.DetectNetworks()
	networks = network.WithConfigured(networks, networkFilter(store, cfg))
	if err != nil {
		fmt.Printf("  Error detecting: %v\n", err)
	} else {
		for _, n := range networks {
			flags := []string{}
			if n.IsTailscale {
				flags = append(flags, "tailscale")
			}
			if n.IsWireless {
				flags = append(flags, "wireless")
			}
			flagStr := ""
			if len(flags) > 0 {
				flagStr = " [" + strings.Join(flags, ", ") + "]"
			}
			fmt.Printf("  %s (%s)%s\n", n.CIDR, n.Interface, flagStr)
		}
	}

	// Tailscale
	fmt.Println()
	fmt.Println("Tailscale:")
	ts := network.GetTailscaleStatus()
	if !ts.Installed {
		fmt.Println("  Not installed")
	} else if !ts.Running {
		fmt.Println("  Installed but not running")
	} else {
		fmt.Printf("  Status: %s\n", ts.StatusLabel())
		// The IP and peer count are cached from the last session and go stale
		// once Tailscale stops, so only report them while connected.
		if ts.Connected {
			fmt.Printf("  IP: %s\n", ts.SelfIP)
			fmt.Printf("  Hostname: %s\n", ts.SelfHostname)
			fmt.Printf("  Peers: %d\n", ts.PeerCount)
		}
	}

	// Server config
	fmt.Println()
	fmt.Println("Server:")
	fmt.Printf("  Port: %d\n", cfg.Server.Port)
	fmt.Printf("  Bind: %s\n", cfg.Server.BindAddress)
	fmt.Printf("  API enabled: %v\n", cfg.Server.EnableAPI)

	return nil
}

func checkTool(name string) {
	path, err := toolPath(name)
	if err != nil {
		fmt.Printf("  %s: not found\n", name)
	} else {
		// Get version
		version := getToolVersion(name)
		if version != "" {
			fmt.Printf("  %s: %s (%s)\n", name, version, path)
		} else {
			fmt.Printf("  %s: %s\n", name, path)
		}
	}
}

// toolPath resolves a tool the same way the scanner does, so `status` and a
// real scan never disagree about whether nmap is present. For nmap that means
// checking the standard install locations as well as PATH.
func toolPath(name string) (string, error) {
	if name == "nmap" {
		return scanner.NmapPath()
	}
	return exec.LookPath(name)
}

func getToolVersion(name string) string {
	var cmd *exec.Cmd
	switch name {
	case "nmap":
		// Resolved path, not the bare name: this has to work in the same case
		// NmapPath exists for, where nmap is installed but not yet on PATH.
		bin, err := scanner.NmapPath()
		if err != nil {
			return ""
		}
		cmd = exec.Command(bin, "--version")
	case "arp-scan":
		cmd = exec.Command("arp-scan", "--version")
	case "tailscale":
		cmd = exec.Command("tailscale", "--version")
	default:
		return ""
	}

	output, err := cmd.Output()
	if err != nil {
		return ""
	}

	// Extract first line
	lines := strings.Split(string(output), "\n")
	if len(lines) > 0 {
		return strings.TrimSpace(lines[0])
	}
	return ""
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
