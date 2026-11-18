// Package config handles configuration loading and management
package config

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/291-Group/LAN-Orangutan/internal/network"
)

// GetDefaultDataDir returns the appropriate default data directory for the current OS
func GetDefaultDataDir() string {
	switch runtime.GOOS {
	case "darwin":
		// macOS: use Application Support
		home, err := os.UserHomeDir()
		if err != nil {
			return "/tmp/lan-orangutan"
		}
		return filepath.Join(home, "Library", "Application Support", "lan-orangutan")

	case "windows":
		// Windows: use AppData
		appData := os.Getenv("APPDATA")
		if appData == "" {
			home, err := os.UserHomeDir()
			if err != nil {
				return "C:\\lan-orangutan"
			}
			return filepath.Join(home, "AppData", "Roaming", "lan-orangutan")
		}
		return filepath.Join(appData, "lan-orangutan")

	default:
		// Linux and others
		// Use /var/lib if running as root, otherwise use user's local share
		if os.Getuid() == 0 {
			return "/var/lib/lan-orangutan"
		}
		home, err := os.UserHomeDir()
		if err != nil {
			return "/tmp/lan-orangutan"
		}
		return filepath.Join(home, ".local", "share", "lan-orangutan")
	}
}

// GetDefaultConfigFile returns the appropriate default config file path for the current OS
func GetDefaultConfigFile() string {
	switch runtime.GOOS {
	case "darwin":
		home, err := os.UserHomeDir()
		if err != nil {
			return "/tmp/lan-orangutan/config.ini"
		}
		return filepath.Join(home, "Library", "Application Support", "lan-orangutan", "config.ini")

	case "windows":
		appData := os.Getenv("APPDATA")
		if appData == "" {
			return "C:\\lan-orangutan\\config.ini"
		}
		return filepath.Join(appData, "lan-orangutan", "config.ini")

	default:
		// Linux: use /etc if running as root, otherwise use user config
		if os.Getuid() == 0 {
			return "/etc/lan-orangutan/config.ini"
		}
		home, err := os.UserHomeDir()
		if err != nil {
			return "/tmp/lan-orangutan/config.ini"
		}
		return filepath.Join(home, ".config", "lan-orangutan", "config.ini")
	}
}

// Config holds all application configuration
type Config struct {
	Server    ServerConfig
	Scanning  ScanningConfig
	Storage   StorageConfig
	Tailscale TailscaleConfig
	UI        UIConfig
}

// ServerConfig holds web server settings
type ServerConfig struct {
	Port        int
	BindAddress string
	EnableAPI   bool

	// Password protects the dashboard and API. Empty means no login is
	// required. May be given as plaintext or as a bcrypt hash.
	Password string

	// SessionHours is how long a login stays valid.
	SessionHours int

	// AllowInsecure permits binding to a non-loopback address without a
	// password. Off by default: doing so exposes the API, which can modify
	// stored data, to everyone on the network.
	AllowInsecure bool
}

// ScanningConfig holds scanner settings
type ScanningConfig struct {
	ScanInterval    int
	MinScanInterval int
	EnablePortScan  bool
	PortScanRange   string

	// ContinuousScan makes the server re-scan the detected networks on its own
	// every ScanInterval seconds, so the device list stays current without
	// anyone clicking Scan or keeping a browser open. On by default: a
	// discovery tool that only ever shows one stale scan is not much use. Set
	// it false to scan only on demand.
	ContinuousScan bool

	// EnableServiceDetection turns on a light probe of a few well-known ports
	// during a scan, used only to identify devices more precisely (a printer, a
	// NAS, a device with a web interface). Off by default so the scan stays a
	// quiet ping sweep unless the user opts in.
	EnableServiceDetection bool

	// OnlyConfiguredNetworks, when true, offers only the networks declared in
	// Networks and ignores everything auto-detection finds. Useful on a host
	// with many Docker bridges, VLANs or routed interfaces where the automatic
	// list is noisy.
	OnlyConfiguredNetworks bool

	// ExcludeNetworks are CIDRs to drop from the auto-detected list, for real
	// networks the user does not want scanned. A blocklist that complements
	// Networks (the allowlist): keep auto-detection but hide a few subnets.
	ExcludeNetworks []string

	// NmapMaxRTTTimeout and NmapMaxRetries override the nmap timing the scanner
	// applies on its own to networks reached through a route or tunnel
	// (WireGuard and the like), where there is no ARP. 0 and -1 mean automatic.
	// Local networks are not affected.
	NmapMaxRTTTimeout time.Duration
	NmapMaxRetries    int

	// Networks are CIDRs the user has declared explicitly, for cases where
	// automatic detection cannot see the right network. A container only sees
	// Docker's private network, so without this it can never scan the LAN.
	Networks []string
}

