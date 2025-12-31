package network

import (
	"encoding/json"
	"os/exec"
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
		DNSName    string   `json:"DNSName"`
		TailscaleIPs []string `json:"TailscaleIPs"`
	} `json:"Self"`
	Peer map[string]struct {
		HostName string `json:"HostName"`
	} `json:"Peer"`
	ExitNodeStatus struct {
		ID string `json:"ID"`
	} `json:"ExitNodeStatus"`
}

// GetTailscaleStatus returns the current Tailscale status
func GetTailscaleStatus() types.TailscaleStatus {
	status := types.TailscaleStatus{}

	// Check if tailscale is installed
	if _, err := exec.LookPath("tailscale"); err != nil {
		status.Installed = false
		return status
	}
	status.Installed = true

	// Get tailscale status
	cmd := exec.Command("tailscale", "status", "--json")
	output, err := cmd.Output()
	if err != nil {
		// Tailscale is installed but not running or not connected
		status.Running = false
		return status
	}

	var tsStatus tailscaleStatusJSON
	if err := json.Unmarshal(output, &tsStatus); err != nil {
		status.Running = false
		return status
	}

	status.Running = true
	status.Version = tsStatus.Version
	status.BackendState = tsStatus.BackendState
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
	status := GetTailscaleStatus()
	return status.Installed && status.Running && status.BackendState == "Running"
}
