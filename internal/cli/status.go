package cli

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/291-Group/LAN-Orangutan/internal/network"
	"github.com/291-Group/LAN-Orangutan/internal/scanner"
	"github.com/291-Group/LAN-Orangutan/internal/types"
)

var statusFormat string

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show system and service status",
	Long: `Show the scanning tools found, the stored devices, the networks in scope,
Tailscale, and the server settings. With --format json the "config" object
is the one GET /api/status returns, alongside the rest.`,
	Args: cobra.NoArgs,
	RunE: runStatus,
}

func init() {
	statusCmd.Flags().StringVar(&statusFormat, "format", "table", "Output format (table, json)")
}

// toolStatus is one scanning tool as "status" reports it.
type toolStatus struct {
	Name    string `json:"name"`
	Found   bool   `json:"found"`
	Path    string `json:"path,omitempty"`
	Version string `json:"version,omitempty"`
	// Note explains a tool the app can use even though it is not on PATH.
	Note string `json:"note,omitempty"`
}

func runStatus(cmd *cobra.Command, args []string) error {
	format, err := outputFormat(statusFormat, "table", "json")
	if err != nil {
		return err
	}

	// Tailscale is looked up by the app in its install locations as well as
	// PATH, so it can be connected while "tailscale" is not a command in this
	// shell. Work that out once, so the tools line and the Tailscale section
	// cannot contradict each other.
	ts := network.GetTailscaleStatus()
	tools := []toolStatus{checkTool("nmap")}
	// arp-scan has no Windows build, so listing it there only ever reports a
	// tool the user cannot install as missing.
	if scanner.ArpScanAvailable() {
		tools = append(tools, checkTool("arp-scan"))
	}
	tsTool := checkTool("tailscale")
	if !tsTool.Found && ts.Installed {
		tsTool.Found = true
		tsTool.Version = ts.Version
		tsTool.Note = "not on PATH; the app uses the copy inside the Tailscale app"
	}
	tools = append(tools, tsTool)

	store, storeErr := openStore()
	var stats types.DeviceStats
	if storeErr == nil {
		stats = store.GetStats()
	}

	networks, netErr := network.DetectNetworks()
	if netErr == nil && store != nil {
		networks = network.WithConfigured(networks, networkFilter(store, cfg))
	}

	if format == "json" {
		out := map[string]any{
			"timestamp": time.Now().Format(time.RFC3339),
			"system": map[string]string{
				"go_version": runtime.Version(),
				"os":         runtime.GOOS,
				"arch":       runtime.GOARCH,
			},
			"tools":     tools,
			"tailscale": ts,
			"config": map[string]any{
				"port":        cfg.Server.Port,
				"bind":        cfg.Server.BindAddress,
				"api_enabled": cfg.Server.EnableAPI,
			},
		}
		if storeErr == nil {
			out["storage"] = map[string]any{
				"devices":  stats,
				"data_dir": cfg.Storage.DataDir,
			}
		} else {
			out["storage"] = map[string]any{"error": storeErr.Error()}
		}
		if netErr == nil {
			if networks == nil {
				networks = []types.Network{}
			}
			out["networks"] = networks
		} else {
			out["networks"] = map[string]any{"error": netErr.Error()}
		}
		return printJSON(out)
	}

	fmt.Println("=== LAN Orangutan Status ===")
	fmt.Println()

	// System info
	fmt.Println("System:")
	fmt.Printf("  Go Version: %s\n", runtime.Version())
	fmt.Printf("  OS/Arch: %s/%s\n", runtime.GOOS, runtime.GOARCH)

	fmt.Println()
	fmt.Println("Tools:")
	for _, t := range tools {
		switch {
		case !t.Found:
			fmt.Printf("  %s: not found\n", t.Name)
		case t.Note != "":
			fmt.Printf("  %s: %s (%s)\n", t.Name, orDash(t.Version), t.Note)
		case t.Version != "":
			fmt.Printf("  %s: %s (%s)\n", t.Name, t.Version, t.Path)
		default:
			fmt.Printf("  %s: %s\n", t.Name, t.Path)
		}
	}

	// Storage
	fmt.Println()
	fmt.Println("Storage:")
	if storeErr != nil {
		fmt.Printf("  Error: %v\n", storeErr)
	} else {
		fmt.Printf("  Devices: %d total (%d online, %d offline)\n", stats.Total, stats.Online, stats.Offline)
		fmt.Printf("  Data directory: %s\n", cfg.Storage.DataDir)
	}

	// Networks
	fmt.Println()
	fmt.Println("Networks:")
	if netErr != nil {
		fmt.Printf("  Error detecting: %v\n", netErr)
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
	fmt.Printf("  API enabled: %s\n", yesNo(cfg.Server.EnableAPI))

	return nil
}

func checkTool(name string) toolStatus {
	path, err := toolPath(name)
	if err != nil {
		return toolStatus{Name: name}
	}
	return toolStatus{Name: name, Found: true, Path: path, Version: getToolVersion(name)}
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
