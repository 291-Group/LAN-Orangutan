package network

import (
	"net"
	"testing"

	"github.com/291-Group/LAN-Orangutan/internal/types"
)

// IsIPv6Network is the guard that keeps a /64 away from nmap. Getting it wrong
// in the permissive direction means asking a sweeping scanner to cover 2^64
// addresses, so the IPv4 cases matter as much as the IPv6 ones.
func TestIsIPv6Network(t *testing.T) {
	ipv6 := []string{
		"2001:db8:abcd:1::/64",
		"fd00:1234::/48",
		" 2606:4700:4700::/48 ",
		"::/0",
	}
	for _, c := range ipv6 {
		if !IsIPv6Network(c) {
			t.Errorf("expected %q to be treated as IPv6", c)
		}
	}

	notIPv6 := []string{
		"192.168.1.0/24",
		"10.0.0.0/8",
		"100.64.0.1/32",
		"",
		"not-a-cidr",
		"2001:db8::1", // an address, not a network
	}
	for _, c := range notIPv6 {
		if IsIPv6Network(c) {
			t.Errorf("expected %q not to be treated as an IPv6 network", c)
		}
	}
}

func TestIsRoutableIPv6(t *testing.T) {
	routable := []string{
		"2001:db8:abcd:1::1", // global unicast
		"fd00:1234::5",       // unique local
	}
	for _, s := range routable {
		if !isRoutableIPv6(net.ParseIP(s)) {
			t.Errorf("expected %s to be routable", s)
		}
	}

	// Every interface has a link-local address, they are not routable, and they
	// are the same everywhere, so presenting one as a network is noise.
	notRoutable := []string{
		"fe80::1",      // link local
		"::1",          // loopback
		"ff02::1",      // multicast
		"::",           // unspecified
		"192.168.1.10", // IPv4
	}
	for _, s := range notRoutable {
		if isRoutableIPv6(net.ParseIP(s)) {
			t.Errorf("expected %s not to be offered as a network", s)
		}
	}
	if isRoutableIPv6(nil) {
		t.Error("expected nil to be rejected")
	}
}

// ValidateCIDR accepts IPv6, which is why IsIPv6Network has to exist as a
// separate guard rather than relying on validation to keep nmap safe.
func TestValidateCIDRAcceptsIPv6SoTheGuardIsNeeded(t *testing.T) {
	if !ValidateCIDR("2001:db8::/64") {
		t.Fatal("ValidateCIDR unexpectedly rejects IPv6; the nmap guard assumes it does not")
	}
}

// Tailscale assigns both an IPv4 CGNAT address and an IPv6 ULA address. Both
// must be recognised, or the IPv6 half shows up as an ordinary network: a
// duplicate card on the dashboard, and a scan that goes to neighbour discovery
// instead of asking Tailscale, which already knows every node on the tailnet.
func TestTailscaleIsRecognisedInBothFamilies(t *testing.T) {
	if !IsTailscaleNetwork("100.64.0.0/10") {
		t.Error("expected the Tailscale IPv4 range to be recognised")
	}
	if !IsTailscaleNetwork("fd7a:115c:a1e0::/48") {
		t.Error("expected the Tailscale IPv6 range to be recognised")
	}
	if IsTailscaleNetwork("2606:6d00:f3e:cb00::/64") {
		t.Error("an ordinary global IPv6 prefix is not Tailscale")
	}
	if IsTailscaleNetwork("192.168.1.0/24") {
		t.Error("an ordinary LAN is not Tailscale")
	}
}

func TestIsTailscaleAddr(t *testing.T) {
	yes := []string{"100.111.185.80", "fd7a:115c:a1e0::1"}
	for _, s := range yes {
		if !isTailscaleAddr(net.ParseIP(s)) {
			t.Errorf("expected %s to be a Tailscale address", s)
		}
	}
	no := []string{"192.168.1.5", "2606:6d00:f3e:cb00::1", "fd00:1234::1"}
	for _, s := range no {
		if isTailscaleAddr(net.ParseIP(s)) {
			t.Errorf("expected %s not to be a Tailscale address", s)
		}
	}
	if isTailscaleAddr(nil) {
		t.Error("expected nil to be rejected")
	}
}

func TestHasIPv6Network(t *testing.T) {
	if HasIPv6Network([]string{"192.168.1.0/24", "10.0.0.0/8"}) {
		t.Error("an IPv4-only list has no IPv6 network")
	}
	if !HasIPv6Network([]string{"192.168.1.0/24", "2001:db8::/64"}) {
		t.Error("a list with a /64 has an IPv6 network")
	}
	if HasIPv6Network(nil) {
		t.Error("an empty list has no IPv6 network")
	}
}

