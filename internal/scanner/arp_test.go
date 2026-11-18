package scanner

import (
	"net"
	"testing"
)

func TestParseIPNeigh4(t *testing.T) {
	// Real `ip -4 neigh show` output. STALE is the state a powered-off device
	// sits in for days, so it must not count as a sighting, and FAILED and
	// INCOMPLETE mean the kernel asked and never got an answer.
	out := `192.168.1.1 dev eth0 lladdr 94:83:c4:5e:d1:27 REACHABLE
192.168.1.42 dev eth0 lladdr 0:11:22:33:44:55 DELAY
192.168.1.43 dev eth0 lladdr 0:11:22:33:44:56 STALE
192.168.1.99 dev eth0  FAILED
192.168.1.77 dev eth0 lladdr 00:1a:2b:3c:4d:5e INCOMPLETE
192.168.1.2 dev eth0 lladdr 00:1a:2b:3c:4d:5f PERMANENT
192.168.1.50 dev eth0 lladdr aa:bb:cc:dd:ee:ff PROBE`

	devices := parseIPNeigh4(out)
	if len(devices) != 3 {
		t.Fatalf("expected 3 present entries, got %d: %+v", len(devices), devices)
	}
	if devices[0].IP != "192.168.1.1" || devices[0].MAC != "94:83:C4:5E:D1:27" {
		t.Errorf("first entry wrong: %+v", devices[0])
	}
	// A single-digit octet must be padded, or the vendor lookup misses.
	if devices[1].MAC != "00:11:22:33:44:55" {
		t.Errorf("short octet not normalized: %q", devices[1].MAC)
	}
	if devices[2].IP != "192.168.1.50" {
		t.Errorf("PROBE entry missing or misordered: %+v", devices)
	}
}

func TestNeighStatePresent(t *testing.T) {
	for _, st := range []string{"REACHABLE", "DELAY", "PROBE"} {
		if !neighStatePresent(st) {
			t.Errorf("%s should count as present", st)
		}
	}
	for _, st := range []string{"STALE", "FAILED", "INCOMPLETE", "PERMANENT", "NOARP", "NONE", ""} {
		if neighStatePresent(st) {
			t.Errorf("%s should not count as present", st)
		}
	}
}

func TestParseIPNeighLine(t *testing.T) {
	ip, mac, state, ok := parseIPNeighLine("fe80::1 dev eth0 lladdr 00:11:22:33:44:55 router REACHABLE")
	if !ok || ip != "fe80::1" || mac != "00:11:22:33:44:55" || state != "REACHABLE" {
		t.Errorf("flagged line parsed as %q %q %q %v", ip, mac, state, ok)
	}
	ip, mac, state, ok = parseIPNeighLine("192.168.1.99 dev eth0  FAILED")
	if !ok || ip != "192.168.1.99" || mac != "" || state != "FAILED" {
		t.Errorf("unresolved line parsed as %q %q %q %v", ip, mac, state, ok)
	}
	if _, _, _, ok := parseIPNeighLine("   "); ok {
		t.Error("a blank line is not an entry")
	}
}

func TestParseARPBSD(t *testing.T) {
	out := `? (192.168.8.1) at 94:83:c4:5e:d1:27 on en0 ifscope [ethernet]
? (192.168.8.188) at b8:27:eb:c8:f0:17 on en0 ifscope [ethernet]
? (192.168.8.55) at (incomplete) on en0 ifscope [ethernet]
? (192.168.8.60) at 00:1a:2b:3c:4d:5e on em0 expired [ethernet]
? (224.0.0.251) at 1:0:5e:0:0:fb on en0 ifscope permanent [ethernet]
? (192.168.8.255) at ff:ff:ff:ff:ff:ff on en0 ifscope [ethernet]`

	devices := parseARPBSD(out)
	if len(devices) != 2 {
		t.Fatalf("expected 2 usable entries, got %d: %+v", len(devices), devices)
	}
	if devices[1].IP != "192.168.8.188" || devices[1].MAC != "B8:27:EB:C8:F0:17" {
		t.Errorf("second entry wrong: %+v", devices[1])
	}
}