// StorageConfig holds data storage settings
type StorageConfig struct {
	MaxDevices    int
	RetentionDays int
	DataDir       string
}

// TailscaleConfig holds Tailscale integration settings
type TailscaleConfig struct {
	Enable     bool
	AutoDetect bool
}

// UIConfig holds user interface settings
type UIConfig struct {
	Theme string
}

// Sensible fallbacks for the scan timings, also used by Normalize to repair an
// out-of-range value rather than let it silently disable a feature.
const (
	defaultScanInterval    = 300
	defaultMinScanInterval = 30
)

// Default returns a Config with default values
func Default() *Config {
	return &Config{
		Server: ServerConfig{
			Port: 291,
			// Reachable from the network, because this is usually installed on
			// a server or a Pi and opened from another machine. Safety comes
			// from RequiresSetup: with no password set, the only thing a
			// visitor can reach is the page that creates one.
			BindAddress:  "0.0.0.0",
			EnableAPI:    true,
			SessionHours: 24 * 7,
		},
		Scanning: ScanningConfig{
			ScanInterval:           defaultScanInterval,
			MinScanInterval:        defaultMinScanInterval,
			EnablePortScan:         false,
			PortScanRange:          "1-1024",
			ContinuousScan:         true,
			EnableServiceDetection: false,
			NmapMaxRetries:         -1,
		},
		Storage: StorageConfig{
			MaxDevices:    1000,
			RetentionDays: 90,
			DataDir:       GetDefaultDataDir(),
		},
		Tailscale: TailscaleConfig{
			Enable:     true,
			AutoDetect: true,
		},
		UI: UIConfig{
			Theme: "auto",
		},
	}
}

// Load reads configuration from an INI file
func Load(path string) (*Config, error) {
	cfg := Default()

	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil // Return defaults if file doesn't exist
		}
		return nil, fmt.Errorf("failed to open config: %w", err)
	}
	defer file.Close()

	var currentSection string
	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}

		// Section header
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			currentSection = strings.ToLower(line[1 : len(line)-1])
			continue
		}

		// Key-value pair
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(strings.ToLower(parts[0]))
		value := strings.TrimSpace(parts[1])

		cfg.setValue(currentSection, key, value)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("failed to read config: %w", err)
	}

	return cfg, nil
}

// setValue sets a configuration value based on section and key
func (c *Config) setValue(section, key, value string) {
	switch section {
	case "server":
		switch key {
		case "port":
			if v, err := strconv.Atoi(value); err == nil {
				c.Server.Port = v
			}
		case "bind_address":
			c.Server.BindAddress = value
		case "enable_api":
			c.Server.EnableAPI = parseBool(value)
		case "password":
			c.Server.Password = value
		case "session_hours":
			if v, err := strconv.Atoi(value); err == nil {
				c.Server.SessionHours = v
			}
		case "allow_insecure":
			c.Server.AllowInsecure = parseBool(value)
		}
	case "scanning":
		switch key {
		case "scan_interval":
			if v, err := strconv.Atoi(value); err == nil {
				c.Scanning.ScanInterval = v
			}
		case "min_scan_interval":
			if v, err := strconv.Atoi(value); err == nil {
				c.Scanning.MinScanInterval = v
			}
		case "enable_port_scan":
			c.Scanning.EnablePortScan = parseBool(value)
		case "port_scan_range":
			c.Scanning.PortScanRange = value
		case "continuous_scan":
			c.Scanning.ContinuousScan = parseBool(value)
		case "enable_service_detection":
			c.Scanning.EnableServiceDetection = parseBool(value)
		case "only_configured_networks":
			c.Scanning.OnlyConfiguredNetworks = parseBool(value)
		case "nmap_max_rtt_timeout":
			if d, ok := parseMillis(value); ok {
				c.Scanning.NmapMaxRTTTimeout = d
			}
		case "nmap_max_retries":
			if v, err := strconv.Atoi(value); err == nil && v >= 0 {
				c.Scanning.NmapMaxRetries = v
			}
		case "networks":
			c.Scanning.Networks = network.ParseNetworkList(value)
		case "exclude_networks":
			c.Scanning.ExcludeNetworks = network.ParseNetworkList(value)
		}
	case "storage":
		switch key {
		case "max_devices":
			if v, err := strconv.Atoi(value); err == nil {
				c.Storage.MaxDevices = v
			}
		case "retention_days":
			if v, err := strconv.Atoi(value); err == nil {
				c.Storage.RetentionDays = v
			}
		case "data_dir":
			c.Storage.DataDir = value
		}
	case "tailscale":
		switch key {
		case "enable":
			c.Tailscale.Enable = parseBool(value)
		case "auto_detect":
			c.Tailscale.AutoDetect = parseBool(value)
		}
	case "ui":
		switch key {
		case "theme":
			c.UI.Theme = value
		}
	}
}

