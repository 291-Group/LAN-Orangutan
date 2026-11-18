package scanner

import (
	"context"
	"net"
	"runtime"
	"strings"

	"github.com/291-Group/LAN-Orangutan/internal/types"
)

// DiscoverARP reads this machine's IPv4 ARP table and returns the entries that
// fall inside cidrs.
//
// A scan runs `nmap -sn`, which on a local subnet finds hosts by sending ARP
// requests and listening for replies. A device that does not answer an ARP
// request is invisible to it, and cheap WiFi hardware that sleeps to save power
// routinely does not, even while it still answers an ordinary ping. The machine
// running the scan has usually spoken to that device already and still holds it
// in its own ARP table, so the device was never actually unknown; we only never
// looked. Reported and diagnosed by the user who opened #48.
//
// This is deliberately a supplemental source. An entry carries an IPv4 address
// and a hardware address and nothing else, so results go through
// MergeSupplemental to fill gaps rather than overwrite what a real scan found.
//
// Results are restricted to the networks actually being scanned. The ARP table
// also holds Docker bridge gateways, VPN peers and every other address this
// host has spoken to, and presenting those as devices on the user's LAN would
// be wrong in exactly the way the IPv6 link-local phantoms were.
func (s *Scanner) DiscoverARP(ctx context.Context, cidrs []string) []types.Device {
	nets := parseCIDRList(cidrs)
	if len(nets) == 0 {
		return nil
	}

	var devices []types.Device
	for _, d := range readARPTable(ctx) {
		ip := net.ParseIP(d.IP)
		if ip == nil || !ipInAny(ip, nets) {
			continue
		}
		if d.Vendor == "" && d.MAC != "" {
			d.Vendor = GetMACVendor(d.MAC)
		}
		d.Type = Classify(d.Vendor, d.Hostname, nil)
		devices = append(devices, d)
	}
	return devices
}

// readARPTable reads and parses the OS ARP table. Linux is asked through `ip`
// first because `arp` is deprecated there and absent on minimal images; the
// BSD-style `arp -an` is kept as a fallback for older systems.
func readARPTable(ctx context.Context) []types.Device {
	switch runtime.GOOS {
	case "linux":
		if out, err := runNeighCommand(ctx, "ip", "-4", "neigh", "show"); err == nil {
			if devices := parseIPNeigh4(string(out)); len(devices) > 0 {
				return devices
			}
		}
		out, err := runNeighCommand(ctx, "arp", "-an")
		if err != nil {
			return nil
		}
		return parseARPBSD(string(out))
	case "darwin":
		out, err := runNeighCommand(ctx, "arp", "-an")
		if err != nil {
			return nil
		}
		return parseARPBSD(string(out))
	case "windows":
		out, err := runNeighCommand(ctx, "arp", "-a")
		if err != nil {
			return nil
		}
		return parseARPWindows(string(out))
	default:
		return nil
	}
}

// parseIPNeigh4 parses Linux `ip -4 neigh show`, keeping only the entries
// whose state is current evidence of presence (see neighStatePresent).
func parseIPNeigh4(output string) []types.Device {
	var devices []types.Device
	for _, line := range strings.Split(output, "\n") {
		ip, mac, state, ok := parseIPNeighLine(line)
		if !ok || !neighStatePresent(state) {
			continue
		}
		if d, ok := arpEntry(ip, mac); ok {
			devices = append(devices, d)
		}
	}
	return devices
}

