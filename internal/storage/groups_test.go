package storage

import (
	"testing"

	"github.com/291-Group/LAN-Orangutan/internal/types"
)

// seedGrouped puts devices in storage and assigns each one a group.
func seedGrouped(t *testing.T, s *Storage, groups map[string]string) {
	t.Helper()
	var devices []types.Device
	for ip := range groups {
		devices = append(devices, types.Device{IP: ip, MAC: "aa:bb:cc:00:00:" + ip[len(ip)-2:]})
	}
	if err := s.MergeDevices(devices); err != nil {
		t.Fatalf("MergeDevices: %v", err)
	}
	for ip, g := range groups {
		group := g
		if err := s.UpdateDeviceFields(ip, nil, nil, &group, nil, nil, nil); err != nil {
			t.Fatalf("UpdateDeviceFields(%s): %v", ip, err)
		}
	}
}

func TestGroupsDefaultToBuiltInList(t *testing.T) {
	s := newTestStorage(t)

	got := s.GetGroups()
	want := DefaultGroups()
	if len(got) != len(want) {
		t.Fatalf("GetGroups() = %v, want the %d built-in defaults", got, len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("group %d = %q, want %q (declaration order matters)", i, got[i], want[i])
		}
	}
	if s.GroupsCustomised() {
		t.Error("GroupsCustomised() = true before any edit")
	}
}

func TestAddGroupAppearsAndPersists(t *testing.T) {
	s := newTestStorage(t)

	if err := s.AddGroup("Wearables"); err != nil {
		t.Fatalf("AddGroup: %v", err)
	}
	if !contains(s.GetGroups(), "Wearables") {
		t.Fatalf("GetGroups() = %v, want it to contain Wearables", s.GetGroups())
	}
	if !s.GroupsCustomised() {
		t.Error("GroupsCustomised() = false after adding a group")
	}
}

func TestAddGroupRejectsDuplicateRegardlessOfCase(t *testing.T) {
	s := newTestStorage(t)

	if err := s.AddGroup("iot"); err == nil {
		t.Fatal("AddGroup(\"iot\") should be rejected: IoT already exists")
	}
}

func TestAddGroupRejectsEmptyAndOverlongNames(t *testing.T) {
	s := newTestStorage(t)

	if err := s.AddGroup("   "); err == nil {
		t.Error("a blank group name should be rejected")
	}
	long := ""
	for i := 0; i < maxGroupNameLen+1; i++ {
		long += "x"
	}
	if err := s.AddGroup(long); err == nil {
		t.Errorf("a %d-character name should be rejected", len(long))
	}
}

func TestAddGroupRejectsControlCharacters(t *testing.T) {
	s := newTestStorage(t)

	if err := s.AddGroup("Bad\nName"); err == nil {
		t.Error("a name containing a control character should be rejected")
	}
}

// The point of the feature: renaming must follow through to the devices, or the
// list and the data drift apart.
func TestRenameGroupRewritesEveryDeviceUsingIt(t *testing.T) {
	s := newTestStorage(t)
	seedGrouped(t, s, map[string]string{
		"192.168.1.11": "IoT",
		"192.168.1.12": "IoT",
		"192.168.1.13": "Server",
	})

	if err := s.RenameGroup("IoT", "Wearables"); err != nil {
		t.Fatalf("RenameGroup: %v", err)
	}

	if contains(s.GetGroups(), "IoT") {
		t.Error("the old group name should be gone from the list")
	}
	if !contains(s.GetGroups(), "Wearables") {
		t.Error("the new group name should be in the list")
	}
	for _, ip := range []string{"192.168.1.11", "192.168.1.12"} {
		if got := s.GetDevice(ip).Group; got != "Wearables" {
			t.Errorf("device %s group = %q, want Wearables", ip, got)
		}
	}
	if got := s.GetDevice("192.168.1.13").Group; got != "Server" {
		t.Errorf("an unrelated device was changed: group = %q, want Server", got)
	}
}

func TestRenameGroupKeepsItsPositionInTheList(t *testing.T) {
	s := newTestStorage(t)

	before := s.GetGroups()
	idx := indexOf(before, "IoT")
	if idx < 0 {
		t.Fatal("expected IoT in the default list")
	}
	if err := s.RenameGroup("IoT", "Wearables"); err != nil {
		t.Fatalf("RenameGroup: %v", err)
	}
	if got := indexOf(s.GetGroups(), "Wearables"); got != idx {
		t.Errorf("renamed group moved to position %d, want %d", got, idx)
	}
}

// Renaming onto another existing group would silently merge them, losing a
// distinction the user never asked to give up.
func TestRenameGroupRejectsCollisionWithAnotherGroup(t *testing.T) {
	s := newTestStorage(t)

	if err := s.RenameGroup("IoT", "Server"); err == nil {
		t.Fatal("renaming IoT onto the existing Server group should be rejected")
	}
}

