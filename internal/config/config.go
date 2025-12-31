// Package config handles configuration loading and management
package config

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	DefaultConfigFile  = "/etc/lan-orangutan/config.ini"
	DefaultDataDir     = "/var/lib/lan-orangutan"
	DefaultDevicesFile = "/var/lib/lan-orangutan/devices.json"
	DefaultStateFile   = "/var/lib/lan-orangutan/scan_state.json"
)

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
}

// ScanningConfig holds scanner settings
type ScanningConfig struct {
	ScanInterval    int
	MinScanInterval int
	EnablePortScan  bool
	PortScanRange   string
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
			Port:        291,
			BindAddress: "0.0.0.0",
			EnableAPI:   true,
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
			DataDir:       DefaultDataDir,
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
