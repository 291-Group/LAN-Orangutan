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
	Installed    bool   `json:"installed"`
	Running      bool   `json:"running"`
	BackendState string `json:"backend_state"`
	Version      string `json:"version"`
	TailnetName  string `json:"tailnet_name"`
	SelfIP       string `json:"self_ip"`
	SelfHostname string `json:"self_hostname"`
	PeerCount    int    `json:"peer_count"`
	ExitNode     string `json:"exit_node,omitempty"`
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
