// Package types defines the core domain types for LAN Orangutan
package types

import "time"

// Device represents a discovered network device
type Device struct {
	IP string `json:"ip"`
	// IPv6 is a routable IPv6 address the same device was also seen at, recorded
	// when IPv6 neighbour discovery matches this device by MAC alongside its IPv4
	// address. It is shown next to the IPv4 address so a dual-stack device
	// displays both. Empty when no routable IPv6 sighting has been attributed to
	// the device (the common case on an IPv4-only home network).
	IPv6     string `json:"ipv6,omitempty"`
	MAC      string `json:"mac"`
	Hostname string `json:"hostname"`
	// CustomHostname is a name the user gave the device, shown in place of the
	// scanned Hostname. Scans only ever update Hostname, so the user's name is
	// never overwritten, and clearing it falls back to what was discovered.
	CustomHostname string `json:"custom_hostname,omitempty"`
	Vendor         string `json:"vendor"`
	// Type is the inferred or user-assigned device kind (Phone, Printer, TV,
	// Router, and so on), or "" when it could not be determined.
	Type string `json:"type,omitempty"`
	// CustomType reports whether Type was set or overridden by the user.
	// When true, automated network scans preserve Type rather than recalculating it.
	CustomType bool `json:"custom_type,omitempty"`
	// WebUI is true when the device answered on a common web port during an
	// opt-in service probe, meaning it likely serves a dashboard or admin page.
	// Always false when service detection is off.
	WebUI bool `json:"web_ui,omitempty"`
	// Risks lists security concerns found during an opt-in service probe, such as
	// an exposed unencrypted service. Empty when nothing notable was found or the
	// probe is off.
	Risks []string `json:"risks,omitempty"`
	Label string   `json:"label"`
	Notes string   `json:"notes"`
	Group string   `json:"group"`
	// Location is where the device is ("Office", "Garage"). Unlike a label it
	// has exactly one value per device, so the table can sort and filter by it.
	Location string `json:"location,omitempty"`
	// Expected marks a device the user has confirmed belongs on the network. It
	// is set from the UI (individually or by baselining the whole list). An
	// online device that is not expected is "unexpected"; an expected device that
	// has gone offline is "missing". Like the other curated fields, it is
	// preserved across scans and protects the device from automatic cleanup.
	Expected     bool      `json:"expected,omitempty"`
	FirstSeen    time.Time `json:"first_seen"`
	LastSeen     time.Time `json:"last_seen"`
	ResponseTime *float64  `json:"response_time,omitempty"`

	// AddressHistory lists earlier IPs this device (matched by its MAC) was seen
	// at, oldest first. Empty for a device that has never changed address.
	AddressHistory []AddressChange `json:"address_history,omitempty"`
}

// AddressChange records an IP a device was previously seen at, and when it moved
// away from it, so the dashboard can show that a device has changed address.
type AddressChange struct {
	IP        string    `json:"ip"`
	ChangedAt time.Time `json:"changed_at"`
}

// DisplayHostname returns the user's custom hostname when one is set, and the
// scanned hostname otherwise.
func (d *Device) DisplayHostname() string {
	if d.CustomHostname != "" {
		return d.CustomHostname
	}
	return d.Hostname
}

// IsOnline returns true if the device was seen within the last hour
func (d *Device) IsOnline() bool {
	return time.Since(d.LastSeen) < time.Hour
}

// IsRecent returns true if the device was seen within the last 5 minutes
func (d *Device) IsRecent() bool {
	return time.Since(d.LastSeen) < 5*time.Minute
}

// Snapshot is a saved copy of the device list at a point in time, so the network
// can be compared against it later to see what has changed.
type Snapshot struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
	Devices   []Device  `json:"devices"`
}

// SnapshotMeta is a snapshot without its device list, for listings.
type SnapshotMeta struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	CreatedAt   time.Time `json:"created_at"`
	DeviceCount int       `json:"device_count"`
}

// Backup is a portable copy of everything a user would not want to lose: the
// device list (with labels, notes and groups), the runtime settings overrides
// (the scan state), and the saved snapshots. It deliberately does NOT include the
// password, which lives in a separate file and is not the user's data to carry
// between machines.
type Backup struct {
	CreatedAt time.Time          `json:"created_at"`
	Devices   map[string]*Device `json:"devices"`
	State     *ScanState         `json:"state,omitempty"`
	Snapshots []Snapshot         `json:"snapshots"`
}

// Network represents a detected network interface
type Network struct {
	CIDR         string `json:"cidr"`
	Interface    string `json:"interface"`
	FriendlyName string `json:"friendly_name"`
	IP           string `json:"ip"`
	IsTailscale  bool   `json:"is_tailscale"`
	IsWireless   bool   `json:"is_wireless"`
	// IsIPv6 marks a network that is discovered rather than swept. An IPv6
	// subnet holds 2^64 addresses, so there is nothing to sweep; devices are
	// found by soliciting neighbours and reading the neighbour cache instead.
	IsIPv6 bool `json:"is_ipv6"`
}