// Re-spelling the same group is not a collision with itself.
func TestRenameGroupAllowsCaseOnlyChange(t *testing.T) {
	s := newTestStorage(t)

	if err := s.RenameGroup("IoT", "IOT"); err != nil {
		t.Fatalf("re-spelling a group should be allowed: %v", err)
	}
	if !contains(s.GetGroups(), "IOT") {
		t.Errorf("GetGroups() = %v, want the new spelling", s.GetGroups())
	}
}

func TestRenameUnknownGroupIsAnError(t *testing.T) {
	s := newTestStorage(t)

	if err := s.RenameGroup("Nope", "Something"); err == nil {
		t.Fatal("renaming a group that does not exist should be an error")
	}
}

// Deleting a group must clear it from devices, never delete the devices.
func TestDeleteGroupClearsItFromDevicesButKeepsThem(t *testing.T) {
	s := newTestStorage(t)
	seedGrouped(t, s, map[string]string{
		"192.168.1.21": "IoT",
		"192.168.1.22": "Server",
	})

	if err := s.DeleteGroup("IoT"); err != nil {
		t.Fatalf("DeleteGroup: %v", err)
	}

	if contains(s.GetGroups(), "IoT") {
		t.Error("the deleted group should be gone from the list")
	}
	d := s.GetDevice("192.168.1.21")
	if d == nil {
		t.Fatal("deleting a group must never delete the device")
	}
	if d.Group != "" {
		t.Errorf("device group = %q, want it cleared", d.Group)
	}
	if got := s.GetDevice("192.168.1.22").Group; got != "Server" {
		t.Errorf("an unrelated device was changed: group = %q, want Server", got)
	}
}

func TestDeleteUnknownGroupIsAnError(t *testing.T) {
	s := newTestStorage(t)

	if err := s.DeleteGroup("Nope"); err == nil {
		t.Fatal("deleting a group that does not exist should be an error")
	}
}

func TestResetGroupsRestoresTheDefaults(t *testing.T) {
	s := newTestStorage(t)

	if err := s.AddGroup("Wearables"); err != nil {
		t.Fatalf("AddGroup: %v", err)
	}
	if err := s.SetGroups(nil); err != nil {
		t.Fatalf("SetGroups(nil): %v", err)
	}
	if s.GroupsCustomised() {
		t.Error("GroupsCustomised() = true after a reset")
	}
	if contains(s.GetGroups(), "Wearables") {
		t.Error("a reset should drop the custom group")
	}
	if len(s.GetGroups()) != len(DefaultGroups()) {
		t.Errorf("GetGroups() = %v, want the defaults back", s.GetGroups())
	}
}

// A group that arrives on a device without being in the list (a restored backup,
// a hand-edited file) must still be selectable, or editing that device would
// silently drop its group.
func TestGroupInUseButMissingFromTheListStillAppears(t *testing.T) {
	s := newTestStorage(t)
	seedGrouped(t, s, map[string]string{"192.168.1.31": "Imported"})

	if err := s.SetGroups([]string{"Server", "Desktop"}); err != nil {
		t.Fatalf("SetGroups: %v", err)
	}
	if !contains(s.GetGroups(), "Imported") {
		t.Errorf("GetGroups() = %v, want the in-use group to be included", s.GetGroups())
	}
}

func TestGroupCountsReportDevicesPerGroup(t *testing.T) {
	s := newTestStorage(t)
	seedGrouped(t, s, map[string]string{
		"192.168.1.41": "IoT",
		"192.168.1.42": "IoT",
		"192.168.1.43": "Server",
	})

	counts := s.GroupCounts()
	if counts["IoT"] != 2 {
		t.Errorf("counts[IoT] = %d, want 2", counts["IoT"])
	}
	if counts["Server"] != 1 {
		t.Errorf("counts[Server] = %d, want 1", counts["Server"])
	}
	if _, ok := counts["Desktop"]; ok {
		t.Error("an unused group should not appear in the counts")
	}
}

func TestSetGroupsRejectsDuplicates(t *testing.T) {
	s := newTestStorage(t)

	if err := s.SetGroups([]string{"Server", "server"}); err == nil {
		t.Fatal("a list containing the same group twice should be rejected")
	}
}

// Edits must survive a restart, which means they are in the state file.
func TestGroupEditsPersistAcrossReload(t *testing.T) {
	dir := t.TempDir()
	devicesFile := dir + "/devices.json"
	stateFile := dir + "/state.json"

	s, err := New(devicesFile, stateFile)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := s.RenameGroup("IoT", "Wearables"); err != nil {
		t.Fatalf("RenameGroup: %v", err)
	}

	reopened, err := New(devicesFile, stateFile)
	if err != nil {
		t.Fatalf("New (reopen): %v", err)
	}
	if !contains(reopened.GetGroups(), "Wearables") {
		t.Errorf("GetGroups() = %v after reload, want the rename to have survived", reopened.GetGroups())
	}
	if contains(reopened.GetGroups(), "IoT") {
		t.Error("the old name came back after a reload")
	}
}

func contains(haystack []string, needle string) bool {
	return indexOf(haystack, needle) >= 0
}

func indexOf(haystack []string, needle string) int {
	for i, s := range haystack {
		if s == needle {
			return i
		}
	}
	return -1
}
