package scanner

import "testing"

func TestParseIPNeigh(t *testing.T) {
	out := `2001:db8::5 dev eth0 lladdr aa:bb:cc:dd:ee:ff REACHABLE
2001:db8::6 dev eth0 lladdr aa:bb:cc:dd:ee:01 STALE
fe80::1 dev eth0 lladdr 00:11:22:33:44:55 router REACHABLE
fe80::dead dev eth0  FAILED
192.168.1.1 dev eth0 lladdr 11:22:33:44:55:66 REACHABLE
ff02::1 dev eth0 lladdr 33:33:00:00:00:01 NOARP
2001:db8::bad dev eth0 lladdr 22:33:44:55:66:77 FAILED`
	got := parseIPNeigh(out)

	// Only the routable, reachable IPv6 neighbor survives: link-local (fe80::),
	// the STALE one (it did not answer the solicitation), the FAILED ones, the
	// IPv4 one and the multicast one are all dropped.
	if len(got) != 1 {
		t.Fatalf("expected 1 device, got %d: %+v", len(got), got)
	}
	if got[0].IP != "2001:db8::5" || got[0].MAC != "AA:BB:CC:DD:EE:FF" {
		t.Errorf("device wrong: %+v", got[0])
	}
}

func TestParseNdp(t *testing.T) {
	out := `Neighbor                             Linklayer Address  Netif Expire    St Flgs Prbs
2001:db8::5%en0                      aa:bb:cc:dd:ee:ff  en0   permanent  R
fe80::1%en0                          0:11:22:33:44:55   en0   23h59m58s R  R
::1                                  (incomplete)       lo0   permanent  R
fe80::2%en0                          (incomplete)       en0   expired    R`
	got := parseNdp(out)

	// Link-local is dropped; only the routable neighbor remains.
	if len(got) != 1 {
		t.Fatalf("expected 1 device, got %d: %+v", len(got), got)
	}
	if got[0].IP != "2001:db8::5" || got[0].MAC != "AA:BB:CC:DD:EE:FF" {
		t.Errorf("device wrong: %+v", got[0])
	}
}

func TestIsUsableIPv6(t *testing.T) {
	cases := map[string]bool{
		"2001:db8::5": true,  // global unicast
		"fd00::1":     true,  // unique local (ULA)
		"fe80::1":     false, // link-local: never a device
		"::1":         false, // loopback
		"ff02::1":     false, // multicast
		"::":          false, // unspecified
		"192.168.1.1": false, // IPv4
		"not-an-ip":   false,
	}
	for ip, want := range cases {
		if got := isUsableIPv6(ip); got != want {
			t.Errorf("isUsableIPv6(%q) = %v, want %v", ip, got, want)
		}
	}
}

func TestNormalizeMAC(t *testing.T) {
	if got := normalizeMAC("0:11:22:33:44:5"); got != "00:11:22:33:44:05" {
		t.Errorf("got %q", got)
	}
	if got := normalizeMAC("aa:bb:cc:dd:ee:ff"); got != "AA:BB:CC:DD:EE:FF" {
		t.Errorf("got %q", got)
	}
}
