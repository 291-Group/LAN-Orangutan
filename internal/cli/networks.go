package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/291-Group/LAN-Orangutan/internal/network"
	"github.com/291-Group/LAN-Orangutan/internal/storage"
)

var networksCmd = &cobra.Command{
	Use:   "networks",
	Short: "List detected network interfaces",
	RunE:  runNetworks,
}

func runNetworks(cmd *cobra.Command, args []string) error {
	networks, err := network.DetectNetworks()
	if err != nil {
		return fmt.Errorf("failed to detect networks: %w", err)
	}

	// Resolve the effective filter, honouring any UI override saved in the data
	// directory; fall back to the config values if the store cannot be opened.
	filter := network.Filter{
		Configured:     cfg.Scanning.Networks,
		Excluded:       cfg.Scanning.ExcludeNetworks,
		OnlyConfigured: cfg.Scanning.OnlyConfiguredNetworks,
	}
	if store, serr := storage.New(cfg.DevicesFile(), cfg.StateFile()); serr == nil {
		filter = networkFilter(store, cfg)
	}
	networks = network.WithConfigured(networks, filter)

	if len(networks) == 0 {
		fmt.Println("No networks detected")
		return nil
	}

	fmt.Println("Detected Networks:")
	fmt.Println()

	for _, n := range networks {
		fmt.Printf("  %s\n", n.CIDR)
		fmt.Printf("    Interface: %s\n", n.Interface)
		fmt.Printf("    Type: %s\n", n.FriendlyName)
		fmt.Printf("    IP: %s\n", n.IP)
		if n.IsTailscale {
			fmt.Printf("    Tailscale: yes\n")
		}
		if n.IsWireless {
			fmt.Printf("    Wireless: yes\n")
		}
		fmt.Println()
	}

	// Show gateway and DNS
	gateway, err := network.GetDefaultGateway()
	if err == nil && gateway != "" {
		fmt.Printf("Default Gateway: %s\n", gateway)
	}

	dns := network.GetDNSServers()
	if len(dns) > 0 {
		fmt.Printf("DNS Servers: %v\n", dns)
	}

	return nil
}
