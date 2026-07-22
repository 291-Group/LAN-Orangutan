package network

import (
	"net"
	"os"
	"strings"

	"github.com/291-Group/LAN-Orangutan/internal/types"
)

// InContainer reports whether this process is running inside a container.
func InContainer() bool {
	if _, err := os.Stat("/.dockerenv"); err == nil {
		return true
	}
	if data, err := os.ReadFile("/proc/1/cgroup"); err == nil {
		s := string(data)
		if strings.Contains(s, "docker") || strings.Contains(s, "containerd") || strings.Contains(s, "kubepods") {
			return true
		}
	}
	return false
}

// dockerPrivateRanges are the address ranges a container sees when it is
// attached to Docker's own network rather than the host's.
var dockerPrivateRanges = []string{
	"172.16.0.0/12",   // default bridge networks
	"192.168.65.0/24", // Docker Desktop's virtual machine
	"10.88.0.0/16",    // podman default
}

// isDockerPrivate reports whether a CIDR belongs to Docker's own networking
// rather than a real LAN.
func isDockerPrivate(cidr string) bool {
	ip, _, err := net.ParseCIDR(cidr)
	if err != nil {
		return false
	}
	for _, r := range dockerPrivateRanges {
		if _, block, err := net.ParseCIDR(r); err == nil && block.Contains(ip) {
			return true
		}
	}
	return false
}

// IsolatedFromLAN reports whether this process can only see container
// networking, with no route onto a real local network.
//
// In that state a scan is not merely empty: the virtual network's gateway
// answers probes on behalf of addresses that do not exist, so results contain
// devices that were never there. That is worse than finding nothing, because
// nothing about the output looks wrong.
func IsolatedFromLAN(detected []types.Network) bool {
	if !InContainer() {
		return false
	}
	if len(detected) == 0 {
		return true
	}
	for _, n := range detected {
		if !isDockerPrivate(n.CIDR) {
			return false
		}
	}
	return true
}

// IsolationWarning describes the problem in a form suitable for showing to a
// user. It returns an empty string when there is nothing wrong.
func IsolationWarning(detected []types.Network) string {
	if !IsolatedFromLAN(detected) {
		return ""
	}
	return "This container can only see Docker's own network, not your LAN. " +
		"Scans will miss your real devices and may report devices that do not exist. " +
		"Use host networking on Linux, or run LAN Orangutan directly on macOS and Windows."
}

// ExcludeContainerNetworks drops Docker's own bridge networks from a detected
// list.
//
// A host running Docker has a docker0 bridge, typically 172.17.0.0/16. It is a
// real interface, so detection finds it, but it holds containers rather than
// devices on your network. Scanning it sweeps 65,536 addresses to find nothing
// of interest, which on a Raspberry Pi takes minutes and makes "Scan All" look
// broken.
//
// Anyone who genuinely wants to scan it can still name it explicitly through
// the networks setting.
func ExcludeContainerNetworks(detected []types.Network) []types.Network {
	out := make([]types.Network, 0, len(detected))
	for _, n := range detected {
		if isDockerPrivate(n.CIDR) {
			continue
		}
		out = append(out, n)
	}
	return out
}
