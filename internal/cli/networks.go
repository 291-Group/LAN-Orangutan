package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/291-Group/LAN-Orangutan/internal/network"
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
