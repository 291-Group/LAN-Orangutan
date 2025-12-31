package cli

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/291-Group/LAN-Orangutan/internal/network"
	"github.com/291-Group/LAN-Orangutan/internal/scanner"
	"github.com/291-Group/LAN-Orangutan/internal/storage"
)

var scanCmd = &cobra.Command{
	Use:   "scan [network|all]",
	Short: "Scan network for devices",
	Long: `Scan a network for devices using nmap or arp-scan.
Specify a network CIDR (e.g., 192.168.1.0/24) or 'all' to scan all detected networks.
If no argument is provided, scans the first detected network.`,
	Args: cobra.MaximumNArgs(1),
	RunE: runScan,
}

func runScan(cmd *cobra.Command, args []string) error {
	// Initialize storage
	store, err := storage.New(cfg.DevicesFile(), cfg.StateFile())
	if err != nil {
		return fmt.Errorf("failed to initialize storage: %w", err)
	}

	// Create scanner
	s := scanner.New(cfg.Scanning.MinScanInterval)

	// Determine networks to scan
	var networks []string

	if len(args) == 0 || args[0] == "" {
		// Scan first detected network
		detected, err := network.DetectNetworks()
		if err != nil {
			return fmt.Errorf("failed to detect networks: %w", err)
		}
		if len(detected) == 0 {
			return fmt.Errorf("no networks detected")
		}
		// Skip Tailscale by default
		for _, n := range detected {
			if !n.IsTailscale {
				networks = append(networks, n.CIDR)
				break
			}
		}
		if len(networks) == 0 {
			networks = append(networks, detected[0].CIDR)
		}
	} else if args[0] == "all" {
		// Scan all detected networks
		detected, err := network.DetectNetworks()
		if err != nil {
			return fmt.Errorf("failed to detect networks: %w", err)
		}
		for _, n := range detected {
			networks = append(networks, n.CIDR)
		}
	} else {
		// Scan specified network
		if !network.ValidateCIDR(args[0]) {
			return fmt.Errorf("invalid CIDR: %s", args[0])
		}
		networks = append(networks, args[0])
	}

	if len(networks) == 0 {
		return fmt.Errorf("no networks to scan")
	}

	// Scan each network
	for _, cidr := range networks {
		// Check rate limit
		lastScan := store.GetLastScan(cidr)
		canScan, waitTime := s.CheckRateLimit(lastScan)
		if !canScan {
			fmt.Printf("Rate limited for %s, wait %.0f seconds\n", cidr, waitTime.Seconds())
			continue
		}

		fmt.Printf("Scanning %s...\n", cidr)

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		result, err := s.Scan(ctx, cidr)
		cancel()

		if err != nil {
			fmt.Fprintf(os.Stderr, "Error scanning %s: %v\n", cidr, err)
			continue
		}

		if !result.Success {
			fmt.Fprintf(os.Stderr, "Scan failed for %s: %s\n", cidr, result.Error)
			continue
		}

		// Merge devices
		if err := store.MergeDevices(result.Devices); err != nil {
			fmt.Fprintf(os.Stderr, "Error saving devices: %v\n", err)
			continue
		}

		// Update last scan time
		if err := store.SetLastScan(cidr, time.Now()); err != nil {
			fmt.Fprintf(os.Stderr, "Error updating scan state: %v\n", err)
		}

		fmt.Printf("Found %d devices using %s (%.2fs)\n", result.DeviceCount, result.Scanner, result.Duration)
	}

	return nil
}
