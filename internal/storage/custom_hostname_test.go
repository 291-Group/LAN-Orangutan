package storage

import (
	"strings"
	"testing"
	"time"

	"github.com/291-Group/LAN-Orangutan/internal/types"
)

func TestCustomHostname_SetAndClear(t *testing.T) {
	s := newTestStorage(t)
	if err := s.MergeDevices([]types.Device{
		{IP: "192.168.1.20", MAC: "aa:bb:cc:00:00:20", Hostname: "android-7f3a"},
	}); err != nil {
		t.Fatal(err)
	}

	if err := s.UpdateDeviceFields("192.168.1.20", nil, nil, nil, nil, nil, nil, strptr("  Living Room TV  "), nil); err != nil {
		t.Fatalf("UpdateDeviceFields: %v", err)
	}
	dev := s.GetDevice("192.168.1.20")
	if dev.CustomHostname != "Living Room TV" {
		t.Fatalf("custom hostname = %q, want trimmed %q", dev.CustomHostname, "Living Room TV")
	}
	if dev.Hostname != "android-7f3a" {
		t.Fatalf("scanned hostname changed to %q", dev.Hostname)
	}
	if got := dev.DisplayHostname(); got != "Living Room TV" {
		t.Fatalf("DisplayHostname = %q", got)
	}
	if !isCurated(dev) {
		t.Fatal("a device with a custom hostname should count as curated")
	}

	// Clearing it falls back to the scanned name.
	if err := s.UpdateDeviceFields("192.168.1.20", nil, nil, nil, nil, nil, nil, strptr(""), nil); err != nil {
		t.Fatalf("UpdateDeviceFields clear: %v", err)
	}
	dev = s.GetDevice("192.168.1.20")
	if dev.CustomHostname != "" || dev.DisplayHostname() != "android-7f3a" {
		t.Fatalf("after clear: custom=%q display=%q", dev.CustomHostname, dev.DisplayHostname())
	}
}

func TestCustomHostname_LeftAloneWhenNil(t *testing.T) {
	s := newTestStorage(t)
	if err := s.MergeDevices([]types.Device{{IP: "192.168.1.21", Hostname: "nas"}}); err != nil {
		t.Fatal(err)
	}
	if err := s.UpdateDeviceFields("192.168.1.21", nil, nil, nil, nil, nil, nil, strptr("Storage"), nil); err != nil {
		t.Fatal(err)
	}
	// Editing another field must not touch the custom hostname.
	if err := s.UpdateDeviceFields("192.168.1.21", strptr("Basement"), nil, nil, nil, nil, nil, nil, nil); err != nil {
		t.Fatal(err)
	}
	if got := s.GetDevice("192.168.1.21").CustomHostname; got != "Storage" {
		t.Fatalf("custom hostname = %q, want Storage", got)
	}
}