// A dual-stack interface holds several addresses in the same /64 (a stable
// one, a temporary privacy one, deprecated ones on their way out). The prefix
// is one network and must be listed once, represented by the stable address.
func TestAppendNetworkDedupesPrefixPerInterface(t *testing.T) {
	flags := map[string]ipv6Flags{
		"2001:db8:1::aaaa": {temporary: true},
		"2001:db8:1::bbbb": {deprecated: true},
		"2001:db8:1::cccc": {},
	}
	mk := func(iface, cidr, ip string) types.Network {
		return types.Network{CIDR: cidr, Interface: iface, IP: ip, IsIPv6: true}
	}
	var nets []types.Network
	nets = appendNetwork(nets, mk("en0", "2001:db8:1::/64", "2001:db8:1::aaaa"), flags)
	nets = appendNetwork(nets, mk("en0", "2001:db8:1::/64", "2001:db8:1::bbbb"), flags)
	nets = appendNetwork(nets, mk("en0", "2001:db8:1::/64", "2001:db8:1::cccc"), flags)
	nets = appendNetwork(nets, mk("en0", "2001:db8:1::/64", "2001:db8:1::dddd"), flags)
	// The same prefix on another interface is a different network.
	nets = appendNetwork(nets, mk("en1", "2001:db8:1::/64", "2001:db8:1::eeee"), flags)

	if len(nets) != 2 {
		t.Fatalf("expected 2 networks, got %d: %+v", len(nets), nets)
	}
	if nets[0].Interface != "en0" || nets[0].IP != "2001:db8:1::cccc" {
		t.Errorf("en0 should be represented by the first stable address, got %+v", nets[0])
	}
	if nets[1].Interface != "en1" {
		t.Errorf("the en1 copy of the prefix should be kept separately, got %+v", nets[1])
	}
}

// Without lifetime flags (an OS that does not expose them) the first address
// seen stands for the prefix, and later ones do not displace it.
func TestAppendNetworkKeepsFirstWithoutFlags(t *testing.T) {
	var nets []types.Network
	nets = appendNetwork(nets, types.Network{CIDR: "fd00::/64", Interface: "eth0", IP: "fd00::1"}, nil)
	nets = appendNetwork(nets, types.Network{CIDR: "fd00::/64", Interface: "eth0", IP: "fd00::2"}, nil)
	if len(nets) != 1 || nets[0].IP != "fd00::1" {
		t.Errorf("expected the first address to stand for the prefix, got %+v", nets)
	}
}

func TestParseProcIfInet6(t *testing.T) {
	data := `20010db8000100000000000000000001 02 40 00 80 eth0
20010db8000100001234567890abcdef 02 40 00 01 eth0
20010db800010000aaaaaaaaaaaaaaaa 02 40 00 21 eth0
fe8000000000000000000000000000ab 02 40 20 80 eth0
garbage line`
	flags := parseProcIfInet6(data)
	if f := flags["2001:db8:1::1"]; f.temporary || f.deprecated {
		t.Errorf("stable address flagged: %+v", f)
	}
	if f := flags["2001:db8:1:0:1234:5678:90ab:cdef"]; !f.temporary || f.deprecated {
		t.Errorf("temporary address not recognised: %+v", f)
	}
	if f := flags["2001:db8:1:0:aaaa:aaaa:aaaa:aaaa"]; !f.temporary || !f.deprecated {
		t.Errorf("deprecated temporary address not recognised: %+v", f)
	}
	if len(flags) != 4 {
		t.Errorf("expected 4 parsed addresses, got %d", len(flags))
	}
}

func TestParseIfconfigInet6(t *testing.T) {
	out := `en0: flags=8863<UP,BROADCAST,SMART,RUNNING,SIMPLEX,MULTICAST> mtu 1500
	inet6 fe80::1c2a:3b4c:5d6e:7f80%en0 prefixlen 64 secured scopeid 0xb
	inet6 2001:db8:1::1 prefixlen 64 autoconf secured
	inet6 2001:db8:1:0:1234:5678:90ab:cdef prefixlen 64 autoconf temporary
	inet6 2001:db8:1:0:aaaa:aaaa:aaaa:aaaa prefixlen 64 deprecated autoconf temporary
	inet 192.168.1.5 netmask 0xffffff00 broadcast 192.168.1.255`
	flags := parseIfconfigInet6(out)
	if f, ok := flags["fe80::1c2a:3b4c:5d6e:7f80"]; !ok || f.temporary {
		t.Errorf("zone suffix not stripped or link-local misflagged: %+v %v", f, ok)
	}
	if f := flags["2001:db8:1::1"]; f.temporary || f.deprecated {
		t.Errorf("stable address flagged: %+v", f)
	}
	if f := flags["2001:db8:1:0:1234:5678:90ab:cdef"]; !f.temporary {
		t.Error("temporary address not recognised")
	}
	if f := flags["2001:db8:1:0:aaaa:aaaa:aaaa:aaaa"]; !f.temporary || !f.deprecated {
		t.Errorf("deprecated temporary address not recognised: %+v", f)
	}
	if _, ok := flags["192.168.1.5"]; ok {
		t.Error("an inet line is not an IPv6 address")
	}
}
