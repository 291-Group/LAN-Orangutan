package storage

import (
	"testing"

	"github.com/291-Group/LAN-Orangutan/internal/types"
)

// TestDeviceMovingIPIsTrackedByMAC covers a device that reappears at a new IP:
// it is recognised by its MAC, its identity and user data move with it, the
// stale entry is removed, and the change is recorded.
func TestDeviceMovingIPIsTrackedByMAC(t *testing.T) {
	s := newTestStorage(t)

	// First seen at .10, with user data and a type.
	if err := s.MergeDevices([]types.Device{
		{IP: "192.168.1.10", MAC: "aa:bb:cc:dd:ee:ff", Hostname: "nas", Type: "Server"},
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.UpdateDeviceFields("192.168.1.10", strptr("File Server"), strptr("rack 1"), strptr("Infra"), nil, nil, nil); err != nil {
		t.Fatal(err)
	}

	// Same MAC reappears at .25.
	if err := s.MergeDevices([]types.Device{
		{IP: "192.168.1.25", MAC: "aa:bb:cc:dd:ee:ff", Hostname: "nas"},
	}); err != nil {
		t.Fatal(err)
	}

	devices := s.GetDevices()
	if _, stale := devices["192.168.1.10"]; stale {
		t.Error("the old IP entry should be removed after the device moved")
	}
	moved := devices["192.168.1.25"]
	if moved == nil {
		t.Fatal("the device should now be at the new IP")
	}
	if moved.Label != "File Server" || moved.Notes != "rack 1" || moved.Group != "Infra" {
		t.Errorf("user data did not move with the device: %+v", moved)
	}
	if moved.Type != "Server" {
		t.Errorf("type should carry over, got %q", moved.Type)
	}
	if len(moved.AddressHistory) != 1 || moved.AddressHistory[0].IP != "192.168.1.10" {
		t.Errorf("address change not recorded: %+v", moved.AddressHistory)
	}
	if moved.AddressHistory[0].ChangedAt.IsZero() {
		t.Error("the change should carry a timestamp")
	}
}

// TestDifferentMACsAreDistinctDevices confirms two devices with different MACs
// at different IPs are not confused for a move.
func TestDifferentMACsAreDistinctDevices(t *testing.T) {
	s := newTestStorage(t)
	_ = s.MergeDevices([]types.Device{{IP: "192.168.1.10", MAC: "aa:aa:aa:aa:aa:aa"}})
	_ = s.MergeDevices([]types.Device{{IP: "192.168.1.11", MAC: "bb:bb:bb:bb:bb:bb"}})

	devices := s.GetDevices()
	if len(devices) != 2 {
		t.Fatalf("expected two distinct devices, got %d", len(devices))
	}
	if len(devices["192.168.1.11"].AddressHistory) != 0 {
		t.Error("a distinct device should have no address history")
	}
}

func strptr(s string) *string { return &s }

// TestSupplementalMergeDoesNotClobber confirms a secondary source (mDNS/IPv6)
// fills gaps but never erases a MAC, vendor, or probe result the primary scan
// established.
func TestSupplementalMergeDoesNotClobber(t *testing.T) {
	s := newTestStorage(t)

	// Primary scan: full device with MAC, vendor, web flag and a risk.
	if err := s.MergeDevices([]types.Device{{
		IP: "192.168.1.5", MAC: "aa:bb:cc:dd:ee:ff", Vendor: "Acme",
		Type: "Server", WebUI: true, Risks: []string{"Telnet is open"},
	}}); err != nil {
		t.Fatal(err)
	}

	// mDNS supplements the same IP: a friendly name, but no MAC, no probe data.
	if err := s.MergeSupplemental([]types.Device{{
		IP: "192.168.1.5", Hostname: "fileserver",
	}}); err != nil {
		t.Fatal(err)
	}

	d := s.GetDevice("192.168.1.5")
	if d == nil {
		t.Fatal("device vanished")
	}
	if d.MAC != "aa:bb:cc:dd:ee:ff" {
		t.Errorf("MAC was clobbered: %q", d.MAC)
	}
	if d.Vendor != "Acme" {
		t.Errorf("vendor was clobbered: %q", d.Vendor)
	}
	if !d.WebUI || len(d.Risks) != 1 {
		t.Errorf("probe fields were clobbered: WebUI=%v Risks=%v", d.WebUI, d.Risks)
	}
	if d.Hostname != "fileserver" {
		t.Errorf("supplemental hostname not filled in: %q", d.Hostname)
	}
}