// ScanState holds the last scan time for rate limiting
type ScanState struct {
	LastScan map[string]time.Time `json:"last_scan"`
	// LastDuration records how long the previous scan of each network took,
	// in seconds, so the UI can estimate progress for subsequent scans.
	LastDuration map[string]float64 `json:"last_duration,omitempty"`
	// ContinuousScan is a runtime override for background scanning, set from the
	// UI. nil means "use the configured default"; a set value wins over config
	// so the user's choice survives a restart.
	ContinuousScan *bool `json:"continuous_scan,omitempty"`
	// RetentionDays and MaxDevices are runtime overrides for the storage limits,
	// set from the Settings page. As with ContinuousScan, nil means "use the
	// configured default" and a set value survives a restart, so a user who never
	// edits config.ini can still change these. 0 means the limit is off (keep
	// devices forever / no cap).
	RetentionDays *int `json:"retention_days,omitempty"`
	MaxDevices    *int `json:"max_devices,omitempty"`
	// EnableServiceDetection is a runtime override for the opt-in port probe, set
	// from the Settings page. nil means "use the configured default".
	EnableServiceDetection *bool `json:"enable_service_detection,omitempty"`
	// ScanInterval is a runtime override for how often the background scanner
	// re-scans, in seconds, set from the Settings page. nil means "use the
	// configured default". The scanner reads it fresh before each wait, so a
	// change takes effect on the next cycle.
	ScanInterval *int `json:"scan_interval,omitempty"`
	// Networks, ExcludeNetworks and OnlyConfiguredNetworks are runtime overrides
	// for the scan-target lists, set from the Settings page. A nil pointer means
	// "use the configured default"; a non-nil slice (even empty) is an explicit
	// override. They are read fresh when scan targets are resolved, so a change
	// takes effect on the next scan.
	Networks               *[]string `json:"networks,omitempty"`
	ExcludeNetworks        *[]string `json:"exclude_networks,omitempty"`
	OnlyConfiguredNetworks *bool     `json:"only_configured_networks,omitempty"`
	// Groups is the user's device-group list, set from the Settings page. nil
	// means "use the built-in defaults"; a non-nil slice replaces them outright,
	// so a group can be added, renamed or removed. Renaming rewrites the group
	// on every device that used it, so the list and the devices never disagree.
	Groups *[]string `json:"groups,omitempty"`

	// Locations is the user's list of places a device can be. There are no
	// built-in ones: rooms are personal, and an English default list would be
	// wrong for most homes and every other language.
	Locations []string `json:"locations,omitempty"`

	// Webhooks are the outbound notifications set up on the Settings page or
	// with `orangutan webhook`. Each one is posted to when a device event it
	// subscribes to happens.
	Webhooks []Webhook `json:"webhooks,omitempty"`
	// MissingCheckedAt is when expected devices were last checked for having
	// gone missing. "Missing" is reached by time passing, not by a scan seeing
	// something, so the check fires only for devices that crossed the offline
	// threshold since this moment, and so fires once per disappearance.
	MissingCheckedAt time.Time `json:"missing_checked_at,omitzero"`
}

// Webhook is one outbound notification target. The URL can hold a secret (a
// Telegram bot token, a Discord webhook key), so it is masked in every API
// response and never logged.
type Webhook struct {
	ID   string `json:"id"`
	Name string `json:"name,omitempty"`
	URL  string `json:"url"`
	// Events lists the device events this webhook receives, such as
	// "device.new".
	Events  []string `json:"events"`
	Enabled bool     `json:"enabled"`
	// LastResult is the outcome of the most recent delivery attempt, or nil
	// when nothing has been sent yet.
	LastResult *WebhookResult `json:"last_result,omitempty"`
}

// WebhookResult records how a delivery went, so the UI and CLI can show
// whether a webhook is working.
type WebhookResult struct {
	Time    time.Time `json:"time"`
	Event   string    `json:"event,omitempty"`
	Success bool      `json:"success"`
	// Status is the HTTP status the receiver answered with, or 0 when it could
	// not be reached at all.
	Status int    `json:"status,omitempty"`
	Error  string `json:"error,omitempty"`
	// Body is a short, redacted excerpt of a successful answer, for showing
	// after a test. It is never stored.
	Body string `json:"-"`
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

// StatusKey is StatusLabel as a stable key ("connected", "loggedOut"), for
// showing the status in the visitor's language. A backend state with no key of
// its own comes back as "" and the caller falls back to StatusLabel.
func (t TailscaleStatus) StatusKey() string {
	if !t.Installed {
		return "notInstalled"
	}
	if !t.Running {
		return "notRunning"
	}
	switch t.BackendState {
	case "Running":
		return "connected"
	case "Stopped":
		return "stopped"
	case "NeedsLogin":
		return "loggedOut"
	case "NeedsMachineAuth":
		return "awaitingApproval"
	case "Starting":
		return "starting"
	case "NoState", "":
		return "unknown"
	}
	return ""
}

// APIResponse is the standard API response wrapper
type APIResponse struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data,omitempty"`
	Error   string      `json:"error,omitempty"`
	// Code and Args identify a validation error so the web interface can
	// show it in the visitor's language; see internal/uerr.
	Code string         `json:"code,omitempty"`
	Args map[string]any `json:"args,omitempty"`
}

// DeviceStats holds device statistics for the dashboard
type DeviceStats struct {
	Total   int            `json:"total"`
	Online  int            `json:"online"`
	Offline int            `json:"offline"`
	Groups  map[string]int `json:"groups"`
}