// ApplyEnv overlays settings from environment variables onto c.
//
// Environment variables take precedence over the config file, which makes the
// app configurable in a container without mounting a config file. Command line
// flags still win over both.
func (c *Config) ApplyEnv() {
	if v := os.Getenv("ORANGUTAN_PORT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			c.Server.Port = n
		}
	}
	if v := os.Getenv("ORANGUTAN_BIND_ADDRESS"); v != "" {
		c.Server.BindAddress = v
	}
	if v := os.Getenv("ORANGUTAN_PASSWORD"); v != "" {
		c.Server.Password = v
	}
	// A password file keeps the secret out of the process environment, which
	// is how container secrets are normally delivered.
	if v := os.Getenv("ORANGUTAN_PASSWORD_FILE"); v != "" {
		if data, err := os.ReadFile(v); err == nil {
			if pw := strings.TrimSpace(string(data)); pw != "" {
				c.Server.Password = pw
			}
		}
	}
	if v := os.Getenv("ORANGUTAN_SESSION_HOURS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			c.Server.SessionHours = n
		}
	}
	if v := os.Getenv("ORANGUTAN_ALLOW_INSECURE"); v != "" {
		c.Server.AllowInsecure = parseBool(v)
	}
	if v := os.Getenv("ORANGUTAN_DATA_DIR"); v != "" {
		c.Storage.DataDir = v
	}
	if v := os.Getenv("ORANGUTAN_NETWORKS"); v != "" {
		c.Scanning.Networks = network.ParseNetworkList(v)
	}
	if v := os.Getenv("ORANGUTAN_ENABLE_SERVICE_DETECTION"); v != "" {
		c.Scanning.EnableServiceDetection = parseBool(v)
	}
	if v := os.Getenv("ORANGUTAN_NMAP_MAX_RTT_TIMEOUT"); v != "" {
		if d, ok := parseMillis(v); ok {
			c.Scanning.NmapMaxRTTTimeout = d
		}
	}
	if v := os.Getenv("ORANGUTAN_NMAP_MAX_RETRIES"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			c.Scanning.NmapMaxRetries = n
		}
	}
	if v := os.Getenv("ORANGUTAN_ONLY_CONFIGURED_NETWORKS"); v != "" {
		c.Scanning.OnlyConfiguredNetworks = parseBool(v)
	}
	if v := os.Getenv("ORANGUTAN_EXCLUDE_NETWORKS"); v != "" {
		c.Scanning.ExcludeNetworks = network.ParseNetworkList(v)
	}
	if v := os.Getenv("ORANGUTAN_SCAN_INTERVAL"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			c.Scanning.ScanInterval = n
		}
	}
	if v := os.Getenv("ORANGUTAN_CONTINUOUS_SCAN"); v != "" {
		c.Scanning.ContinuousScan = parseBool(v)
	}
	if v := os.Getenv("ORANGUTAN_THEME"); v != "" {
		c.UI.Theme = v
	}
}

