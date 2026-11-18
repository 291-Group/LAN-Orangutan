package scanner

import (
	"net"
	"strings"
	"testing"
	"time"
)

func TestNmapArgsLocalNetworkUnchanged(t *testing.T) {
	got := strings.Join(nmapArgs("192.168.1.0/24", true, time.Second, 2), " ")
	if got != "-sn -oX - 192.168.1.0/24" {
		t.Errorf("local network args = %q, want the plain ping sweep", got)
	}
}

func TestNmapArgsRoutedNetworkGetsTiming(t *testing.T) {
	got := strings.Join(nmapArgs("10.253.0.0/24", false, 200*time.Millisecond, 1), " ")
	want := "-sn --max-rtt-timeout 200ms --max-retries 1 -oX - 10.253.0.0/24"
	if got != want {
		t.Errorf("routed args = %q, want %q", got, want)
	}
}

func TestRoutedTimingDefaultsAndOverride(t *testing.T) {
	s := New(0, false)
	if rtt, retries := s.routedTiming(); rtt != DefaultRoutedRTT || retries != DefaultRoutedRetries {
		t.Errorf("defaults = %v/%d, want 1s/2", rtt, retries)
	}
	// A zero Scanner, which tests and ad hoc callers build directly, must get
	// the same defaults rather than "--max-retries 0".
	var zero Scanner
	if rtt, retries := zero.routedTiming(); rtt != DefaultRoutedRTT || retries != DefaultRoutedRetries {
		t.Errorf("zero Scanner = %v/%d, want 1s/2", rtt, retries)
	}
	s.SetRoutedTiming(200*time.Millisecond, 1)
	if rtt, retries := s.routedTiming(); rtt != 200*time.Millisecond || retries != 1 {
		t.Errorf("override = %v/%d, want 200ms/1", rtt, retries)
	}
	// Zero retries is a real setting, not "unset".
	s.SetRoutedTiming(0, 0)
	if rtt, retries := s.routedTiming(); rtt != time.Second || retries != 0 {
		t.Errorf("rtt unset, retries 0 = %v/%d, want 1s/0", rtt, retries)
	}
	// -1 puts retries back on automatic.
	s.SetRoutedTiming(0, -1)
	if _, retries := s.routedTiming(); retries != DefaultRoutedRetries {
		t.Errorf("retries -1 = %d, want the default", retries)
	}
}

func TestOnLocalBroadcastNet(t *testing.T) {
	lan := ifaceNet{broadcast: true, ip: net.ParseIP("192.168.1.20")}
	wg := ifaceNet{broadcast: false, ip: net.ParseIP("10.253.0.2")}
	nets := []ifaceNet{lan, wg}

	if !onLocalBroadcastNet("192.168.1.0/24", nets) {
		t.Error("the LAN this machine is on should be local")
	}
	// The reporter's case: this machine has an address on the WireGuard network,
	// but the tunnel is point to point, so there is no ARP there.
	if onLocalBroadcastNet("10.253.0.0/24", nets) {
		t.Error("a network on a tunnel interface should count as routed")
	}
	if onLocalBroadcastNet("172.16.5.0/24", nets) {
		t.Error("a network this machine has no address in should count as routed")
	}
	if onLocalBroadcastNet("not-a-cidr", nets) {
		t.Error("an invalid CIDR should not count as local")
	}
}
