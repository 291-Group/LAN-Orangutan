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
			ScanInterval:    300,
			MinScanInterval: 30,
			EnablePortScan:  false,
			PortScanRange:   "1-1024",
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
		case "networks":
			c.Scanning.Networks = network.ParseNetworkList(value)
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
	if v := os.Getenv("ORANGUTAN_SCAN_INTERVAL"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			c.Scanning.ScanInterval = n
		}
	}
	if v := os.Getenv("ORANGUTAN_THEME"); v != "" {
		c.UI.Theme = v
	}
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
