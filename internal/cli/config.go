package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/291-Group/LAN-Orangutan/internal/auth"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Display current configuration",
	RunE:  runConfig,
}

func runConfig(cmd *cobra.Command, args []string) error {
	fmt.Println("=== LAN Orangutan Configuration ===")
	fmt.Printf("Config file: %s\n", cfgFile)
	fmt.Println()

	fmt.Println("[server]")
	fmt.Printf("  port = %d\n", cfg.Server.Port)
	fmt.Printf("  bind_address = %s\n", cfg.Server.BindAddress)
	fmt.Printf("  enable_api = %v\n", cfg.Server.EnableAPI)
	// Never print the password or its hash. Show only whether one is set, and
	// where it came from, which is what someone checking their setup needs.
	fmt.Printf("  password = %s\n", passwordSummary())
	fmt.Printf("  session_hours = %d\n", cfg.Server.SessionHours)
	fmt.Printf("  allow_insecure = %v\n", cfg.Server.AllowInsecure)
	fmt.Println()

	fmt.Println("[scanning]")
	fmt.Printf("  scan_interval = %d\n", cfg.Scanning.ScanInterval)
	fmt.Printf("  min_scan_interval = %d\n", cfg.Scanning.MinScanInterval)
	fmt.Printf("  enable_port_scan = %v\n", cfg.Scanning.EnablePortScan)
	fmt.Printf("  port_scan_range = %s\n", cfg.Scanning.PortScanRange)
	fmt.Println()

	fmt.Println("[storage]")
	fmt.Printf("  max_devices = %d\n", cfg.Storage.MaxDevices)
	fmt.Printf("  retention_days = %d\n", cfg.Storage.RetentionDays)
	fmt.Printf("  data_dir = %s\n", cfg.Storage.DataDir)
	fmt.Println()

	fmt.Println("[tailscale]")
	fmt.Printf("  enable = %v\n", cfg.Tailscale.Enable)
	fmt.Printf("  auto_detect = %v\n", cfg.Tailscale.AutoDetect)
	fmt.Println()

	fmt.Println("[ui]")
	fmt.Printf("  theme = %s\n", cfg.UI.Theme)

	return nil
}

// passwordSummary describes the password state without revealing it.
func passwordSummary() string {
	if cfg.Server.Password != "" {
		return "(set in config or environment)"
	}
	if auth.LoadHash(cfg.PasswordFile()) != "" {
		return "(set during first run setup)"
	}
	if cfg.RequiresSetup() {
		return "(not set - you will be asked to create one on first visit)"
	}
	return "(not set - not required with this bind address)"
}
