package storage

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/291-Group/LAN-Orangutan/internal/types"
	"github.com/291-Group/LAN-Orangutan/internal/uerr"
)

// One bad field must leave the whole request unapplied: a new location is not
// added to the list and no other field changes when, say, the hostname fails.
func TestUpdateDeviceFieldsIsAllOrNothing(t *testing.T) {
	s := newTestStorage(t)
	if err := s.MergeDevices([]types.Device{{IP: "192.168.1.70", Hostname: "pi"}}); err != nil {
		t.Fatal(err)
	}
	bad := "bad\x00name"
	err := s.UpdateDeviceFields("192.168.1.70", strptr("Shelf"), nil, nil, nil, nil, nil, &bad, strptr("Attic"))
	if e, ok := uerr.Of(err); !ok || e.Code != "hostname.controlChars" {
		t.Fatalf("expected hostname.controlChars, got %v", err)
	}
	for _, l := range s.GetLocations() {
		if l == "Attic" {
			t.Error("a location from a rejected request must not be added to the list")
		}
	}
	d := s.GetDevice("192.168.1.70")
	if d.Label != "" || d.Location != "" {
		t.Errorf("fields from a rejected request were applied: %+v", d)
	}
}

func TestUpdateDeviceFieldsNotFound(t *testing.T) {
	s := newTestStorage(t)
	err := s.UpdateDeviceFields("10.9.9.9", strptr("x"), nil, nil, nil, nil, nil, nil, nil)
	if e, ok := uerr.Of(err); !ok || e.Code != "device.notFound" || e.Args["ip"] != "10.9.9.9" {
		t.Errorf("expected device.notFound with the ip, got %v", err)
	}
	if err == nil || err.Error() != "no device at 10.9.9.9" {
		t.Errorf("text = %v", err)
	}
	err = s.DeleteDevice("10.9.9.9")
	if e, ok := uerr.Of(err); !ok || e.Code != "device.notFound" {
		t.Errorf("DeleteDevice should use the same code, got %v", err)
	}
}

// A group typed with stray spaces or in the wrong case must not become a
// second, phantom group next to the listed one.
func TestUpdateDeviceFieldsNormalisesGroup(t *testing.T) {
	s := newTestStorage(t)
	if err := s.MergeDevices([]types.Device{{IP: "192.168.1.71"}, {IP: "192.168.1.72"}}); err != nil {
		t.Fatal(err)
	}
	if err := s.UpdateDeviceFields("192.168.1.71", nil, nil, strptr("  Office  "), nil, nil, nil, nil, nil); err != nil {
		t.Fatal(err)
	}
	if got := s.GetDevice("192.168.1.71").Group; got != "Office" {
		t.Errorf("group not trimmed: %q", got)
	}
	if err := s.UpdateDeviceFields("192.168.1.72", nil, nil, strptr("office"), nil, nil, nil, nil, nil); err != nil {
		t.Fatal(err)
	}
	if got := s.GetDevice("192.168.1.72").Group; got != "Office" {
		t.Errorf("group should take the spelling already in use: %q", got)
	}
	if n := len(s.GroupCounts()); n != 1 {
		t.Errorf("expected one group in use, got %d: %v", n, s.GroupCounts())
	}
	// Clearing still works, and the rules for a listed group apply.
	if err := s.UpdateDeviceFields("192.168.1.72", nil, nil, strptr("   "), nil, nil, nil, nil, nil); err != nil {
		t.Fatal(err)
	}
	if got := s.GetDevice("192.168.1.72").Group; got != "" {
		t.Errorf("blank group should clear, got %q", got)
	}
	long := strings.Repeat("g", maxGroupNameLen+1)
	err := s.UpdateDeviceFields("192.168.1.72", nil, nil, &long, nil, nil, nil, nil, nil)
	if e, ok := uerr.Of(err); !ok || e.Code != "group.tooLong" {
		t.Errorf("expected group.tooLong, got %v", err)
	}
}

func TestUpdateDeviceFieldsLabelAndNotesLimits(t *testing.T) {
	s := newTestStorage(t)
	if err := s.MergeDevices([]types.Device{{IP: "192.168.1.73"}}); err != nil {
		t.Fatal(err)
	}
	long := strings.Repeat("l", MaxLabelLen+1)
	err := s.UpdateDeviceFields("192.168.1.73", &long, nil, nil, nil, nil, nil, nil, nil)
	if e, ok := uerr.Of(err); !ok || e.Code != "label.tooLong" || e.Args["max"] != MaxLabelLen {
		t.Errorf("expected label.tooLong, got %v", err)
	}
	ctl := "tab\there"
	err = s.UpdateDeviceFields("192.168.1.73", &ctl, nil, nil, nil, nil, nil, nil, nil)
	if e, ok := uerr.Of(err); !ok || e.Code != "label.controlChars" {
		t.Errorf("expected label.controlChars, got %v", err)
	}
	notes := strings.Repeat("n", MaxNotesLen+1)
	err = s.UpdateDeviceFields("192.168.1.73", nil, &notes, nil, nil, nil, nil, nil, nil)
	if e, ok := uerr.Of(err); !ok || e.Code != "notes.tooLong" || e.Args["max"] != MaxNotesLen {
		t.Errorf("expected notes.tooLong, got %v", err)
	}
	// Notes keep their line breaks; a label is trimmed.
	ok := "line one\nline two"
	if err := s.UpdateDeviceFields("192.168.1.73", strptr("  NAS  "), &ok, nil, nil, nil, nil, nil, nil); err != nil {
		t.Fatal(err)
	}
	d := s.GetDevice("192.168.1.73")
	if d.Label != "NAS" || d.Notes != ok {
		t.Errorf("label/notes stored as %q / %q", d.Label, d.Notes)
	}
}

// Readers share the lock and only reload when a file stamp changed. A change
// written by another Storage on the same files must still be seen by a plain
// getter, through that fast path.
func TestReadersSeeOtherProcessWrites(t *testing.T) {
	dir := t.TempDir()
	devices, state := filepath.Join(dir, "devices.json"), filepath.Join(dir, "state.json")
	server, err := New(devices, state)
	if err != nil {
		t.Fatal(err)
	}
	if err := server.MergeDevices([]types.Device{{IP: "192.168.1.80", Hostname: "before"}}); err != nil {
		t.Fatal(err)
	}
	if server.GetDevice("192.168.1.80").Hostname != "before" {
		t.Fatal("setup: device not stored")
	}

	cli, err := New(devices, state)
	if err != nil {
		t.Fatal(err)
	}
	if err := cli.UpdateDeviceFields("192.168.1.80", strptr("Printer"), nil, nil, nil, nil, nil, nil, strptr("Hall")); err != nil {
		t.Fatal(err)
	}
	if err := cli.AddGroup("Lab"); err != nil {
		t.Fatal(err)
	}

	// Every one of these is a read-only getter on the first instance.
	if got := server.GetDevice("192.168.1.80"); got == nil || got.Label != "Printer" {
		t.Errorf("GetDevice did not pick up the other process's device write: %+v", got)
	}
	if got := server.GetDevices()["192.168.1.80"]; got == nil || got.Location != "Hall" {
		t.Errorf("GetDevices did not pick up the other process's device write: %+v", got)
	}
	found := false
	for _, g := range server.GetGroups() {
		if g == "Lab" {
			found = true
		}
	}
	if !found {
		t.Errorf("GetGroups did not pick up the other process's state write: %v", server.GetGroups())
	}
	if locs := server.GetLocations(); len(locs) != 1 || locs[0] != "Hall" {
		t.Errorf("GetLocations did not pick up the new location: %v", locs)
	}
}
