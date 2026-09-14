package storage

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/291-Group/LAN-Orangutan/internal/types"
)

// newTestStorage returns storage backed by a throwaway directory.
func newTestStorage(t *testing.T) *Storage {
	t.Helper()

	dir := t.TempDir()
	s, err := New(filepath.Join(dir, "devices.json"), filepath.Join(dir, "state.json"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return s
}

func TestMostRecentScanIsZeroBeforeAnyScan(t *testing.T) {
	s := newTestStorage(t)

	if got := s.GetMostRecentScan(); !got.IsZero() {
		t.Errorf("GetMostRecentScan() = %v, want the zero time before any scan", got)
	}
}

func TestMostRecentScanWithOneNetwork(t *testing.T) {
	s := newTestStorage(t)

	when := time.Now().Add(-30 * time.Minute).Truncate(time.Second)
	if err := s.SetLastScan("192.168.1.0/24", when); err != nil {
		t.Fatalf("SetLastScan: %v", err)
	}

	if got := s.GetMostRecentScan(); !got.Equal(when) {
		t.Errorf("GetMostRecentScan() = %v, want %v", got, when)
	}
}

func TestMostRecentScanPicksTheLatest(t *testing.T) {
	s := newTestStorage(t)

	older := time.Now().Add(-2 * time.Hour).Truncate(time.Second)
	newer := time.Now().Add(-5 * time.Minute).Truncate(time.Second)
	middle := time.Now().Add(-1 * time.Hour).Truncate(time.Second)

	// Set them out of order, so the result cannot come from insertion order.
	if err := s.SetLastScan("10.0.0.0/24", middle); err != nil {
		t.Fatalf("SetLastScan: %v", err)
	}
	if err := s.SetLastScan("192.168.1.0/24", newer); err != nil {
		t.Fatalf("SetLastScan: %v", err)
	}
	if err := s.SetLastScan("172.16.0.0/24", older); err != nil {
		t.Fatalf("SetLastScan: %v", err)
	}

	if got := s.GetMostRecentScan(); !got.Equal(newer) {
		t.Errorf("GetMostRecentScan() = %v, want the most recent %v", got, newer)
	}
}

func TestMostRecentScanSurvivesReload(t *testing.T) {
	dir := t.TempDir()
	devices := filepath.Join(dir, "devices.json")
	state := filepath.Join(dir, "state.json")

	first, err := New(devices, state)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	when := time.Now().Add(-15 * time.Minute).Truncate(time.Second)
	if err := first.SetLastScan("192.168.1.0/24", when); err != nil {
		t.Fatalf("SetLastScan: %v", err)
	}

	// A restart must not lose the timestamp, otherwise the dashboard would
	// claim nothing had ever been scanned.
	second, err := New(devices, state)
	if err != nil {
		t.Fatalf("reopening storage: %v", err)
	}

	if got := second.GetMostRecentScan(); !got.Equal(when) {
		t.Errorf("after reload GetMostRecentScan() = %v, want %v", got, when)
	}
}

func TestContinuousScanDefaultsToConfigWhenUnset(t *testing.T) {
	dir := t.TempDir()
	s, err := New(filepath.Join(dir, "devices.json"), filepath.Join(dir, "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !s.ContinuousScanEnabled(true) {
		t.Error("with no override saved, want the config default (true) to win")
	}
	if s.ContinuousScanEnabled(false) {
		t.Error("with no override saved, want the config default (false) to win")
	}
}

func TestContinuousScanOverrideSurvivesReload(t *testing.T) {
	dir := t.TempDir()
	devices := filepath.Join(dir, "devices.json")
	state := filepath.Join(dir, "state.json")

	first, err := New(devices, state)
	if err != nil {
		t.Fatal(err)
	}
	// User turns continuous scanning off even though config defaults it on.
	if err := first.SetContinuousScan(false); err != nil {
		t.Fatal(err)
	}

	second, err := New(devices, state)
	if err != nil {
		t.Fatal(err)
	}
	if second.ContinuousScanEnabled(true) {
		t.Error("saved override (false) should win over the config default (true) after reload")
	}
}

func TestMergeIPv6NeighborsEnrichesByMAC(t *testing.T) {
	s := newTestStorage(t)
	// A device found by the reliable IPv4 scan.
	if err := s.MergeDevices([]types.Device{{IP: "192.168.1.5", MAC: "00:1A:2B:3C:4D:5E"}}); err != nil {
		t.Fatal(err)
	}
	// The same physical device sighted over IPv6 (same MAC, routable address).
	if err := s.MergeIPv6Neighbors([]types.Device{{IP: "2001:db8::5", MAC: "00:1A:2B:3C:4D:5E", Hostname: "printer"}}); err != nil {
		t.Fatal(err)
	}
	devs := s.GetDevices()
	if len(devs) != 1 {
		t.Fatalf("an IPv6 sighting of a known device should enrich it, not add a second entry: got %d", len(devs))
	}
	d, ok := devs["192.168.1.5"]
	if !ok || d.Hostname != "printer" {
		t.Errorf("expected the IPv4 device enriched with the hostname, got %+v", devs)
	}
	if d.IPv6 != "2001:db8::5" {
		t.Errorf("expected the device's routable IPv6 address recorded, got %q", d.IPv6)
	}
}

func TestMergeIPv6NeighborsCreatesOnlyForRealMAC(t *testing.T) {
	s := newTestStorage(t)
	err := s.MergeIPv6Neighbors([]types.Device{
		{IP: "fe80::1", MAC: "00:1A:2B:3C:4D:5E"},     // link-local: never a device
		{IP: "2001:db8::a", MAC: "AA:BB:CC:DD:EE:FF"}, // randomised MAC, no match: privacy ghost
		{IP: "2001:db8::b", MAC: "00:1A:2B:3C:4D:99"}, // real MAC, no match: genuine IPv6-only device
	})
	if err != nil {
		t.Fatal(err)
	}
	devs := s.GetDevices()
	if len(devs) != 1 {
		t.Fatalf("only the real-MAC IPv6-only device should be created: got %d: %+v", len(devs), devs)
	}
	if _, ok := devs["2001:db8::b"]; !ok {
		t.Errorf("expected the real-MAC device created, got %+v", devs)
	}
}

func TestPruneEphemeralIPv6OnLoad(t *testing.T) {
	dir := t.TempDir()
	devicesFile := filepath.Join(dir, "devices.json")
	stateFile := filepath.Join(dir, "state.json")

	// An install that accumulated IPv6 phantoms.
	seed := map[string]types.Device{
		"192.168.1.5": {IP: "192.168.1.5", MAC: "00:1A:2B:3C:4D:5E"}, // real LAN device: keep
		"fe80::1":     {IP: "fe80::1", MAC: "00:1A:2B:3C:4D:5E"},     // link-local phantom: drop
		"2001:db8::a": {IP: "2001:db8::a", MAC: "AA:BB:CC:DD:EE:FF"}, // randomised IPv6 phantom: drop
		"2001:db8::b": {IP: "2001:db8::b", MAC: "00:1A:2B:3C:4D:99"}, // real IPv6-only device: keep
	}
	data, err := json.MarshalIndent(seed, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(devicesFile, data, 0600); err != nil {
		t.Fatal(err)
	}

	s, err := New(devicesFile, stateFile)
	if err != nil {
		t.Fatal(err)
	}
	devs := s.GetDevices()
	if len(devs) != 2 {
		t.Fatalf("expected 2 real devices after pruning phantoms, got %d: %+v", len(devs), devs)
	}
	if _, ok := devs["192.168.1.5"]; !ok {
		t.Error("real LAN device was pruned")
	}
	if _, ok := devs["2001:db8::b"]; !ok {
		t.Error("real IPv6-only device was pruned")
	}
	if _, ok := devs["fe80::1"]; ok {
		t.Error("link-local phantom survived the prune")
	}
	if _, ok := devs["2001:db8::a"]; ok {
		t.Error("randomised IPv6 phantom survived the prune")
	}
}

func TestPruneKeepsCuratedIPv6Device(t *testing.T) {
	dir := t.TempDir()
	devicesFile := filepath.Join(dir, "devices.json")
	stateFile := filepath.Join(dir, "state.json")

	// A device keyed by a link-local address that the user has labelled: it must
	// survive the prune with its label intact, not be wiped on every restart.
	seed := map[string]types.Device{
		"fe80::abcd": {IP: "fe80::abcd", MAC: "AA:BB:CC:DD:EE:FF", Label: "Front door cam", Group: "IoT"},
		"fe80::dead": {IP: "fe80::dead", MAC: "02:00:00:00:00:01"}, // unlabelled phantom: drop
	}
	data, err := json.MarshalIndent(seed, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(devicesFile, data, 0600); err != nil {
		t.Fatal(err)
	}

	s, err := New(devicesFile, stateFile)
	if err != nil {
		t.Fatal(err)
	}
	devs := s.GetDevices()
	if d, ok := devs["fe80::abcd"]; !ok || d.Label != "Front door cam" {
		t.Errorf("a curated device must survive the prune with its label; got %+v", devs["fe80::abcd"])
	}
	if _, ok := devs["fe80::dead"]; ok {
		t.Error("unlabelled link-local phantom should have been pruned")
	}
}

// TestIPv6RealisticDualStackNetwork simulates a real IPv6-enabled home network
// (which the IPv4-only test LAN cannot exercise) and asserts LAN Orangutan
// discovers the legitimate IPv6 devices while dropping only the privacy noise.
func TestIPv6RealisticDualStackNetwork(t *testing.T) {
	s := newTestStorage(t)

	// The IPv4/ARP scan finds the routable dual-stack devices first.
	if err := s.MergeDevices([]types.Device{
		{IP: "192.168.1.1", MAC: "3C:37:86:11:22:33", Hostname: "router"},
		{IP: "192.168.1.20", MAC: "A4:83:E7:44:55:66", Hostname: "macbook"},
	}); err != nil {
		t.Fatal(err)
	}

	// Then the IPv6 neighbour cache is read on a dual-stack network:
	err := s.MergeIPv6Neighbors([]types.Device{
		// The macbook again, over its global IPv6 (same NIC MAC): must ENRICH,
		// not create a second entry.
		{IP: "2001:db8:1::20", MAC: "A4:83:E7:44:55:66"},
		// A genuinely IPv6-only device (e.g. a NAS) with a real hardware MAC and
		// no IPv4: must be CREATED and shown.
		{IP: "2001:db8:1::50", MAC: "B8:27:EB:77:88:99", Hostname: "nas"},
		// A ULA IPv6-only device with a real MAC: must be CREATED.
		{IP: "fd00:abcd::7", MAC: "DC:A6:32:AA:BB:CC", Hostname: "camera"},
		// A privacy phone: rotating global IPv6 + randomised MAC, not seen over
		// IPv4: must be SKIPPED (this is the phantom source).
		{IP: "2001:db8:1::f1ee", MAC: "AA:BB:CC:DD:EE:F1"},
		// Link-local: never a device.
		{IP: "fe80::1", MAC: "3C:37:86:11:22:33"},
	})
	if err != nil {
		t.Fatal(err)
	}

	devs := s.GetDevices()
	// Expect exactly 4 real devices: router + macbook (dual-stack, one entry) +
	// nas + camera (IPv6-only). No phantom, no link-local, no duplicate.
	if len(devs) != 4 {
		t.Fatalf("expected 4 real devices, got %d: %+v", len(devs), devs)
	}
	want := map[string]bool{"192.168.1.1": true, "192.168.1.20": true, "2001:db8:1::50": true, "fd00:abcd::7": true}
	for ip := range want {
		if _, ok := devs[ip]; !ok {
			t.Errorf("missing expected device %s", ip)
		}
	}
	if _, ok := devs["2001:db8:1::20"]; ok {
		t.Error("dual-stack macbook was duplicated by its IPv6 address instead of enriched")
	}
	if _, ok := devs["2001:db8:1::f1ee"]; ok {
		t.Error("privacy phantom (randomised MAC) was created")
	}
	if _, ok := devs["fe80::1"]; ok {
		t.Error("link-local was listed as a device")
	}

	t.Logf("Resulting inventory on a dual-stack network:")
	for ip, d := range devs {
		t.Logf("  %-18s %s", ip, d.Hostname)
	}
}

// --- Storage limits: retention + max_devices enforcement (v3.4.2) ---

// seedDevice inserts a device straight into the map with a chosen LastSeen, so a
// test can create devices that are already "old" without waiting. Same-package
// (white-box) access. MAC is set to the IP just to keep it unique.
func seedDevice(s *Storage, ip string, lastSeen time.Time, mut func(*types.Device)) {
	d := &types.Device{IP: ip, MAC: ip, LastSeen: lastSeen, FirstSeen: lastSeen}
	if mut != nil {
		mut(d)
	}
	s.mu.Lock()
	s.devices[ip] = d
	s.mu.Unlock()
}

func TestRetentionPrunesOldOfflineDevices(t *testing.T) {
	s := newTestStorage(t)
	now := time.Now()
	seedDevice(s, "10.0.0.1", now, nil)                        // online: keep
	seedDevice(s, "10.0.0.2", now.Add(-48*time.Hour), nil)     // offline 2d, within 90d: keep
	seedDevice(s, "10.0.0.3", now.Add(-100*24*time.Hour), nil) // offline 100d: prune

	if err := s.SetLimits(90, 0); err != nil {
		t.Fatalf("SetLimits: %v", err)
	}
	devs := s.GetDevices()
	if _, ok := devs["10.0.0.3"]; ok {
		t.Error("a device offline past retention_days should be pruned")
	}
	if len(devs) != 2 {
		t.Errorf("want 2 devices kept, got %d", len(devs))
	}
}

func TestRetentionKeepsCuratedDevice(t *testing.T) {
	s := newTestStorage(t)
	now := time.Now()
	seedDevice(s, "10.0.0.9", now.Add(-365*24*time.Hour), func(d *types.Device) {
		d.Label = "NAS"
	})
	if err := s.SetLimits(90, 0); err != nil {
		t.Fatalf("SetLimits: %v", err)
	}
	if _, ok := s.GetDevices()["10.0.0.9"]; !ok {
		t.Error("a curated device must never be pruned by retention, even a year offline")
	}
}

func TestMaxDevicesEvictsOldestOffline(t *testing.T) {
	s := newTestStorage(t)
	now := time.Now()
	// Five offline devices (all > 1h old, so evictable), staggered by age.
	for i := 1; i <= 5; i++ {
		seedDevice(s, fmt.Sprintf("10.0.1.%d", i), now.Add(-time.Duration(i)*24*time.Hour), nil)
	}
	if err := s.SetLimits(0, 3); err != nil { // cap 3, retention off
		t.Fatalf("SetLimits: %v", err)
	}
	devs := s.GetDevices()
	if len(devs) != 3 {
		t.Fatalf("want cap of 3, got %d", len(devs))
	}
	for _, gone := range []string{"10.0.1.4", "10.0.1.5"} { // the two oldest
		if _, ok := devs[gone]; ok {
			t.Errorf("%s (oldest) should have been evicted", gone)
		}
	}
	for _, kept := range []string{"10.0.1.1", "10.0.1.2", "10.0.1.3"} {
		if _, ok := devs[kept]; !ok {
			t.Errorf("%s (newer) should have been kept", kept)
		}
	}
}

func TestMaxDevicesNeverEvictsCuratedOrOnline(t *testing.T) {
	s := newTestStorage(t)
	now := time.Now()
	seedDevice(s, "10.0.2.1", now, nil)                                                              // online
	seedDevice(s, "10.0.2.2", now.Add(-500*time.Hour), func(d *types.Device) { d.Group = "Server" }) // curated, old
	seedDevice(s, "10.0.2.3", now.Add(-500*time.Hour), nil)                                          // plain old: evictable
	seedDevice(s, "10.0.2.4", now.Add(-400*time.Hour), nil)                                          // plain old: evictable

	if err := s.SetLimits(0, 1); err != nil { // cap 1, but only 2 are evictable
		t.Fatalf("SetLimits: %v", err)
	}
	devs := s.GetDevices()
	if _, ok := devs["10.0.2.1"]; !ok {
		t.Error("online device must never be evicted by the cap")
	}
	if _, ok := devs["10.0.2.2"]; !ok {
		t.Error("curated device must never be evicted by the cap")
	}
	if _, ok := devs["10.0.2.3"]; ok {
		t.Error("plain old device should be evicted")
	}
	if _, ok := devs["10.0.2.4"]; ok {
		t.Error("plain old device should be evicted")
	}
	if len(devs) != 2 {
		t.Errorf("only the pinned (online + curated) devices remain, want 2, got %d", len(devs))
	}
}

func TestZeroLimitsDisableEnforcement(t *testing.T) {
	s := newTestStorage(t)
	now := time.Now()
	seedDevice(s, "10.0.3.1", now.Add(-1000*24*time.Hour), nil) // ancient
	for i := 0; i < 50; i++ {
		seedDevice(s, fmt.Sprintf("10.0.4.%d", i), now.Add(-1000*time.Hour), nil)
	}
	if err := s.SetLimits(0, 0); err != nil { // both off
		t.Fatalf("SetLimits: %v", err)
	}
	if got := len(s.GetDevices()); got != 51 {
		t.Errorf("zero limits must not remove anything, kept %d of 51", got)
	}
}

func TestEnforcementRunsAndPersistsOnScan(t *testing.T) {
	dir := t.TempDir()
	devicesFile := filepath.Join(dir, "devices.json")
	stateFile := filepath.Join(dir, "state.json")
	s, err := New(devicesFile, stateFile)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	now := time.Now()
	seedDevice(s, "10.0.5.1", now.Add(-200*24*time.Hour), nil) // old, should go on the next scan
	// Enable retention without SetLimits' immediate sweep, to prove the scan
	// (merge) path itself enforces.
	s.mu.Lock()
	s.retentionDays = 90
	s.mu.Unlock()

	if err := s.MergeDevices([]types.Device{{IP: "10.0.5.2", MAC: "aa:bb:cc:dd:ee:01"}}); err != nil {
		t.Fatalf("MergeDevices: %v", err)
	}
	if _, ok := s.GetDevices()["10.0.5.1"]; ok {
		t.Error("a scan should have pruned the device offline past retention")
	}
	// The prune is saved, not just in memory: a reload does not resurrect it.
	s2, err := New(devicesFile, stateFile)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	if _, ok := s2.GetDevices()["10.0.5.1"]; ok {
		t.Error("the prune should have been written to disk")
	}
}

func TestLimitOverridesResolveAndPersist(t *testing.T) {
	dir := t.TempDir()
	devices := filepath.Join(dir, "devices.json")
	state := filepath.Join(dir, "state.json")

	first, err := New(devices, state)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	// No override yet: the config default wins.
	if got := first.RetentionDaysOrDefault(90); got != 90 {
		t.Errorf("with no override, want config default 90, got %d", got)
	}
	if got := first.MaxDevicesOrDefault(1000); got != 1000 {
		t.Errorf("with no override, want config default 1000, got %d", got)
	}

	// Save overrides: retention 30, cap 250.
	r, m := 30, 250
	if err := first.SetLimitOverrides(&r, &m); err != nil {
		t.Fatalf("SetLimitOverrides: %v", err)
	}
	if got := first.RetentionDaysOrDefault(90); got != 30 {
		t.Errorf("override should win, want 30, got %d", got)
	}

	// Survives a restart.
	second, err := New(devices, state)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	if got := second.RetentionDaysOrDefault(90); got != 30 {
		t.Errorf("override should survive reload, want 30, got %d", got)
	}
	if got := second.MaxDevicesOrDefault(1000); got != 250 {
		t.Errorf("override should survive reload, want 250, got %d", got)
	}
	ro, mo := second.GetLimitOverrides()
	if ro == nil || *ro != 30 || mo == nil || *mo != 250 {
		t.Errorf("GetLimitOverrides = %v,%v want 30,250", ro, mo)
	}

	// Clearing an override falls back to the config default.
	if err := second.SetLimitOverrides(nil, &m); err != nil {
		t.Fatalf("clear retention override: %v", err)
	}
	if got := second.RetentionDaysOrDefault(90); got != 90 {
		t.Errorf("cleared override should fall back to default 90, got %d", got)
	}
	if got := second.MaxDevicesOrDefault(1000); got != 250 {
		t.Errorf("max override should remain 250, got %d", got)
	}
}

func TestServiceDetectionOverrideResolvesAndPersists(t *testing.T) {
	dir := t.TempDir()
	devices := filepath.Join(dir, "devices.json")
	state := filepath.Join(dir, "state.json")

	first, err := New(devices, state)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	// No override: config default wins, both ways.
	if !first.ServiceDetectionEnabled(true) {
		t.Error("with no override, want config default true")
	}
	if first.ServiceDetectionEnabled(false) {
		t.Error("with no override, want config default false")
	}
	// Turn it on even though config defaults it off.
	if err := first.SetServiceDetection(true); err != nil {
		t.Fatalf("SetServiceDetection: %v", err)
	}
	if !first.ServiceDetectionEnabled(false) {
		t.Error("override (true) should win over config default (false)")
	}
	// Survives a restart.
	second, err := New(devices, state)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	if !second.ServiceDetectionEnabled(false) {
		t.Error("override should survive reload")
	}
}

func TestScanIntervalOverrideResolvesAndPersists(t *testing.T) {
	dir := t.TempDir()
	devices := filepath.Join(dir, "devices.json")
	state := filepath.Join(dir, "state.json")

	first, err := New(devices, state)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if got := first.ScanIntervalOrDefault(300); got != 300 {
		t.Errorf("with no override, want config default 300, got %d", got)
	}
	if err := first.SetScanInterval(60); err != nil {
		t.Fatalf("SetScanInterval: %v", err)
	}
	if got := first.ScanIntervalOrDefault(300); got != 60 {
		t.Errorf("override should win, want 60, got %d", got)
	}
	second, err := New(devices, state)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	if got := second.ScanIntervalOrDefault(300); got != 60 {
		t.Errorf("override should survive reload, want 60, got %d", got)
	}
}

func TestNetworkOverridesResolveAndPersist(t *testing.T) {
	dir := t.TempDir()
	devices := filepath.Join(dir, "devices.json")
	state := filepath.Join(dir, "state.json")

	first, err := New(devices, state)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	// No override: config defaults win.
	if got := first.NetworksOrDefault([]string{"10.0.0.0/24"}); len(got) != 1 || got[0] != "10.0.0.0/24" {
		t.Errorf("no override should return the config default, got %v", got)
	}
	if first.OnlyConfiguredOrDefault(false) {
		t.Error("no override should return the config default (false)")
	}

	// Save overrides.
	inc := []string{"192.168.10.0/24", "10.5.0.0/16"}
	exc := []string{"10.5.99.0/24"}
	only := true
	if err := first.SetNetworkOverrides(&inc, &exc, &only); err != nil {
		t.Fatalf("SetNetworkOverrides: %v", err)
	}
	if got := first.NetworksOrDefault(nil); len(got) != 2 {
		t.Errorf("include override should win, got %v", got)
	}
	if !first.OnlyConfiguredOrDefault(false) {
		t.Error("only_configured override should win")
	}

	// Survives a restart.
	second, err := New(devices, state)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	if got := second.ExcludeNetworksOrDefault(nil); len(got) != 1 || got[0] != "10.5.99.0/24" {
		t.Errorf("exclude override should survive reload, got %v", got)
	}

	// An explicit empty override is distinct from "no override": include is
	// cleared to empty, exclude falls back to the config default.
	empty := []string{}
	if err := second.SetNetworkOverrides(&empty, nil, nil); err != nil {
		t.Fatalf("set empty: %v", err)
	}
	if got := second.NetworksOrDefault([]string{"cfg"}); len(got) != 0 {
		t.Errorf("an explicit empty override should win over the config default, got %v", got)
	}
	if got := second.ExcludeNetworksOrDefault([]string{"cfgexc"}); len(got) != 1 || got[0] != "cfgexc" {
		t.Errorf("cleared exclude override should fall back to the config default, got %v", got)
	}
}

func TestSnapshotSaveListGetDelete(t *testing.T) {
	s := newTestStorage(t)
	if err := s.MergeDevices([]types.Device{
		{IP: "192.168.1.5", MAC: "aa:bb:cc:00:00:01", Hostname: "nas"},
		{IP: "192.168.1.6", MAC: "aa:bb:cc:00:00:02"},
	}); err != nil {
		t.Fatal(err)
	}
	meta, err := s.SaveSnapshot("baseline")
	if err != nil {
		t.Fatalf("SaveSnapshot: %v", err)
	}
	if meta.Name != "baseline" || meta.DeviceCount != 2 {
		t.Errorf("meta = %+v, want name baseline / count 2", meta)
	}
	metas, err := s.ListSnapshots()
	if err != nil {
		t.Fatal(err)
	}
	if len(metas) != 1 || metas[0].ID != meta.ID {
		t.Fatalf("list = %+v, want the one snapshot", metas)
	}
	full, err := s.GetSnapshot(meta.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(full.Devices) != 2 {
		t.Errorf("snapshot devices = %d, want 2", len(full.Devices))
	}
	if err := s.DeleteSnapshot(meta.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if metas, _ = s.ListSnapshots(); len(metas) != 0 {
		t.Errorf("after delete, list = %+v, want empty", metas)
	}
}

func TestSnapshotRejectsBadID(t *testing.T) {
	s := newTestStorage(t)
	for _, bad := range []string{"", "../etc/passwd", "abc", "12a", "1/2"} {
		if _, err := s.GetSnapshot(bad); err == nil {
			t.Errorf("GetSnapshot(%q) should reject a non-numeric id", bad)
		}
	}
}

func TestSnapshotEmptyNameDefaults(t *testing.T) {
	s := newTestStorage(t)
	meta, err := s.SaveSnapshot("   ")
	if err != nil {
		t.Fatal(err)
	}
	if meta.Name == "" {
		t.Error("a blank name should default to a timestamp, not stay empty")
	}
}

func TestBackupExportImportRoundtrip(t *testing.T) {
	dir := t.TempDir()
	s, err := New(filepath.Join(dir, "devices.json"), filepath.Join(dir, "state.json"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	// Seed a curated device, a settings override, and a snapshot.
	if err := s.MergeDevices([]types.Device{
		{IP: "192.168.1.5", MAC: "aa:bb:cc:00:00:01", Label: "NAS", Group: "Server"},
	}); err != nil {
		t.Fatal(err)
	}
	rv := 45
	if err := s.SetLimitOverrides(&rv, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SaveSnapshot("snap1"); err != nil {
		t.Fatal(err)
	}

	backup := s.ExportBackup()
	if len(backup.Devices) != 1 || len(backup.Snapshots) != 1 {
		t.Fatalf("backup should have 1 device and 1 snapshot, got %d/%d", len(backup.Devices), len(backup.Snapshots))
	}
	if backup.CreatedAt.IsZero() {
		t.Error("backup should carry a creation time")
	}

	// Restore into a fresh, separate instance.
	dir2 := t.TempDir()
	s2, err := New(filepath.Join(dir2, "devices.json"), filepath.Join(dir2, "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := s2.ImportBackup(backup); err != nil {
		t.Fatalf("ImportBackup: %v", err)
	}
	devs := s2.GetDevices()
	if len(devs) != 1 || devs["192.168.1.5"] == nil || devs["192.168.1.5"].Label != "NAS" {
		t.Errorf("restored devices wrong: %+v", devs)
	}
	if s2.RetentionDaysOrDefault(90) != 45 {
		t.Errorf("restored retention override should be 45, got %d", s2.RetentionDaysOrDefault(90))
	}
	metas, _ := s2.ListSnapshots()
	if len(metas) != 1 || metas[0].Name != "snap1" {
		t.Errorf("restored snapshots wrong: %+v", metas)
	}

	// A reload sees the restored, persisted data.
	s3, err := New(filepath.Join(dir2, "devices.json"), filepath.Join(dir2, "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(s3.GetDevices()) != 1 {
		t.Error("restored devices should persist across reload")
	}
}

func boolptr(b bool) *bool { return &b }

func TestExpectedFlagAndBaseline(t *testing.T) {
	s := newTestStorage(t)
	if err := s.MergeDevices([]types.Device{
		{IP: "192.168.1.5", MAC: "aa:bb:cc:00:00:01"},
		{IP: "192.168.1.6", MAC: "aa:bb:cc:00:00:02"},
	}); err != nil {
		t.Fatal(err)
	}
	// Mark one expected via UpdateDeviceFields.
	if err := s.UpdateDeviceFields("192.168.1.5", nil, nil, nil, boolptr(true)); err != nil {
		t.Fatal(err)
	}
	if !s.GetDevices()["192.168.1.5"].Expected {
		t.Error("device should be marked expected")
	}
	if s.GetDevices()["192.168.1.6"].Expected {
		t.Error("other device should not be expected")
	}
	// An expected device is curated, so retention must never prune it even when
	// long offline.
	s.mu.Lock()
	s.devices["192.168.1.5"].LastSeen = time.Now().Add(-365 * 24 * time.Hour)
	s.retentionDays = 90
	removed := s.enforceLimitsLocked(time.Now())
	s.mu.Unlock()
	_ = removed
	if _, ok := s.GetDevices()["192.168.1.5"]; !ok {
		t.Error("an expected device offline past retention must not be pruned")
	}
	// Baseline the whole list.
	if err := s.MarkAllExpected(true); err != nil {
		t.Fatal(err)
	}
	for ip, d := range s.GetDevices() {
		if !d.Expected {
			t.Errorf("%s should be expected after MarkAllExpected(true)", ip)
		}
	}
	// And clear it.
	if err := s.MarkAllExpected(false); err != nil {
		t.Fatal(err)
	}
	for ip, d := range s.GetDevices() {
		if d.Expected {
			t.Errorf("%s should not be expected after MarkAllExpected(false)", ip)
		}
	}
}
