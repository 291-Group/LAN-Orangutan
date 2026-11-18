// Package network handles network interface detection and utilities
package network

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"runtime"
	"strconv"
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

	flags := ipv6AddrFlags()
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

			isV6 := ipNet.IP.To4() == nil
			// A link-local IPv6 address is not a network anyone can act on:
			// every interface has one, they are not routable, and they are
			// identical across machines. Loopback, multicast and the
			// unspecified address are not networks either.
			if isV6 && !isRoutableIPv6(ipNet.IP) {
				continue
			}

			// Calculate network CIDR
			ones, _ := ipNet.Mask.Size()
			networkIP := ipNet.IP.Mask(ipNet.Mask)
			cidr := fmt.Sprintf("%s/%d", networkIP.String(), ones)

			network := types.Network{
				CIDR:         cidr,
				Interface:    iface.Name,
				FriendlyName: getFriendlyName(iface.Name, ipNet.IP),
				IP:           ipNet.IP.String(),
				IsTailscale:  isTailscaleInterface(iface.Name, ipNet.IP),
				IsWireless:   isWirelessInterface(iface.Name),
				IsIPv6:       isV6,
			}
			networks = appendNetwork(networks, network, flags)
		}
	}

	return networks, nil
}

// appendNetwork adds n to the list unless the same prefix on the same
// interface is already there. A dual-stack interface carries several
// addresses in one IPv6 /64 (a stable one, a temporary privacy one, and
// deprecated ones on their way out), and listing the prefix once per address
// shows the user the same network three or four times. When the prefix is
// already listed, n replaces the entry only if it is a better address to show
// for it: one the OS does not mark temporary or deprecated.
func appendNetwork(networks []types.Network, n types.Network, flags map[string]ipv6Flags) []types.Network {
	for i, existing := range networks {
		if existing.Interface != n.Interface || existing.CIDR != n.CIDR {
			continue
		}
		if addrRank(n.IP, flags) < addrRank(existing.IP, flags) {
			networks[i] = n
		}
		return networks
	}
	return append(networks, n)
}

// ipv6Flags is what the OS says about an address's lifetime, where it says
// anything at all.
type ipv6Flags struct {
	temporary  bool
	deprecated bool
}

// addrRank orders addresses for choosing which one represents a prefix: lower
// is better. A deprecated address is about to disappear and a temporary one
// rotates, so a stable address is preferred when the OS tells them apart.
func addrRank(ip string, flags map[string]ipv6Flags) int {
	f := flags[ip]
	rank := 0
	if f.temporary {
		rank += 1
	}
	if f.deprecated {
		rank += 2
	}
	return rank
}

// ipv6AddrFlags reads the lifetime flags of every IPv6 address on the host,
// keyed by address. Go's net package does not expose them, so they come from
// /proc on Linux and from ifconfig on macOS; elsewhere the map is empty and
// the first address listed for a prefix is the one shown.
func ipv6AddrFlags() map[string]ipv6Flags {
	switch runtime.GOOS {
	case "linux":
		data, err := os.ReadFile("/proc/net/if_inet6")
		if err != nil {
			return nil
		}
		return parseProcIfInet6(string(data))
	case "darwin":
		out, err := runCommand("ifconfig")
		if err != nil {
			return nil
		}
		return parseIfconfigInet6(string(out))
	}
	return nil
}

// Linux address flags as /proc/net/if_inet6 prints them (IFA_F_* in
// linux/if_addr.h).
const (
	ifaFlagTemporary  = 0x01
	ifaFlagDeprecated = 0x20
)

// parseProcIfInet6 parses /proc/net/if_inet6, one address per line as
// "<32 hex digits> <ifindex> <prefixlen> <scope> <flags> <ifname>".
func parseProcIfInet6(data string) map[string]ipv6Flags {
	flags := make(map[string]ipv6Flags)
	for _, line := range strings.Split(data, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 6 || len(fields[0]) != 32 {
			continue
		}
		raw, err := hex.DecodeString(fields[0])
		if err != nil {
			continue
		}
		bits, err := strconv.ParseUint(fields[4], 16, 32)
		if err != nil {
			continue
		}
		flags[net.IP(raw).String()] = ipv6Flags{
			temporary:  bits&ifaFlagTemporary != 0,
			deprecated: bits&ifaFlagDeprecated != 0,
		}
	}
	return flags
}

// parseIfconfigInet6 parses macOS ifconfig output, where an address line reads
// "inet6 2001:db8::1 prefixlen 64 autoconf secured" and the words after the
// prefix length include "temporary" or "deprecated" when they apply. The
// zone suffix ("%en0") on link-local addresses is dropped, matching how the
// net package prints them.
func parseIfconfigInet6(out string) map[string]ipv6Flags {
	flags := make(map[string]ipv6Flags)
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 || fields[0] != "inet6" {
			continue
		}
		addr := fields[1]
		if i := strings.IndexByte(addr, '%'); i >= 0 {
			addr = addr[:i]
		}
		ip := net.ParseIP(addr)
		if ip == nil {
			continue
		}
		var f ipv6Flags
		for _, w := range fields[2:] {
			switch w {
			case "temporary":
				f.temporary = true
			case "deprecated":
				f.deprecated = true
			}
		}
		flags[ip.String()] = f
	}
	return flags
}

