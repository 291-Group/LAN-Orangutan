package cli

import (
	"path/filepath"
	"testing"

	"github.com/291-Group/LAN-Orangutan/internal/config"
	"github.com/291-Group/LAN-Orangutan/internal/storage"
)

func testStore(t *testing.T) *storage.Storage {
	t.Helper()
	// settingRows reads the package-level config the way the commands do.
	cfg = config.Default()
	dir := t.TempDir()
	store, err := storage.New(filepath.Join(dir, "devices.json"), filepath.Join(dir, "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func TestSetSettingClampsAndReportsOverride(t *testing.T) {
	store := testStore(t)
	if err := setSetting(store, "scan_interval", "5"); err != nil {
		t.Fatal(err)
	}
	if got := store.ScanIntervalOrDefault(300); got != settingsMinInterval {
		t.Fatalf("interval = %d, want clamped to %d", got, settingsMinInterval)
	}
	for _, r := range settingRows(store) {
		if r.Key == "scan_interval" && !r.Overridden {
			t.Fatal("scan_interval should show as an override")
		}
	}
	if err := resetSetting(store, "scan_interval"); err != nil {
		t.Fatal(err)
	}
	if _, interval, _ := store.GetScanOverrides(); interval != nil {
		t.Fatal("reset should clear the override")
	}
}

func TestSetNetworksLeavesTheOtherListsAlone(t *testing.T) {
	store := testStore(t)
	if err := setSetting(store, "exclude_networks", "10.99.0.0/16"); err != nil {
		t.Fatal(err)
	}
	if err := setSetting(store, "networks", "192.168.10.0/24, 192.168.10.0/24,10.0.5.0/24"); err != nil {
		t.Fatal(err)
	}
	networks, exclude, _ := store.GetNetworkOverrides()
	if networks == nil || len(*networks) != 2 {
		t.Fatalf("networks = %v, want two deduplicated entries", networks)
	}
	if exclude == nil || (*exclude)[0] != "10.99.0.0/16" {
		t.Fatalf("exclude list was lost: %v", exclude)
	}
	if err := setSetting(store, "networks", "nope"); err == nil {
		t.Fatal("a bad CIDR must be refused")
	}
	if err := setSetting(store, "networks", ""); err != nil {
		t.Fatal(err)
	}
	if networks, _, _ := store.GetNetworkOverrides(); networks != nil {
		t.Fatal("an empty value should clear the override")
	}
}

func TestSetSettingRejectsUnknownKeyAndBadBool(t *testing.T) {
	store := testStore(t)
	if err := setSetting(store, "theme", "dark"); err == nil {
		t.Fatal("theme is not a server setting")
	}
	if err := setSetting(store, "continuous_scan", "maybe"); err == nil {
		t.Fatal("a non yes/no value must be refused")
	}
	if err := setSetting(store, "continuous_scan", "off"); err != nil {
		t.Fatal(err)
	}
	if store.ContinuousScanEnabled(true) {
		t.Fatal("continuous_scan should be off")
	}
}