// parseARPBSD parses macOS and BSD `arp -an`. A line reads
// "? (192.168.1.5) at 0:11:22:33:44:55 on en0 ifscope [ethernet]", and an
// unresolved entry carries "(incomplete)" where the address would be. FreeBSD
// marks an entry past its lifetime with "expired"; such an entry is a memory
// of a device, not a sighting, so it is skipped.
func parseARPBSD(output string) []types.Device {
	var devices []types.Device
	for _, line := range strings.Split(output, "\n") {
		openIdx := strings.IndexByte(line, '(')
		closeIdx := strings.IndexByte(line, ')')
		if openIdx < 0 || closeIdx <= openIdx+1 {
			continue
		}
		ip := line[openIdx+1 : closeIdx]

		rest := strings.Fields(line[closeIdx+1:])
		mac := ""
		expired := false
		for i, f := range rest {
			if f == "at" && i+1 < len(rest) {
				mac = rest[i+1]
			}
			if f == "expired" {
				expired = true
			}
		}
		if expired {
			continue
		}
		if d, ok := arpEntry(ip, mac); ok {
			devices = append(devices, d)
		}
	}
	return devices
}

// parseARPWindows parses Windows `arp -a`. Entries are indented under a
// per-interface header and read "  192.168.1.5    00-11-22-33-44-55   dynamic".
// Windows separates octets with hyphens, and lists static entries for broadcast
// and multicast addresses that arpEntry rejects. A "static" row with a unicast
// address is one the user pinned by hand and an "invalid" row is one the stack
// gave up on; neither says the device is there, so both are skipped. The type
// column is localised on non-English Windows, where this check cannot apply
// and the hardware-address filter is what remains.
func parseARPWindows(output string) []types.Device {
	var devices []types.Device
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		if len(fields) >= 3 {
			switch strings.ToLower(fields[2]) {
			case "static", "invalid":
				continue
			}
		}
		if d, ok := arpEntry(fields[0], strings.ReplaceAll(fields[1], "-", ":")); ok {
			devices = append(devices, d)
		}
	}
	return devices
}

// arpEntry validates one parsed row and builds a device from it. A row is kept
// only when it names a real IPv4 host and a real unicast hardware address, so
// broadcast, multicast and unresolved rows never become devices.
func arpEntry(ipStr, mac string) (types.Device, bool) {
	ip := net.ParseIP(ipStr)
	if ip == nil || ip.To4() == nil {
		return types.Device{}, false
	}
	if ip.IsLoopback() || ip.IsMulticast() || ip.IsUnspecified() {
		return types.Device{}, false
	}
	mac = normalizeMAC(mac)
	if !isUnicastMAC(mac) {
		return types.Device{}, false
	}
	return types.Device{IP: ip.String(), MAC: mac}, true
}

// isUnicastMAC reports whether mac is a real per-device hardware address.
//
// The ARP table lists more than hosts. The broadcast address appears on every
// system, IPv4 multicast groups appear as 01:00:5E:xx:xx:xx, and an entry the
// kernel never resolved shows as "(incomplete)" or similar. The low bit of the
// first octet is the IEEE group bit: set means multicast or broadcast, clear
// means a single machine, which is the only kind worth recording.
func isUnicastMAC(mac string) bool {
	parts := strings.Split(mac, ":")
	if len(parts) != 6 {
		return false
	}
	for _, p := range parts {
		if len(p) != 2 {
			return false
		}
		for i := 0; i < 2; i++ {
			c := p[i]
			if (c < '0' || c > '9') && (c < 'A' || c > 'F') {
				return false
			}
		}
	}
	// The group bit is bit 0 of the first octet, which is the low bit of that
	// octet's second hex digit.
	switch c := parts[0][1]; {
	case c >= '0' && c <= '9':
		if (c-'0')&1 != 0 {
			return false
		}
	default:
		if (c-'A'+10)&1 != 0 {
			return false
		}
	}
	return mac != "00:00:00:00:00:00"
}

// parseCIDRList turns the scanned network list into matchable networks,
// ignoring anything that will not parse rather than failing the whole read.
func parseCIDRList(cidrs []string) []*net.IPNet {
	var nets []*net.IPNet
	for _, c := range cidrs {
		if _, n, err := net.ParseCIDR(strings.TrimSpace(c)); err == nil {
			nets = append(nets, n)
		}
	}
	return nets
}

func ipInAny(ip net.IP, nets []*net.IPNet) bool {
	for _, n := range nets {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}
