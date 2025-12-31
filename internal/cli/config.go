package cli

import (
	"fmt"

	"github.com/spf13/cobra"
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
