package network

import (
	"encoding/json"
	"net"
	"os/exec"
	"runtime"
	"strings"

	"github.com/291-Group/LAN-Orangutan/internal/types"
)

// tailscaleStatusJSON represents the JSON output from `tailscale status --json`
type tailscaleStatusJSON struct {
	Version        string `json:"Version"`
	BackendState   string `json:"BackendState"`
	CurrentTailnet struct {
		Name string `json:"Name"`
	} `json:"CurrentTailnet"`
	Self struct {
		DNSName      string   `json:"DNSName"`
		HostName     string   `json:"HostName"`
		OS           string   `json:"OS"`
		TailscaleIPs []string `json:"TailscaleIPs"`
	} `json:"Self"`
	Peer           map[string]tailscalePeerJSON `json:"Peer"`
	ExitNodeStatus struct {
		ID string `json:"ID"`
	} `json:"ExitNodeStatus"`
}

// tailscalePeerJSON represents one peer in `tailscale status --json`
type tailscalePeerJSON struct {
	TailscaleIPs []string `json:"TailscaleIPs"`
	HostName     string   `json:"HostName"`
	DNSName      string   `json:"DNSName"`
	OS           string   `json:"OS"`
	Online       bool     `json:"Online"`
}

// shortName returns the peer's friendly name, preferring the hostname and
// falling back to the first label of its DNS name.
func (p tailscalePeerJSON) shortName() string {
	if p.HostName != "" {
		return p.HostName
	}
	name := strings.TrimSuffix(p.DNSName, ".")
	if name == "" {
		return ""
	}
	return strings.Split(name, ".")[0]
}

// ipv4 returns the peer's Tailscale IPv4 address, or "" if it has none.
func (p tailscalePeerJSON) ipv4() string {
	for _, addr := range p.TailscaleIPs {
		if ip := net.ParseIP(addr); ip != nil && ip.To4() != nil {
			return addr
		}
	}
	return ""
}

// toDevice converts a peer into a device record.
func (p tailscalePeerJSON) toDevice() types.Device {
	return types.Device{
		IP:       p.ipv4(),
		Hostname: p.shortName(),
		// Tailscale peers have no MAC address to look a hardware vendor up
		// from, but it does report each peer's operating system, which is the
		// most useful thing to identify it by.
		Vendor: p.OS,
	}
}

// GetTailscaleDevices returns the devices currently reachable over Tailscale,
// including this machine.
//
// Tailscale peers cannot be found by scanning: every node sits on its own /32,
// so there is no subnet to sweep. Tailscale already knows the whole tailnet, so
// the peers are read from it directly instead.
//
// Only peers that are currently online are returned. Offline peers are known to
// Tailscale but reporting them as discovered would mark them as seen just now.
func GetTailscaleDevices() []types.Device {
	tailscaleBin := findTailscaleBinary()
	if tailscaleBin == "" {
		return nil
	}

	output, err := runCommand(tailscaleBin, "status", "--json")
	if err != nil {
		return nil
	}

	var tsStatus tailscaleStatusJSON
	if err := json.Unmarshal(output, &tsStatus); err != nil {
		return nil
	}

	// A stopped or logged-out Tailscale still lists the peers it saw last time,
	// none of which are reachable now.
	if tsStatus.BackendState != "Running" {
		return nil
	}

	var devices []types.Device

	self := tailscalePeerJSON{
		TailscaleIPs: tsStatus.Self.TailscaleIPs,
		HostName:     tsStatus.Self.HostName,
		DNSName:      tsStatus.Self.DNSName,
		OS:           tsStatus.Self.OS,
	}
	if d := self.toDevice(); d.IP != "" {
		devices = append(devices, d)
	}

	for _, peer := range tsStatus.Peer {
		if !peer.Online {
			continue
		}
		if d := peer.toDevice(); d.IP != "" {
			devices = append(devices, d)
		}
	}

	return devices
}

// findTailscaleBinary finds the tailscale CLI binary path
func findTailscaleBinary() string {
	// First check if it's in PATH
	if path, err := exec.LookPath("tailscale"); err == nil {
		return path
	}

	// Check platform-specific locations
	switch runtime.GOOS {
	case "darwin":
		// macOS: Tailscale app installs CLI here
		paths := []string{
			"/Applications/Tailscale.app/Contents/MacOS/Tailscale",
			"/usr/local/bin/tailscale",
		}
		for _, p := range paths {
			if _, err := exec.LookPath(p); err == nil {
				return p
			}
		}
	case "windows":
		paths := []string{
			`C:\Program Files\Tailscale\tailscale.exe`,
			`C:\Program Files (x86)\Tailscale\tailscale.exe`,
		}
		for _, p := range paths {
			if _, err := exec.LookPath(p); err == nil {
				return p
			}
		}
	}

	return ""
}

// GetTailscaleStatus returns the current Tailscale status
func GetTailscaleStatus() types.TailscaleStatus {
	status := types.TailscaleStatus{}

	// Check if tailscale is installed
	tailscaleBin := findTailscaleBinary()
	if tailscaleBin == "" {
		status.Installed = false
		return status
	}
	status.Installed = true

	// Get tailscale status
	output, err := runCommand(tailscaleBin, "status", "--json")
	if err != nil {
		// Tailscale is installed but not running, not connected, or wedged.
		status.Running = false
		return status
	}

	var tsStatus tailscaleStatusJSON
	if err := json.Unmarshal(output, &tsStatus); err != nil {
		status.Running = false
		return status
	}

	// The daemon answered, but that only means Tailscale is installed and its
	// service is up. The user may still be logged out or have stopped it, so
	// the backend state decides whether it is actually connected.
	status.Running = true
	status.Version = tsStatus.Version
	status.BackendState = tsStatus.BackendState
	status.Connected = tsStatus.BackendState == "Running"
	status.TailnetName = tsStatus.CurrentTailnet.Name
	status.PeerCount = len(tsStatus.Peer)

	// Get self info
	if len(tsStatus.Self.TailscaleIPs) > 0 {
		status.SelfIP = tsStatus.Self.TailscaleIPs[0]
	}
	if tsStatus.Self.DNSName != "" {
		// Remove trailing dot and tailnet suffix
		hostname := strings.TrimSuffix(tsStatus.Self.DNSName, ".")
		parts := strings.Split(hostname, ".")
		if len(parts) > 0 {
			status.SelfHostname = parts[0]
		}
	}

	// Check for exit node
	if tsStatus.ExitNodeStatus.ID != "" {
		status.ExitNode = tsStatus.ExitNodeStatus.ID
	}

	return status
}

// IsTailscaleConnected returns true if Tailscale is connected
func IsTailscaleConnected() bool {
	return GetTailscaleStatus().Connected
}
