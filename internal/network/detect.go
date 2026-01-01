// Package network handles network interface detection and utilities
package network

import (
	"encoding/json"
	"fmt"
	"net"
	"os/exec"
	"runtime"
	"strings"

	"github.com/291-Group/LAN-Orangutan/internal/types"
)

// DetectNetworks discovers available network interfaces and their CIDRs
// Uses Go's standard library for cross-platform compatibility
func DetectNetworks() ([]types.Network, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, fmt.Errorf("failed to get interfaces: %w", err)
	}

	var networks []types.Network
	for _, iface := range ifaces {
		// Skip loopback and down interfaces
		if iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		if iface.Flags&net.FlagUp == 0 {
			continue
		}

		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}

		for _, addr := range addrs {
			ipNet, ok := addr.(*net.IPNet)
			if !ok {
				continue
			}

			// Skip IPv6
			if ipNet.IP.To4() == nil {
				continue
			}

			// Calculate network CIDR
			ones, _ := ipNet.Mask.Size()
			networkIP := ipNet.IP.Mask(ipNet.Mask)
			cidr := fmt.Sprintf("%s/%d", networkIP.String(), ones)

			network := types.Network{
				CIDR:         cidr,
				Interface:    iface.Name,
				FriendlyName: getFriendlyName(iface.Name),
				IP:           ipNet.IP.String(),
				IsTailscale:  isTailscaleInterface(iface.Name),
				IsWireless:   isWirelessInterface(iface.Name),
			}
			networks = append(networks, network)
		}
	}

	return networks, nil
}

// calculateCIDR calculates the network CIDR from an IP and prefix length
func calculateCIDR(ip string, prefixLen int) string {
	parsedIP := net.ParseIP(ip)
	if parsedIP == nil {
		return ""
	}

	ipv4 := parsedIP.To4()
	if ipv4 == nil {
		return ""
	}

	// Create subnet mask
	mask := net.CIDRMask(prefixLen, 32)

	// Calculate network address
	network := make(net.IP, 4)
	for i := 0; i < 4; i++ {
		network[i] = ipv4[i] & mask[i]
	}

	return fmt.Sprintf("%s/%d", network.String(), prefixLen)
}

// getFriendlyName returns a user-friendly name for an interface
func getFriendlyName(ifname string) string {
	switch {
	case strings.HasPrefix(ifname, "tailscale") || ifname == "utun4":
		return "Tailscale VPN"
	case strings.HasPrefix(ifname, "wlan") || strings.HasPrefix(ifname, "wlp"):
		return "Wi-Fi"
	case strings.HasPrefix(ifname, "eth") || strings.HasPrefix(ifname, "enp") || strings.HasPrefix(ifname, "eno"):
		return "Ethernet"
	// macOS interfaces
	case ifname == "en0":
		return "Wi-Fi" // Usually Wi-Fi on macOS
	case strings.HasPrefix(ifname, "en"):
		return "Ethernet" // Other en* interfaces are usually Ethernet
	case strings.HasPrefix(ifname, "bridge"):
		return "Bridge"
	case strings.HasPrefix(ifname, "br"):
		return "Bridge"
	case strings.HasPrefix(ifname, "docker"):
		return "Docker"
	case strings.HasPrefix(ifname, "veth"):
		return "Virtual Ethernet"
	case strings.HasPrefix(ifname, "virbr"):
		return "Virtual Bridge"
	case strings.HasPrefix(ifname, "utun"):
		return "VPN Tunnel"
	case strings.HasPrefix(ifname, "awdl"):
		return "Apple Wireless Direct"
	case strings.HasPrefix(ifname, "llw"):
		return "Low Latency WLAN"
	default:
		return ifname
	}
}

// isTailscaleInterface returns true if the interface is a Tailscale interface
func isTailscaleInterface(ifname string) bool {
	return strings.HasPrefix(ifname, "tailscale") || ifname == "utun4"
}

// isWirelessInterface returns true if the interface is a wireless interface
func isWirelessInterface(ifname string) bool {
	// Linux wireless interfaces
	if strings.HasPrefix(ifname, "wlan") || strings.HasPrefix(ifname, "wlp") {
		return true
	}
	// macOS - en0 is usually Wi-Fi
	if ifname == "en0" {
		return true
	}
	return false
}

// GetDefaultGateway returns the default gateway IP
func GetDefaultGateway() (string, error) {
	var cmd *exec.Cmd

	switch runtime.GOOS {
	case "darwin":
		// macOS: use netstat
		cmd = exec.Command("netstat", "-rn")
	case "linux":
		// Linux: try ip command first
		cmd = exec.Command("ip", "-j", "route", "show", "default")
	default:
		// Fallback for other systems
		cmd = exec.Command("netstat", "-rn")
	}

	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get default route: %w", err)
	}

	if runtime.GOOS == "linux" {
		// Parse JSON output from ip command
		var routes []struct {
			Gateway string `json:"gateway"`
		}
		if err := json.Unmarshal(output, &routes); err == nil && len(routes) > 0 {
			return routes[0].Gateway, nil
		}
	}

	// Parse netstat output (works on macOS and as fallback)
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) >= 2 && (fields[0] == "default" || fields[0] == "0.0.0.0") {
			gateway := fields[1]
			if net.ParseIP(gateway) != nil {
				return gateway, nil
			}
		}
	}

	return "", nil
}

// GetDNSServers returns configured DNS servers
func GetDNSServers() []string {
	// Try systemd-resolved first
	cmd := exec.Command("resolvectl", "status")
	output, err := cmd.Output()
	if err == nil {
		return parseDNSFromResolvectl(string(output))
	}

	// Fallback to /etc/resolv.conf
	return parseDNSFromResolvConf()
}

func parseDNSFromResolvectl(output string) []string {
	var servers []string
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		if strings.Contains(line, "DNS Servers:") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				for _, s := range strings.Fields(parts[1]) {
					if net.ParseIP(s) != nil {
						servers = append(servers, s)
					}
				}
			}
		}
	}
	return servers
}

func parseDNSFromResolvConf() []string {
	cmd := exec.Command("grep", "nameserver", "/etc/resolv.conf")
	output, err := cmd.Output()
	if err != nil {
		return nil
	}

	var servers []string
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[0] == "nameserver" {
			if net.ParseIP(fields[1]) != nil {
				servers = append(servers, fields[1])
			}
		}
	}
	return servers
}

// ValidateCIDR checks if a CIDR string is valid
func ValidateCIDR(cidr string) bool {
	_, _, err := net.ParseCIDR(cidr)
	return err == nil
}

// ParseIPRange parses a port range string (e.g., "1-1024")
func ParsePortRange(rangeStr string) (start, end int, err error) {
	parts := strings.Split(rangeStr, "-")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("invalid port range format")
	}

	var startPort, endPort int
	if _, err := fmt.Sscanf(parts[0], "%d", &startPort); err != nil {
		return 0, 0, fmt.Errorf("invalid start port")
	}
	if _, err := fmt.Sscanf(parts[1], "%d", &endPort); err != nil {
		return 0, 0, fmt.Errorf("invalid end port")
	}

	if startPort < 1 || startPort > 65535 || endPort < 1 || endPort > 65535 {
		return 0, 0, fmt.Errorf("port out of range")
	}
	if startPort > endPort {
		return 0, 0, fmt.Errorf("start port greater than end port")
	}

	return startPort, endPort, nil
}