func TestCustomHostname_Rejected(t *testing.T) {
	s := newTestStorage(t)
	if err := s.MergeDevices([]types.Device{{IP: "192.168.1.22"}}); err != nil {
		t.Fatal(err)
	}
	for name, h := range map[string]string{
		"control character": "bad\x00name",
		"newline":           "two\nlines",
		"too long":          strings.Repeat("a", MaxCustomHostnameLen+1),
	} {
		if err := s.UpdateDeviceFields("192.168.1.22", nil, nil, nil, nil, nil, nil, &h, nil); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
	if got := s.GetDevice("192.168.1.22").CustomHostname; got != "" {
		t.Fatalf("a rejected value was stored: %q", got)
	}
	ok := strings.Repeat("a", MaxCustomHostnameLen)
	if err := s.UpdateDeviceFields("192.168.1.22", nil, nil, nil, nil, nil, nil, &ok, nil); err != nil {
		t.Fatalf("a %d-character name should be accepted: %v", MaxCustomHostnameLen, err)
	}
}

// Every scan path must leave the user's name alone, including the main scan,
// which does replace the scanned hostname when it finds a new one.
func TestCustomHostname_SurvivesScans(t *testing.T) {
	s := newTestStorage(t)
	ip := "192.168.1.23"
	if err := s.MergeDevices([]types.Device{{IP: ip, MAC: "aa:bb:cc:00:00:23", Hostname: "old-name"}}); err != nil {
		t.Fatal(err)
	}
	if err := s.UpdateDeviceFields(ip, nil, nil, nil, nil, nil, nil, strptr("Printer Upstairs"), nil); err != nil {
		t.Fatal(err)
	}

	if err := s.MergeDevices([]types.Device{{IP: ip, MAC: "aa:bb:cc:00:00:23", Hostname: "new-name"}}); err != nil {
		t.Fatal(err)
	}
	if err := s.MergeSupplemental([]types.Device{{IP: ip, MAC: "aa:bb:cc:00:00:23", Hostname: "mdns-name"}}); err != nil {
		t.Fatal(err)
	}
	if err := s.UpdateDevice(&types.Device{IP: ip, MAC: "aa:bb:cc:00:00:23", Hostname: "updated"}); err != nil {
		t.Fatal(err)
	}

	dev := s.GetDevice(ip)
	if dev.CustomHostname != "Printer Upstairs" {
		t.Fatalf("custom hostname = %q after scans", dev.CustomHostname)
	}
	if dev.Hostname == "old-name" {
		t.Fatal("the scanned hostname should still update underneath the custom one")
	}
}

// A device that moves to a new IP (matched by MAC) keeps its custom name.
func TestCustomHostname_FollowsMovedDevice(t *testing.T) {
	s := newTestStorage(t)
	mac := "aa:bb:cc:00:00:24"
	if err := s.MergeDevices([]types.Device{{IP: "192.168.1.24", MAC: mac, Hostname: "tv"}}); err != nil {
		t.Fatal(err)
	}
	if err := s.UpdateDeviceFields("192.168.1.24", nil, nil, nil, nil, nil, nil, strptr("Living Room TV"), nil); err != nil {
		t.Fatal(err)
	}
	if err := s.MergeDevices([]types.Device{{IP: "192.168.1.99", MAC: mac, Hostname: "tv"}}); err != nil {
		t.Fatal(err)
	}
	dev := s.GetDevice("192.168.1.99")
	if dev == nil {
		t.Fatal("moved device not found at its new IP")
	}
	if dev.CustomHostname != "Living Room TV" {
		t.Fatalf("custom hostname = %q after the move", dev.CustomHostname)
	}
}

// The custom name must survive a restart, which means it is persisted.
func TestCustomHostname_Persists(t *testing.T) {
	s := newTestStorage(t)
	if err := s.MergeDevices([]types.Device{{IP: "192.168.1.25", Hostname: "pi"}}); err != nil {
		t.Fatal(err)
	}
	if err := s.UpdateDeviceFields("192.168.1.25", nil, nil, nil, nil, nil, nil, strptr("Pi-hole"), nil); err != nil {
		t.Fatal(err)
	}
	reopened, err := New(s.devicesFile, s.stateFile)
	if err != nil {
		t.Fatal(err)
	}
	if got := reopened.GetDevice("192.168.1.25").CustomHostname; got != "Pi-hole" {
		t.Fatalf("after reopen: custom hostname = %q", got)
	}
}

// The server and a CLI command can share a data directory. An edit made by one
// store must be seen by the other and must not be lost when the other saves.
func TestSharedDataDir_EditIsNotOverwritten(t *testing.T) {
	server := newTestStorage(t)
	if err := server.MergeDevices([]types.Device{{IP: "192.168.1.30", MAC: "aa:bb:cc:00:00:30", Hostname: "cam"}}); err != nil {
		t.Fatal(err)
	}

	cli, err := New(server.devicesFile, server.stateFile)
	if err != nil {
		t.Fatal(err)
	}
	// Make sure the second write lands on a different modification time even on
	// a filesystem with coarse timestamps.
	time.Sleep(10 * time.Millisecond)
	if err := cli.UpdateDeviceFields("192.168.1.30", strptr("Front Door"), nil, nil, nil, nil, nil, strptr("Doorbell"), nil); err != nil {
		t.Fatal(err)
	}

	if got := server.GetDevice("192.168.1.30"); got.CustomHostname != "Doorbell" || got.Label != "Front Door" {
		t.Fatalf("server did not see the CLI edit: hostname=%q label=%q", got.CustomHostname, got.Label)
	}

	// The server's next scan must keep the edit, not write its stale copy back.
	time.Sleep(10 * time.Millisecond)
	if err := server.MergeDevices([]types.Device{{IP: "192.168.1.31", MAC: "aa:bb:cc:00:00:31"}}); err != nil {
		t.Fatal(err)
	}
	reopened, err := New(server.devicesFile, server.stateFile)
	if err != nil {
		t.Fatal(err)
	}
	if got := reopened.GetDevice("192.168.1.30"); got == nil || got.CustomHostname != "Doorbell" {
		t.Fatalf("CLI edit lost after the server saved: %+v", got)
	}
	if reopened.GetDevice("192.168.1.31") == nil {
		t.Fatal("server's own change was lost")
	}
}

// Groups live in the state file. A group added by one process must show up in
// the other, and survive the other's next save.
func TestSharedDataDir_GroupsFollowOtherProcess(t *testing.T) {
	server := newTestStorage(t)
	cli, err := New(server.devicesFile, server.stateFile)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(10 * time.Millisecond)
	if err := cli.AddGroup("Garage"); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, g := range server.GetGroups() {
		if g == "Garage" {
			found = true
		}
	}
	if !found {
		t.Fatalf("server did not see the group added by the CLI: %v", server.GetGroups())
	}
	time.Sleep(10 * time.Millisecond)
	if err := server.SetScanInterval(600); err != nil {
		t.Fatal(err)
	}
	reopened, err := New(server.devicesFile, server.stateFile)
	if err != nil {
		t.Fatal(err)
	}
	for _, g := range reopened.GetGroups() {
		if g == "Garage" {
			return
		}
	}
	t.Fatal("group added by the CLI was lost when the server saved its state")
}