// getFriendlyName returns a user-friendly name for an interface
func getFriendlyName(ifname string, ip net.IP) string {
	switch {
	case isTailscaleInterface(ifname, ip):
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

// tailscaleCGNAT is the IPv4 range Tailscale assigns to every node.
var tailscaleCGNAT = &net.IPNet{
	IP:   net.IPv4(100, 64, 0, 0),
	Mask: net.CIDRMask(10, 32),
}

// tailscaleULA is the IPv6 range Tailscale assigns alongside the IPv4 address.
// Unlike the utun device number, this prefix is fixed, so it identifies a
// Tailscale address reliably. Tailscale already knows every node on the
// tailnet, so a network in this range is asked rather than discovered.
var tailscaleULA = func() *net.IPNet {
	_, n, _ := net.ParseCIDR("fd7a:115c:a1e0::/48")
	return n
}()

// isTailscaleAddr reports whether ip is an address Tailscale assigned, in
// either family.
func isTailscaleAddr(ip net.IP) bool {
	if ip == nil {
		return false
	}
	if v4 := ip.To4(); v4 != nil {
		return tailscaleCGNAT.Contains(v4)
	}
	return tailscaleULA.Contains(ip)
}

// IsTailscaleNetwork reports whether a CIDR belongs to the Tailscale network.
// Tailscale hands every node a single-address /32, so such a network cannot be
// swept for devices; its peers have to be read from Tailscale itself.
func IsTailscaleNetwork(cidr string) bool {
	ip, _, err := net.ParseCIDR(cidr)
	if err != nil {
		return false
	}
	return isTailscaleAddr(ip)
}

// isTailscaleInterface returns true if the interface is a Tailscale interface.
//
// The interface name alone is not enough to tell: Linux uses "tailscale0", but
// macOS uses a utun device whose number depends on how many tunnels happen to
// exist, so it is not always utun4. Fall back to checking whether the address
// sits in the range Tailscale assigns, which is what actually identifies it.
// The range is only trusted on a tunnel interface, since it is shared with
// carrier-grade NAT that an ISP could legitimately hand out on a real NIC.
func isTailscaleInterface(ifname string, ip net.IP) bool {
	if strings.HasPrefix(strings.ToLower(ifname), "tailscale") {
		return true
	}
	if !strings.HasPrefix(ifname, "utun") && !strings.HasPrefix(ifname, "tun") {
		return false
	}
	return isTailscaleAddr(ip)
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
	var (
		name string
		args []string
	)

	switch runtime.GOOS {
	case "darwin":
		// macOS: use netstat
		name, args = "netstat", []string{"-rn"}
	case "linux":
		// Linux: try ip command first
		name, args = "ip", []string{"-j", "route", "show", "default"}
	default:
		// Fallback for other systems
		name, args = "netstat", []string{"-rn"}
	}

	output, err := runCommand(name, args...)
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
	output, err := runCommand("resolvectl", "status")
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
	output, err := runCommand("grep", "nameserver", "/etc/resolv.conf")
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

// isRoutableIPv6 reports whether ip is an IPv6 address worth presenting as a
// network: a routable unicast address, either global or unique-local. Link
// local is excluded on purpose, since every interface has one and none of them
// identifies a network a user can do anything with.
func isRoutableIPv6(ip net.IP) bool {
	if ip == nil || ip.To4() != nil {
		return false
	}
	return !ip.IsLoopback() && !ip.IsMulticast() &&
		!ip.IsUnspecified() && !ip.IsLinkLocalUnicast()
}

// IsIPv6Network reports whether cidr names an IPv6 network.
//
// This is the guard that keeps an IPv6 prefix away from nmap. A /64 holds 2^64
// addresses, so asking a sweeping scanner to cover one is not a slow scan, it
// is an impossible one. Callers route these to neighbour discovery instead.
func IsIPv6Network(cidr string) bool {
	ip, _, err := net.ParseCIDR(strings.TrimSpace(cidr))
	if err != nil {
		return false
	}
	return ip.To4() == nil
}

// HasIPv6Network reports whether any of cidrs names an IPv6 network. A scan
// of an IPv6 network already solicits and reads the neighbour cache, so a
// caller that runs a separate IPv6 pass after scanning should skip it when
// this is true, or every neighbour is found and counted twice.
func HasIPv6Network(cidrs []string) bool {
	for _, c := range cidrs {
		if IsIPv6Network(c) {
			return true
		}
	}
	return false
}

// ParsePortRange parses a port range string (e.g., "1-1024")
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
