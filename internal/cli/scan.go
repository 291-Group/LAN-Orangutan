package cli

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"time"

	"github.com/spf13/cobra"

	"github.com/291-Group/LAN-Orangutan/internal/network"
	"github.com/291-Group/LAN-Orangutan/internal/notify"
	"github.com/291-Group/LAN-Orangutan/internal/scanner"
)

var scanCmd = &cobra.Command{
	Use:   "scan [network|all]",
	Short: "Scan network for devices",
	Long: `Scan a network for devices using nmap or arp-scan.
Specify a network CIDR (e.g., 192.168.1.0/24) or 'all' to scan all detected networks.
If no argument is provided, scans the first detected network.

Exits with status 1 when no network could be scanned.`,
	Args: cobra.MaximumNArgs(1),
	RunE: runScan,
}

func runScan(cmd *cobra.Command, args []string) error {
	target := ""
	if len(args) == 1 {
		target = args[0]
	}
	if target != "" && target != "all" && !network.ValidateCIDR(target) {
		return fmt.Errorf("invalid CIDR: %s", target)
	}

	store, err := openStore()
	if err != nil {
		return err
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

	// Tell the webhooks what this scan changed, once it is over. The process
	// exits as soon as the scan returns, so unlike the server it waits for the
	// deliveries, with a limit so an unreachable webhook cannot hang the
	// command.
	events := notify.Begin(store)
	defer func() {
		events.Finish()
		if !notify.Wait(time.Minute) {
			fmt.Fprintln(os.Stderr, "Some webhook notifications were still being sent and were abandoned.")
		}
	}()

	// Create scanner
	s := scanner.New(cfg.Scanning.MinScanInterval, cfg.Scanning.EnableServiceDetection)
	s.SetRoutedTiming(cfg.Scanning.NmapMaxRTTTimeout, cfg.Scanning.NmapMaxRetries)

	// Determine networks to scan
	var networks []string
	switch target {
	case "":
		// Scan first detected network
		detected, err := network.DetectNetworks()
		if err != nil {
			return fmt.Errorf("failed to detect networks: %w", err)
		}
		detected = network.WithConfigured(detected, networkFilter(store, cfg))
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
	case "all":
		detected, err := network.DetectNetworks()
		if err != nil {
			return fmt.Errorf("failed to detect networks: %w", err)
		}
		detected = network.WithConfigured(detected, networkFilter(store, cfg))
		for _, n := range detected {
			networks = append(networks, n.CIDR)
		}
	default:
		networks = append(networks, target)
	}

	if len(networks) == 0 {
		return fmt.Errorf("no networks to scan")
	}

	// The networks that completed, for the ARP-table pass below, and the ones
	// that did not, with the reason, reported once that pass has had its say.
	var scanned []string
	type failure struct{ cidr, reason string }
	var failed []failure

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
			failed = append(failed, failure{cidr, err.Error()})
			continue
		}
		if !result.Success {
			failed = append(failed, failure{cidr, result.Error})
			continue
		}

		// Merge devices
		if err := store.MergeScan(cidr, result.Devices); err != nil {
			fmt.Fprintf(os.Stderr, "Error saving devices: %v\n", err)
			continue
		}
		scanned = append(scanned, cidr)

		// Update last scan time
		if err := store.SetLastScan(cidr, time.Now()); err != nil {
			fmt.Fprintf(os.Stderr, "Error updating scan state: %v\n", err)
		}

		fmt.Printf("Found %d devices using %s (%.2fs)\n\n", result.DeviceCount, result.Scanner, result.Duration)

		// Display found devices
		if len(result.Devices) > 0 {
			rows := make([][]string, 0, len(result.Devices))
			hasMac := false
			for _, d := range result.Devices {
				if d.MAC != "" {
					hasMac = true
				}
				rows = append(rows, []string{
					d.IP,
					orDash(d.MAC),
					truncate(orDash(d.Hostname), 20),
					truncate(orDash(d.Vendor), 30),
				})
			}
			if err := printTable(os.Stdout, []string{"IP", "MAC", "HOSTNAME", "VENDOR"}, rows); err != nil {
				return err
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

	// The ARP table holds devices this machine has already spoken to that did not
	// answer nmap's ARP probe during the sweep, which is ordinary behaviour for
	// hardware that sleeps to save power. Runs for any scan, not just "all",
	// because a single-network scan misses those devices in exactly the same way.
	// Restricted to the networks that were scanned, as the server does, so
	// container bridges and VPN peers cannot show up as LAN devices.
	if arp := s.DiscoverARP(context.Background(), scanned); len(arp) > 0 {
		if err := store.MergeSupplemental(arp); err != nil {
			fmt.Fprintf(os.Stderr, "Error saving ARP table entries: %v\n", err)
		} else {
			fmt.Printf("Found %s in the ARP table\n", count(len(arp), "device", "devices"))
		}
	}

	// A network nmap could not scan may still be partly known from the ARP
	// table. That is worth saving, but it is a fallback, not a scan, and is
	// reported as one rather than as a success.
	fallback := 0
	for _, f := range failed {
		arp := s.DiscoverARP(context.Background(), []string{f.cidr})
		if len(arp) == 0 {
			fmt.Fprintf(os.Stderr, "Scan failed for %s: %s\n", f.cidr, f.reason)
			continue
		}
		if err := store.MergeSupplemental(arp); err != nil {
			fmt.Fprintf(os.Stderr, "Scan failed for %s: %s\n", f.cidr, f.reason)
			fmt.Fprintf(os.Stderr, "Error saving ARP table entries: %v\n", err)
			continue
		}
		fmt.Fprintf(os.Stderr, "nmap failed for %s (%s); fell back to the ARP table and found %s\n", f.cidr, f.reason, count(len(arp), "device", "devices"))
		fallback++
	}

	// When scanning everything, also pick up IPv6 neighbors and mDNS
	// announcements, which cannot be reached by sweeping a range. IPv6 neighbors
	// go through MergeIPv6Neighbors, as the dashboard scan does, so a dual-stack
	// device is matched by MAC and enriched rather than listed twice. mDNS merges
	// as an ordinary secondary source.
	// An IPv6 network in the scanned set has already been through neighbour
	// discovery, so the post-pass would only repeat it.
	if target == "all" {
		ctx := context.Background()
		if !network.HasIPv6Network(scanned) {
			if ipv6 := s.DiscoverIPv6(ctx); len(ipv6) > 0 {
				if err := store.MergeIPv6Neighbors(ipv6); err != nil {
					fmt.Fprintf(os.Stderr, "Error saving IPv6 neighbors: %v\n", err)
				} else {
					fmt.Printf("Found %s\n", count(len(ipv6), "IPv6 neighbor", "IPv6 neighbors"))
				}
			}
		}
		if mdns := s.DiscoverMDNS(ctx); len(mdns) > 0 {
			if err := store.MergeSupplemental(mdns); err != nil {
				fmt.Fprintf(os.Stderr, "Error saving mDNS devices: %v\n", err)
			} else {
				fmt.Printf("Found %s\n", count(len(mdns), "mDNS device", "mDNS devices"))
			}
		}
	}

	// Exit status tells a script what it got: 0 when every network was
	// scanned, 1 when nothing came back at all, and 2 when nmap failed on every
	// network but the ARP table still gave a partial picture.
	if len(scanned) == 0 && len(failed) > 0 {
		if fallback == 0 {
			return fmt.Errorf("no network could be scanned")
		}
		fmt.Fprintln(os.Stderr, "nmap did not complete on any network; the results above come from the ARP table only")
		return exitCode(2)
	}
	return nil
}
