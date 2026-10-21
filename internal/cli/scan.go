package cli

import (
	"context"
	"fmt"
	"os"
	"runtime"
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
	// Apply the retention window and device cap (a UI override wins over the
	// config default), so a CLI scan bounds the stored list the same way the
	// server does.
	if err := store.SetLimits(
		store.RetentionDaysOrDefault(cfg.Storage.RetentionDays),
		store.MaxDevicesOrDefault(cfg.Storage.MaxDevices),
	); err != nil {
		return fmt.Errorf("failed to apply storage limits: %w", err)
	}

	// Create scanner
	s := scanner.New(cfg.Scanning.MinScanInterval, cfg.Scanning.EnableServiceDetection)

	// Determine networks to scan
	var networks []string

	if len(args) == 0 || args[0] == "" {
		// Scan first detected network
		detected, err := network.DetectNetworks()
		detected = network.WithConfigured(detected, networkFilter(store, cfg))
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
		detected = network.WithConfigured(detected, networkFilter(store, cfg))
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

		fmt.Printf("Found %d devices using %s (%.2fs)\n\n", result.DeviceCount, result.Scanner, result.Duration)

		// Display found devices
		if len(result.Devices) > 0 {
			fmt.Printf("%-16s %-18s %-20s %s\n", "IP", "MAC", "HOSTNAME", "VENDOR")
			fmt.Printf("%-16s %-18s %-20s %s\n", "──────────────", "─────────────────", "───────────────────", "──────────────────────")
			hasMac := false
			for _, d := range result.Devices {
				hostname := d.Hostname
				if hostname == "" {
					hostname = "-"
				}
				vendor := d.Vendor
				if vendor == "" {
					vendor = "-"
				}
				mac := d.MAC
				if mac == "" {
					mac = "-"
				} else {
					hasMac = true
				}
				fmt.Printf("%-16s %-18s %-20s %s\n", d.IP, mac, truncate(hostname, 20), truncate(vendor, 30))
			}
			fmt.Println()

			// Warn if no MAC addresses found (permission issue)
			if !hasMac && os.Getuid() != 0 {
				switch runtime.GOOS {
				case "darwin":
					fmt.Println("Note: Run with sudo to get MAC addresses and vendor info:")
					fmt.Println("  sudo ./orangutan scan")
				case "linux":
					fmt.Println("Note: Run with sudo for MAC addresses and vendor info:")
					fmt.Println("  sudo orangutan scan")
				}
				fmt.Println()
			}
		}
	}

	// When scanning everything, also pick up IPv6 neighbors and mDNS
	// announcements, which cannot be reached by sweeping a range. Both merge as
	// secondary sources so they enrich rather than overwrite.
	if len(args) > 0 && args[0] == "all" {
		ctx := context.Background()
		if ipv6 := s.DiscoverIPv6(ctx); len(ipv6) > 0 {
			if err := store.MergeIPv6Neighbors(ipv6); err != nil {
				fmt.Fprintf(os.Stderr, "Error saving IPv6 neighbors: %v\n", err)
			} else {
				fmt.Printf("Found %d IPv6 neighbor(s)\n", len(ipv6))
			}
		}
		if mdns := s.DiscoverMDNS(ctx); len(mdns) > 0 {
			if err := store.MergeSupplemental(mdns); err != nil {
				fmt.Fprintf(os.Stderr, "Error saving mDNS devices: %v\n", err)
			} else {
				fmt.Printf("Found %d mDNS device(s)\n", len(mdns))
			}
		}
	}

	return nil
}

// truncate shortens a string to maxLen, adding "..." if truncated
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return s[:maxLen]
	}
	return s[:maxLen-3] + "..."
}