func TestParseARPWindows(t *testing.T) {
	out := `
Interface: 192.168.8.224 --- 0x5
  Internet Address      Physical Address      Type
  192.168.8.1           94-83-c4-5e-d1-27     dynamic
  192.168.8.188         b8-27-eb-c8-f0-17     dynamic
  192.168.8.200         00-1a-2b-3c-4d-5e     static
  192.168.8.201         00-00-00-00-00-00     invalid
  192.168.8.255         ff-ff-ff-ff-ff-ff     static
  224.0.0.22            01-00-5e-00-00-16     static`

	devices := parseARPWindows(out)
	if len(devices) != 2 {
		t.Fatalf("expected 2 usable entries, got %d: %+v", len(devices), devices)
	}
	if devices[0].MAC != "94:83:C4:5E:D1:27" {
		t.Errorf("hyphenated MAC not converted: %q", devices[0].MAC)
	}
	// The header row names a column, not a host, and must not survive.
	for _, d := range devices {
		if d.IP == "Internet" {
			t.Errorf("header row parsed as a device: %+v", d)
		}
	}
}

func TestIsUnicastMAC(t *testing.T) {
	good := []string{"94:83:C4:5E:D1:27", "B8:27:EB:C8:F0:17", "02:42:AC:11:00:02"}
	for _, m := range good {
		if !isUnicastMAC(m) {
			t.Errorf("expected %q to be a unicast MAC", m)
		}
	}
	bad := []string{
		"FF:FF:FF:FF:FF:FF", // broadcast
		"01:00:5E:00:00:FB", // IPv4 multicast
		"00:00:00:00:00:00", // unresolved
		"(INCOMPLETE)",      // macOS placeholder
		"94:83:C4:5E:D1",    // too short
		"ZZ:83:C4:5E:D1:27", // not hex
	}
	for _, m := range bad {
		if isUnicastMAC(m) {
			t.Errorf("expected %q to be rejected", m)
		}
	}
}

func TestArpEntryRejectsNonHosts(t *testing.T) {
	cases := []struct{ ip, mac string }{
		{"127.0.0.1", "94:83:C4:5E:D1:27"},   // loopback
		{"224.0.0.251", "01:00:5E:00:00:FB"}, // multicast
		{"0.0.0.0", "94:83:C4:5E:D1:27"},     // unspecified
		{"fe80::1", "94:83:C4:5E:D1:27"},     // not IPv4
		{"Internet", "Physical"},             // header row
		{"192.168.1.5", "ff:ff:ff:ff:ff:ff"}, // broadcast MAC
	}
	for _, c := range cases {
		if _, ok := arpEntry(c.ip, c.mac); ok {
			t.Errorf("expected %q / %q to be rejected", c.ip, c.mac)
		}
	}
	if d, ok := arpEntry("192.168.1.5", "0:11:22:33:44:55"); !ok {
		t.Error("expected a real host to be accepted")
	} else if d.MAC != "00:11:22:33:44:55" {
		t.Errorf("MAC not normalized: %q", d.MAC)
	}
}

// The ARP table holds every address this host has spoken to, including Docker
// bridges and VPN peers. Only addresses inside a network actually being scanned
// may become devices, or a scan of the LAN starts reporting the container
// gateway as something on the user's network.
func TestIPInAnyRestrictsToScannedNetworks(t *testing.T) {
	nets := parseCIDRList([]string{"192.168.8.0/24", " 10.0.5.0/24 ", "not-a-cidr"})
	if len(nets) != 2 {
		t.Fatalf("expected 2 parsed networks, got %d", len(nets))
	}

	in := []string{"192.168.8.1", "192.168.8.254", "10.0.5.7"}
	for _, s := range in {
		if !ipInAny(net.ParseIP(s), nets) {
			t.Errorf("expected %s to be inside the scanned networks", s)
		}
	}
	out := []string{"172.17.0.1", "100.124.165.42", "192.168.9.1"}
	for _, s := range out {
		if ipInAny(net.ParseIP(s), nets) {
			t.Errorf("expected %s to be outside the scanned networks", s)
		}
	}
}

func TestParseCIDRListEmpty(t *testing.T) {
	if nets := parseCIDRList(nil); len(nets) != 0 {
		t.Errorf("expected no networks from nil, got %d", len(nets))
	}
}
