// Package network handles network interface detection and utilities
package network

import (
	"encoding/json"
	"fmt"
	"net"
	"os/exec"
	"strings"

	"github.com/291-Group/LAN-Orangutan/internal/types"
)

// ipAddrInfo represents the JSON output from `ip -j addr show`
type ipAddrInfo struct {
	IfIndex   int    `json:"ifindex"`
	IfName    string `json:"ifname"`
	Flags     []string `json:"flags"`
	AddrInfo  []addrInfo `json:"addr_info"`
}

type addrInfo struct {
	Family    string `json:"family"`
	Local     string `json:"local"`
	PrefixLen int    `json:"prefixlen"`
}

// DetectNetworks discovers available network interfaces and their CIDRs
func DetectNetworks() ([]types.Network, error) {
	cmd := exec.Command("ip", "-j", "addr", "show")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to run ip command: %w", err)
	}

	var interfaces []ipAddrInfo
	if err := json.Unmarshal(output, &interfaces); err != nil {
		return nil, fmt.Errorf("failed to parse ip output: %w", err)
	}

	var networks []types.Network
	for _, iface := range interfaces {
		// Skip loopback and down interfaces
		if iface.IfName == "lo" {
			continue
		}
		isUp := false
		for _, flag := range iface.Flags {
			if flag == "UP" {
				isUp = true
				break
			}
		}
		if !isUp {
			continue
		}

		for _, addr := range iface.AddrInfo {
			if addr.Family != "inet" {
				continue
			}

			cidr := calculateCIDR(addr.Local, addr.PrefixLen)
			if cidr == "" {
				continue
			}

			network := types.Network{
				CIDR:         cidr,
				Interface:    iface.IfName,
				FriendlyName: getFriendlyName(iface.IfName),
				IP:           addr.Local,
				IsTailscale:  isTailscaleInterface(iface.IfName),
				IsWireless:   isWirelessInterface(iface.IfName),
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
	case strings.HasPrefix(ifname, "tailscale"):
		return "Tailscale VPN"
	case strings.HasPrefix(ifname, "wlan") || strings.HasPrefix(ifname, "wlp"):
		return "Wi-Fi"
	case strings.HasPrefix(ifname, "eth") || strings.HasPrefix(ifname, "enp") || strings.HasPrefix(ifname, "eno"):
		return "Ethernet"
	case strings.HasPrefix(ifname, "br"):
		return "Bridge"
	case strings.HasPrefix(ifname, "docker"):
		return "Docker"
	case strings.HasPrefix(ifname, "veth"):
		return "Virtual Ethernet"
	case strings.HasPrefix(ifname, "virbr"):
		return "Virtual Bridge"
	default:
		return ifname
	}
}

// isTailscaleInterface returns true if the interface is a Tailscale interface
func isTailscaleInterface(ifname string) bool {
	return strings.HasPrefix(ifname, "tailscale")
}

// isWirelessInterface returns true if the interface is a wireless interface
func isWirelessInterface(ifname string) bool {
	return strings.HasPrefix(ifname, "wlan") || strings.HasPrefix(ifname, "wlp")
}

// GetDefaultGateway returns the default gateway IP
func GetDefaultGateway() (string, error) {
	cmd := exec.Command("ip", "-j", "route", "show", "default")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get default route: %w", err)
	}

	var routes []struct {
		Gateway string `json:"gateway"`
	}
	if err := json.Unmarshal(output, &routes); err != nil {
		return "", fmt.Errorf("failed to parse route output: %w", err)
	}

	if len(routes) > 0 && routes[0].Gateway != "" {
		return routes[0].Gateway, nil
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
