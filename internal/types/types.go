// Package types defines the core domain types for LAN Orangutan
package types

import "time"

// Device represents a discovered network device
type Device struct {
	IP           string    `json:"ip"`
	MAC          string    `json:"mac"`
	Hostname     string    `json:"hostname"`
	Vendor       string    `json:"vendor"`
	Label        string    `json:"label"`
	Notes        string    `json:"notes"`
	Group        string    `json:"group"`
	FirstSeen    time.Time `json:"first_seen"`
	LastSeen     time.Time `json:"last_seen"`
	ResponseTime *float64  `json:"response_time,omitempty"`
}

// IsOnline returns true if the device was seen within the last hour
func (d *Device) IsOnline() bool {
	return time.Since(d.LastSeen) < time.Hour
}

// IsRecent returns true if the device was seen within the last 5 minutes
func (d *Device) IsRecent() bool {
	return time.Since(d.LastSeen) < 5*time.Minute
}

// Network represents a detected network interface
type Network struct {
	CIDR         string `json:"cidr"`
	Interface    string `json:"interface"`
	FriendlyName string `json:"friendly_name"`
	IP           string `json:"ip"`
	IsTailscale  bool   `json:"is_tailscale"`
	IsWireless   bool   `json:"is_wireless"`
}

// ScanState holds the last scan time for rate limiting
type ScanState struct {
	LastScan map[string]time.Time `json:"last_scan"`
	// LastDuration records how long the previous scan of each network took,
	// in seconds, so the UI can estimate progress for subsequent scans.
	LastDuration map[string]float64 `json:"last_duration,omitempty"`
}

// ScanResult represents the outcome of a network scan
type ScanResult struct {
	Success     bool      `json:"success"`
	Error       string    `json:"error,omitempty"`
	Devices     []Device  `json:"devices"`
	DeviceCount int       `json:"device_count"`
	Network     string    `json:"network"`
	Scanner     string    `json:"scanner"`
	Duration    float64   `json:"duration"`
	Timestamp   time.Time `json:"timestamp"`
}

// TailscaleStatus represents Tailscale connection status
type TailscaleStatus struct {
	Installed bool `json:"installed"`
	// Running reports only that the Tailscale daemon answered. It stays true
	// when the user is logged out or has stopped Tailscale, so it must not be
	// used to decide whether traffic can flow: use Connected for that.
	Running bool `json:"running"`
	// Connected reports whether Tailscale is actually up and usable.
	Connected    bool   `json:"connected"`
	BackendState string `json:"backend_state"`
	Version      string `json:"version"`
	TailnetName  string `json:"tailnet_name"`
	SelfIP       string `json:"self_ip"`
	SelfHostname string `json:"self_hostname"`
	PeerCount    int    `json:"peer_count"`
	ExitNode     string `json:"exit_node,omitempty"`
}

// StatusLabel describes the Tailscale connection in words suitable for display.
func (t TailscaleStatus) StatusLabel() string {
	if !t.Installed {
		return "Not installed"
	}
	if !t.Running {
		return "Not running"
	}

	switch t.BackendState {
	case "Running":
		return "Connected"
	case "Stopped":
		return "Stopped"
	case "NeedsLogin":
		return "Logged out"
	case "NeedsMachineAuth":
		return "Awaiting approval"
	case "Starting":
		return "Starting"
	case "NoState", "":
		return "Unknown"
	default:
		return t.BackendState
	}
}

// APIResponse is the standard API response wrapper
type APIResponse struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data,omitempty"`
	Error   string      `json:"error,omitempty"`
}

// DeviceStats holds device statistics for the dashboard
type DeviceStats struct {
	Total   int            `json:"total"`
	Online  int            `json:"online"`
	Offline int            `json:"offline"`
	Groups  map[string]int `json:"groups"`
}
