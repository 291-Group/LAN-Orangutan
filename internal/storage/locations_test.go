package storage

import (
	"strings"
	"testing"

	"github.com/291-Group/LAN-Orangutan/internal/types"
)

func TestLocationsAddRenameDelete(t *testing.T) {
	s := newTestStorage(t)
	if got := s.GetLocations(); len(got) != 0 {
		t.Fatalf("a new install should have no locations, got %v", got)
	}
	if err := s.AddLocation("Office"); err != nil {
		t.Fatal(err)
	}
	if err := s.AddLocation("office"); err == nil {
		t.Error("a duplicate that differs only in case should be refused")
	}
	if err := s.MergeDevices([]types.Device{{IP: "192.168.1.40"}, {IP: "192.168.1.41"}}); err != nil {
		t.Fatal(err)
	}
	for _, ip := range []string{"192.168.1.40", "192.168.1.41"} {
		if err := s.UpdateDeviceFields(ip, nil, nil, nil, nil, nil, nil, nil, strptr("Office")); err != nil {
			t.Fatal(err)
		}
	}
	if n := s.LocationCounts()["Office"]; n != 2 {
		t.Fatalf("count = %d, want 2", n)
	}

	// Renaming moves every device with it.
	if err := s.RenameLocation("Office", "Study"); err != nil {
		t.Fatal(err)
	}
	if got := s.GetDevice("192.168.1.40").Location; got != "Study" {
		t.Errorf("device location after rename = %q, want Study", got)
	}

	// Deleting keeps the devices and clears the location.
	if err := s.DeleteLocation("Study"); err != nil {
		t.Fatal(err)
	}
	d := s.GetDevice("192.168.1.41")
	if d == nil || d.Location != "" {
		t.Fatalf("after delete: device %+v should exist with no location", d)
	}
	if got := s.GetLocations(); len(got) != 0 {
		t.Errorf("locations after delete = %v", got)
	}
}

// Setting a device to a place that is not on the list yet adds it, matched
// case-insensitively to an existing one so "office" lands in "Office".
func TestSettingANewLocationAddsIt(t *testing.T) {
	s := newTestStorage(t)
	if err := s.AddLocation("Office"); err != nil {
		t.Fatal(err)
	}
	if err := s.MergeDevices([]types.Device{{IP: "192.168.1.42"}, {IP: "192.168.1.43"}}); err != nil {
		t.Fatal(err)
	}
	if err := s.UpdateDeviceFields("192.168.1.42", nil, nil, nil, nil, nil, nil, nil, strptr("  Garage ")); err != nil {
		t.Fatal(err)
	}
	if err := s.UpdateDeviceFields("192.168.1.43", nil, nil, nil, nil, nil, nil, nil, strptr("office")); err != nil {
		t.Fatal(err)
	}
	if got := s.GetDevice("192.168.1.43").Location; got != "Office" {
		t.Errorf("location = %q, want the existing spelling Office", got)
	}
	if got := strings.Join(s.GetLocations(), ","); got != "Office,Garage" {
		t.Errorf("locations = %q, want Office,Garage", got)
	}
	long := strings.Repeat("x", maxLocationNameLen+1)
	if err := s.UpdateDeviceFields("192.168.1.42", nil, nil, nil, nil, nil, nil, nil, &long); err == nil {
		t.Error("an over-long location should be refused")
	}
	// Clearing it.
	if err := s.UpdateDeviceFields("192.168.1.42", nil, nil, nil, nil, nil, nil, nil, strptr("")); err != nil {
		t.Fatal(err)
	}
	if got := s.GetDevice("192.168.1.42").Location; got != "" {
		t.Errorf("location after clearing = %q", got)
	}
}

// A device's location survives scans and follows it to a new IP, and having
// one protects the device from automatic cleanup like any other curated field.
func TestLocationSurvivesScansAndMoves(t *testing.T) {
	s := newTestStorage(t)
	mac := "aa:bb:cc:00:00:44"
	if err := s.MergeDevices([]types.Device{{IP: "192.168.1.44", MAC: mac}}); err != nil {
		t.Fatal(err)
	}
	if err := s.UpdateDeviceFields("192.168.1.44", nil, nil, nil, nil, nil, nil, nil, strptr("Kitchen")); err != nil {
		t.Fatal(err)
	}
	if err := s.MergeDevices([]types.Device{{IP: "192.168.1.44", MAC: mac, Hostname: "fridge"}}); err != nil {
		t.Fatal(err)
	}
	if err := s.MergeDevices([]types.Device{{IP: "192.168.1.99", MAC: mac}}); err != nil {
		t.Fatal(err)
	}
	d := s.GetDevice("192.168.1.99")
	if d == nil || d.Location != "Kitchen" {
		t.Fatalf("moved device = %+v, want Location Kitchen", d)
	}
	if !isCurated(d) {
		t.Error("a device with a location should count as curated")
	}
}

// Scanning an IPv6 network must apply the same guards as the IPv6 pass: a
// privacy address with a randomised MAC and no known device is not stored.
func TestMergeScanIPv6UsesNeighbourGuards(t *testing.T) {
	s := newTestStorage(t)
	privacy := types.Device{IP: "2606:6d00:f3e:cb00:1c2d:3e4f:5a6b:7c8d", MAC: "8a:11:22:33:44:55"} // locally administered
	if err := s.MergeScan("2606:6d00:f3e:cb00::/64", []types.Device{privacy}); err != nil {
		t.Fatal(err)
	}
	if s.GetDevice(privacy.IP) != nil {
		t.Error("a privacy address from an IPv6 scan should not be stored as a device")
	}
	// An IPv4 scan still goes through the normal merge.
	if err := s.MergeScan("192.168.1.0/24", []types.Device{{IP: "192.168.1.50"}}); err != nil {
		t.Fatal(err)
	}
	if s.GetDevice("192.168.1.50") == nil {
		t.Error("an IPv4 scan result should be stored")
	}
}
