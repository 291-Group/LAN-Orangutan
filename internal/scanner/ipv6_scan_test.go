package scanner

import (
	"context"
	"testing"
	"time"

	"github.com/291-Group/LAN-Orangutan/internal/types"
)

// An IPv6 network must never reach nmap. A /64 holds 2^64 addresses, so a
// sweep is not slow, it is impossible, and nmap would either refuse or sit
// there. Scan must route it to neighbour discovery, and keep only the
// neighbours inside the prefix it was asked about.
func TestScanRoutesIPv6AwayFromNmap(t *testing.T) {
	solicited := 0
	s := &Scanner{discoverIPv6: func(context.Context) []types.Device {
		solicited++
		return []types.Device{
			{IP: "2001:db8:abcd:1::5", MAC: "AA:BB:CC:DD:EE:01"},
			{IP: "2001:db8:abcd:1::6", MAC: "AA:BB:CC:DD:EE:02"},
			{IP: "2001:db8:ffff::1", MAC: "AA:BB:CC:DD:EE:03"}, // another prefix
		}
	}}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	result, err := s.Scan(ctx, "2001:db8:abcd:1::/64")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Scanner != "neighbor discovery" {
		t.Errorf("expected the neighbour-discovery path, got scanner %q", result.Scanner)
	}
	if !result.Success {
		t.Errorf("expected success, got error %q", result.Error)
	}
	if solicited != 1 {
		t.Errorf("neighbours solicited %d times, want once", solicited)
	}
	if result.DeviceCount != 2 || len(result.Devices) != 2 {
		t.Fatalf("expected the 2 devices inside the prefix, got %d: %+v", result.DeviceCount, result.Devices)
	}
	for _, d := range result.Devices {
		if d.IP == "2001:db8:ffff::1" {
			t.Errorf("a neighbour outside the scanned prefix was returned: %+v", d)
		}
	}
}

// A prefix with no neighbours is an empty success rather than a failure.
func TestScanIPv6NetworkEmptyIsSuccess(t *testing.T) {
	s := &Scanner{discoverIPv6: func(context.Context) []types.Device { return nil }}
	result, err := s.Scan(context.Background(), "2001:db8:abcd:1::/64")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success || result.DeviceCount != 0 {
		t.Errorf("expected an empty success, got %+v", result)
	}
}
