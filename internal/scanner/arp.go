package scanner

import (
	"context"
	"net"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/291-Group/LAN-Orangutan/internal/types"
)

const arpCommandTimeout = 3 * time.Second

// DiscoverIPv4 reads the local IPv4 neighbor cache and returns entries inside
// the networks just scanned. It does not probe the network: the cache supplies
// devices that are already known to the host but may not answer nmap's ping.
func (s *Scanner) DiscoverIPv4(ctx context.Context, networks []string) []types.Device {
	if ctx.Err() != nil {
		return nil
	}

	devices := filterIPv4Neighbors(readIPv4Neighbors(ctx), networks)
	for i := range devices {
		if devices[i].Vendor == "" && devices[i].MAC != "" {
			devices[i].Vendor = GetMACVendor(devices[i].MAC)
		}
		devices[i].Type = Classify(devices[i].Vendor, devices[i].Hostname, nil)
	}
	return devices
}

func readIPv4Neighbors(ctx context.Context) []types.Device {
	var output []byte
	var err error

	switch runtime.GOOS {
	case "linux":
		output, err = os.ReadFile("/proc/net/arp")
	case "darwin":
		output, err = runARPCommand(ctx, "arp", "-an")
	case "windows":
		output, err = runARPCommand(ctx, "arp", "-a")
	default:
		return nil
	}
	if err != nil {
		return nil
	}

	if runtime.GOOS == "linux" {
		return parseProcARP(string(output))
	}
	return parseARPOutput(string(output))
}

func runARPCommand(ctx context.Context, name string, args ...string) ([]byte, error) {
	commandCtx, cancel := context.WithTimeout(ctx, arpCommandTimeout)
	defer cancel()
	return exec.CommandContext(commandCtx, name, args...).Output()
}

// parseProcARP parses Linux /proc/net/arp, whose Flags column marks complete
// entries with 0x2. Incomplete requests have no usable link-layer address.
func parseProcARP(output string) []types.Device {
	var devices []types.Device
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 6 || fields[0] == "IP" {
			continue
		}

		flagsText := strings.TrimPrefix(strings.ToLower(fields[2]), "0x")
		flags, err := strconv.ParseUint(flagsText, 16, 32)
		if err != nil || flags&0x2 == 0 {
			continue
		}
		if device, ok := newIPv4Neighbor(fields[0], fields[3]); ok {
			devices = append(devices, device)
		}
	}
	return devices
}

// parseARPOutput parses the IPv4 rows emitted by macOS `arp -an` and Windows
// `arp -a`. Rows without a complete six-byte hardware address are ignored.
func parseARPOutput(output string) []types.Device {
	var devices []types.Device
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Fields(line)
		for i, field := range fields {
			ipText := strings.Trim(field, "(),")
			ip := net.ParseIP(ipText)
			if ip == nil || ip.To4() == nil {
				continue
			}

			for _, candidate := range fields[i+1:] {
				if device, ok := newIPv4Neighbor(ipText, strings.Trim(candidate, "(),")); ok {
					devices = append(devices, device)
					break
				}
			}
			break
		}
	}
	return devices
}

func newIPv4Neighbor(ipText, macText string) (types.Device, bool) {
	ip := net.ParseIP(ipText)
	if ip == nil || ip.To4() == nil || ip.IsUnspecified() || ip.IsLoopback() || ip.IsMulticast() {
		return types.Device{}, false
	}

	mac, err := net.ParseMAC(macText)
	if err != nil || len(mac) != 6 {
		return types.Device{}, false
	}
	zeroMAC := true
	for _, octet := range mac {
		if octet != 0 {
			zeroMAC = false
			break
		}
	}
	if zeroMAC {
		return types.Device{}, false
	}

	return types.Device{IP: ip.To4().String(), MAC: normalizeMAC(mac.String())}, true
}

func filterIPv4Neighbors(devices []types.Device, networks []string) []types.Device {
	var ranges []*net.IPNet
	for _, cidr := range networks {
		_, ipNet, err := net.ParseCIDR(cidr)
		if err == nil && ipNet.IP.To4() != nil {
			ranges = append(ranges, ipNet)
		}
	}
	if len(ranges) == 0 {
		return nil
	}

	seen := make(map[string]bool)
	filtered := make([]types.Device, 0, len(devices))
	for _, device := range devices {
		ip := net.ParseIP(device.IP)
		if ip == nil || ip.To4() == nil {
			continue
		}
		ip = ip.To4()
		if seen[ip.String()] {
			continue
		}
		for _, ipNet := range ranges {
			if ipNet.Contains(ip) {
				device.IP = ip.String()
				filtered = append(filtered, device)
				seen[device.IP] = true
				break
			}
		}
	}
	return filtered
}