// Normalize repairs out-of-range scan timings, clamping them back to the
// defaults, and returns a human-readable note for each value it had to correct.
// A non-positive scan_interval would otherwise silently disable the background
// scanner even with continuous_scan on, and a non-positive min_scan_interval
// would silently disable per-network rate limiting; both are corrected here so
// a config typo cannot quietly turn a feature off. Call it after loading the
// config from file and environment.
func (c *Config) Normalize() []string {
	var notes []string

	if c.Scanning.MinScanInterval <= 0 {
		notes = append(notes, fmt.Sprintf(
			"min_scan_interval was %d; using %d so per-network rate limiting stays on",
			c.Scanning.MinScanInterval, defaultMinScanInterval))
		c.Scanning.MinScanInterval = defaultMinScanInterval
	}

	if c.Scanning.ScanInterval <= 0 {
		notes = append(notes, fmt.Sprintf(
			"scan_interval was %d; using %d so continuous scanning works when enabled",
			c.Scanning.ScanInterval, defaultScanInterval))
		c.Scanning.ScanInterval = defaultScanInterval
	}

	return notes
}

// IsLoopbackBind reports whether the configured bind address only accepts
// connections from the machine the app is running on.
//
// An empty bind address means every interface, which is not loopback.
func (c *Config) IsLoopbackBind() bool {
	return isLoopbackAddress(c.Server.BindAddress)
}

func isLoopbackAddress(addr string) bool {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return false
	}
	if strings.EqualFold(addr, "localhost") {
		return true
	}
	// Strip brackets from IPv6 literals such as [::1].
	addr = strings.TrimPrefix(strings.TrimSuffix(addr, "]"), "[")

	ip := net.ParseIP(addr)
	if ip == nil {
		return false
	}
	return ip.IsLoopback()
}

// RequiresSetup reports whether the user must create a password before the
// dashboard becomes usable.
//
// Setup is required whenever the server can be reached from the network and no
// password exists yet. Bound to loopback the dashboard is already private, so
// a password would be friction with no benefit, and an operator who has some
// other protection in front can opt out entirely.
func (c *Config) RequiresSetup() bool {
	if c.Server.AllowInsecure {
		return false
	}
	if c.IsLoopbackBind() {
		return false
	}
	return c.Server.Password == ""
}

// PasswordFile returns the path where a password created through the setup page
// is stored.
//
// It lives beside the data rather than in the config file, so that completing
// setup does not rewrite a file the user maintains by hand.
func (c *Config) PasswordFile() string {
	return filepath.Join(c.Storage.DataDir, "auth")
}

// APITokenFile is the path to the file holding the hash of the API token.
//
// Beside the password file and for the same reason: it is created by using the
// app, not by editing configuration, so it must never live in a file the user
// maintains by hand.
func (c *Config) APITokenFile() string {
	return filepath.Join(c.Storage.DataDir, "api-token")
}

// SessionTTL returns how long a login should remain valid.
func (c *Config) SessionTTL() time.Duration {
	if c.Server.SessionHours <= 0 {
		return 24 * 7 * time.Hour
	}
	return time.Duration(c.Server.SessionHours) * time.Hour
}

// DevicesFile returns the full path to the devices JSON file
func (c *Config) DevicesFile() string {
	return filepath.Join(c.Storage.DataDir, "devices.json")
}

// StateFile returns the full path to the scan state file
func (c *Config) StateFile() string {
	return filepath.Join(c.Storage.DataDir, "scan_state.json")
}

// parseBool parses common boolean representations
func parseBool(s string) bool {
	s = strings.ToLower(s)
	return s == "true" || s == "yes" || s == "1" || s == "on"
}

// parseMillis reads a timeout written the way nmap writes one ("200ms", "1s")
// or as a plain number of milliseconds ("200"). The result is never shorter
// than a millisecond: nmap takes the value in whole milliseconds, so anything
// finer ("500us") would reach it as 0 and turn the timeout off entirely.
func parseMillis(v string) (time.Duration, bool) {
	v = strings.TrimSpace(v)
	if n, err := strconv.Atoi(v); err == nil {
		if n <= 0 {
			return 0, false
		}
		return time.Duration(n) * time.Millisecond, true
	}
	d, err := time.ParseDuration(v)
	if err != nil || d <= 0 {
		return 0, false
	}
	if d < time.Millisecond {
		d = time.Millisecond
	}
	return d, true
}
