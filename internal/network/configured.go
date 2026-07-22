package network

import (
	"net"
	"strings"

	"github.com/291-Group/LAN-Orangutan/internal/types"
)

// WithConfigured returns the detected networks plus any the user has declared
// explicitly, with duplicates removed.
//
// Automatic detection reads the machine's own interfaces, which is wrong in a
// container: it sees only Docker's private network and never the LAN the user
// actually wants scanned. Declaring a network here makes it available to scan
// and visible on the dashboard even though no local interface belongs to it.
func WithConfigured(detected []types.Network, configured []string) []types.Network {
	// Docker's own bridge is a real interface but holds containers, not devices
	// on your network. Sweeping its /16 finds nothing and takes minutes.
	detected = ExcludeContainerNetworks(detected)

	seen := make(map[string]bool, len(detected)+len(configured))
	result := make([]types.Network, 0, len(detected)+len(configured))

	for _, n := range detected {
		if seen[n.CIDR] {
			continue
		}
		seen[n.CIDR] = true
		result = append(result, n)
	}

	for _, cidr := range configured {
		cidr = strings.TrimSpace(cidr)
		if cidr == "" || seen[cidr] {
			continue
		}
		if !ValidateCIDR(cidr) {
			continue
		}
		seen[cidr] = true
		result = append(result, types.Network{
			CIDR:         cidr,
			Interface:    "configured",
			FriendlyName: friendlyNameFor(cidr),
		})
	}

	return result
}

// friendlyNameFor labels a declared network so it is distinguishable from one
// that was detected.
func friendlyNameFor(cidr string) string {
	if ip, _, err := net.ParseCIDR(cidr); err == nil {
		return "Configured (" + ip.String() + ")"
	}
	return "Configured"
}

// ParseNetworkList splits a comma or space separated list of CIDRs, as supplied
// through the config file or the environment.
func ParseNetworkList(s string) []string {
	fields := strings.FieldsFunc(s, func(r rune) bool {
		return r == ',' || r == ' ' || r == '\t'
	})

	out := make([]string, 0, len(fields))
	for _, f := range fields {
		if f = strings.TrimSpace(f); f != "" {
			out = append(out, f)
		}
	}
	return out
}
