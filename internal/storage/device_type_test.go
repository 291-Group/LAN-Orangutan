package storage

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/291-Group/LAN-Orangutan/internal/scanner"
	"github.com/291-Group/LAN-Orangutan/internal/types"
)

func TestUpdateDeviceFields_CustomType(t *testing.T) {
	s := newTestStorage(t)

	// Create initial device with auto-detected type
	if err := s.MergeDevices([]types.Device{
		{IP: "192.168.1.50", MAC: "50:c7:bf:11:22:33", Hostname: "switch-room", Type: "Router"},
	}); err != nil {
		t.Fatal(err)
	}

	dev := s.GetDevice("192.168.1.50")
	if dev == nil {
		t.Fatal("device not found")
	}
	if dev.Type != "Router" || dev.CustomType {
		t.Fatalf("expected initial type Router, custom_type false, got type=%q custom_type=%v", dev.Type, dev.CustomType)
	}

	// User overrides type to a supported taxonomy type "Server"
	customType := "Server"
	if err := s.UpdateDeviceFields("192.168.1.50", nil, nil, nil, nil, &customType, nil); err != nil {
		t.Fatalf("UpdateDeviceFields: %v", err)
	}

	dev = s.GetDevice("192.168.1.50")
	if dev.Type != "Server" {
		t.Errorf("expected Type 'Server', got %q", dev.Type)
	}
	if !dev.CustomType {
		t.Errorf("expected CustomType true, got %v", dev.CustomType)
	}
}

func TestUpdateDeviceFields_InvalidType_Rejected(t *testing.T) {
	s := newTestStorage(t)

	if err := s.MergeDevices([]types.Device{
		{IP: "192.168.1.51", MAC: "50:c7:bf:11:22:34", Hostname: "dev", Type: "Computer"},
	}); err != nil {
		t.Fatal(err)
	}

	// Arbitrary free-text type must be rejected
	invalidType := "Smart Plug"
	if err := s.UpdateDeviceFields("192.168.1.51", nil, nil, nil, nil, &invalidType, nil); err == nil {
		t.Fatal("expected error updating with invalid type 'Smart Plug', got nil")
	}

	// Oversized type must be rejected
	tooLong := strings.Repeat("A", scanner.MaxTypeLen+1)
	if err := s.UpdateDeviceFields("192.168.1.51", nil, nil, nil, nil, &tooLong, nil); err == nil {
		t.Fatal("expected error updating with oversized type, got nil")
	}

	// Control character must be rejected
	control := "Server\n"
	if err := s.UpdateDeviceFields("192.168.1.51", nil, nil, nil, nil, &control, nil); err == nil {
		t.Fatal("expected error updating with control character, got nil")
	}

	// Device should remain untouched
	dev := s.GetDevice("192.168.1.51")
	if dev.Type != "Computer" || dev.CustomType {
		t.Fatalf("expected original type Computer and CustomType false, got %q, %v", dev.Type, dev.CustomType)
	}
}

func TestMergeDevices_PreservesCustomType(t *testing.T) {
	s := newTestStorage(t)

	// Device originally scanned as Router
	if err := s.MergeDevices([]types.Device{
		{IP: "192.168.1.50", MAC: "50:c7:bf:11:22:33", Hostname: "switch-room", Type: "Router"},
	}); err != nil {
		t.Fatal(err)
	}

	// User overrides to standard device type "Server"
	userType := "Server"
	isCustom := true
	if err := s.UpdateDeviceFields("192.168.1.50", nil, nil, nil, nil, &userType, &isCustom); err != nil {
		t.Fatal(err)
	}

	// Subsequent scan discovers the device again and classifies it as Router
	if err := s.MergeDevices([]types.Device{
		{IP: "192.168.1.50", MAC: "50:c7:bf:11:22:33", Hostname: "switch-room", Type: "Router"},
	}); err != nil {
		t.Fatal(err)
	}

	dev := s.GetDevice("192.168.1.50")
	if dev.Type != "Server" {
		t.Errorf("scan merge should not overwrite custom type; expected 'Server', got %q", dev.Type)
	}
	if !dev.CustomType {
		t.Errorf("custom_type should remain true after scan merge; got %v", dev.CustomType)
	}
}

func TestMergeDevices_UpdatesNonCustomType(t *testing.T) {
	s := newTestStorage(t)

	// Device discovered with initial guess
	if err := s.MergeDevices([]types.Device{
		{IP: "192.168.1.60", MAC: "00:11:22:33:44:55", Hostname: "box", Type: "Computer"},
	}); err != nil {
		t.Fatal(err)
	}

	// Subsequent scan refines classification to Server
	if err := s.MergeDevices([]types.Device{
		{IP: "192.168.1.60", MAC: "00:11:22:33:44:55", Hostname: "box", Type: "Server"},
	}); err != nil {
		t.Fatal(err)
	}

	dev := s.GetDevice("192.168.1.60")
	if dev.Type != "Server" {
		t.Errorf("non-custom type should update on rescan; expected 'Server', got %q", dev.Type)
	}
}

func TestUpdateDeviceFields_ResetToAuto(t *testing.T) {
	s := newTestStorage(t)

	if err := s.MergeDevices([]types.Device{
		{IP: "192.168.1.70", MAC: "aa:bb:cc:11:22:33", Hostname: "dev", Type: "Computer"},
	}); err != nil {
		t.Fatal(err)
	}

	// User assigns custom type
	customType := "Game Console"
	if err := s.UpdateDeviceFields("192.168.1.70", nil, nil, nil, nil, &customType, nil); err != nil {
		t.Fatal(err)
	}
	if d := s.GetDevice("192.168.1.70"); !d.CustomType {
		t.Fatal("expected CustomType to be true")
	}

	// User resets back to auto (empty string)
	empty := ""
	if err := s.UpdateDeviceFields("192.168.1.70", nil, nil, nil, nil, &empty, nil); err != nil {
		t.Fatal(err)
	}

	dev := s.GetDevice("192.168.1.70")
	if dev.Type != "" {
		t.Errorf("expected empty type after reset, got %q", dev.Type)
	}
	if dev.CustomType {
		t.Errorf("expected CustomType false after reset, got %v", dev.CustomType)
	}

	// Next scan can now re-classify device
	if err := s.MergeDevices([]types.Device{
		{IP: "192.168.1.70", MAC: "aa:bb:cc:11:22:33", Hostname: "dev", Type: "Server"},
	}); err != nil {
		t.Fatal(err)
	}

	dev = s.GetDevice("192.168.1.70")
	if dev.Type != "Server" {
		t.Errorf("expected re-classified Server after reset, got %q", dev.Type)
	}
}

func TestCustomTypePersistsAcrossIPChange(t *testing.T) {
	s := newTestStorage(t)

	// First seen at .10 with custom type
	if err := s.MergeDevices([]types.Device{
		{IP: "192.168.1.10", MAC: "aa:bb:cc:dd:ee:01", Hostname: "iot-device"},
	}); err != nil {
		t.Fatal(err)
	}
	iotType := "IoT"
	if err := s.UpdateDeviceFields("192.168.1.10", nil, nil, nil, nil, &iotType, nil); err != nil {
		t.Fatal(err)
	}

	// Device moves to .20, scan reports Router guess
	if err := s.MergeDevices([]types.Device{
		{IP: "192.168.1.20", MAC: "aa:bb:cc:dd:ee:01", Hostname: "iot-device", Type: "Router"},
	}); err != nil {
		t.Fatal(err)
	}

	moved := s.GetDevice("192.168.1.20")
	if moved == nil {
		t.Fatal("device not found at new IP")
	}
	if moved.Type != "IoT" {
		t.Errorf("expected custom type to carry over to new IP, got %q", moved.Type)
	}
	if !moved.CustomType {
		t.Errorf("expected CustomType flag to carry over to new IP, got %v", moved.CustomType)
	}
}

func TestMergeSupplementalAndIPv6_PreservesCustomType(t *testing.T) {
	s := newTestStorage(t)

	// Primary device with custom type
	if err := s.MergeDevices([]types.Device{
		{IP: "192.168.1.80", MAC: "00:1a:2b:3c:4d:88", Hostname: "my-pi"},
	}); err != nil {
		t.Fatal(err)
	}
	customType := "Server"
	if err := s.UpdateDeviceFields("192.168.1.80", nil, nil, nil, nil, &customType, nil); err != nil {
		t.Fatal(err)
	}

	// Secondary supplemental source reports different type
	if err := s.MergeSupplemental([]types.Device{
		{IP: "192.168.1.80", Type: "Router"},
	}); err != nil {
		t.Fatal(err)
	}

	// IPv6 neighbor discovery reports different type
	if err := s.MergeIPv6Neighbors([]types.Device{
		{IP: "2001:db8::80", MAC: "00:1a:2b:3c:4d:88", Type: "Computer"},
	}); err != nil {
		t.Fatal(err)
	}

	dev := s.GetDevice("192.168.1.80")
	if dev.Type != "Server" {
		t.Errorf("custom type was overwritten by secondary sources, got %q", dev.Type)
	}
	if !dev.CustomType {
		t.Errorf("CustomType flag was lost; got %v", dev.CustomType)
	}
}

func TestDeviceSerializationWithCustomType(t *testing.T) {
	dir := t.TempDir()
	devicesFile := filepath.Join(dir, "devices.json")
	stateFile := filepath.Join(dir, "state.json")

	s1, err := New(devicesFile, stateFile)
	if err != nil {
		t.Fatal(err)
	}

	if err := s1.MergeDevices([]types.Device{
		{IP: "192.168.1.90", MAC: "11:22:33:44:55:66", Hostname: "nas"},
	}); err != nil {
		t.Fatal(err)
	}

	customType := "Server"
	if err := s1.UpdateDeviceFields("192.168.1.90", nil, nil, nil, nil, &customType, nil); err != nil {
		t.Fatal(err)
	}

	// Verify file on disk contains custom_type JSON field
	data, err := os.ReadFile(devicesFile)
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	entry := raw["192.168.1.90"].(map[string]interface{})
	if entry["type"] != "Server" {
		t.Errorf("expected JSON type 'Server', got %v", entry["type"])
	}
	if entry["custom_type"] != true {
		t.Errorf("expected JSON custom_type true, got %v", entry["custom_type"])
	}

	// Reopen storage and confirm reload preserves custom type
	s2, err := New(devicesFile, stateFile)
	if err != nil {
		t.Fatal(err)
	}
	dev := s2.GetDevice("192.168.1.90")
	if dev == nil || dev.Type != "Server" || !dev.CustomType {
		t.Fatalf("reloaded device should have type 'Server' and CustomType true, got %+v", dev)
	}
}

func TestRetentionKeepsCustomTypeDevice(t *testing.T) {
	s := newTestStorage(t)
	now := time.Now()

	// Seed an offline device with CustomType true and no label/notes/group/expected
	seedDevice(s, "10.0.0.19", now.Add(-365*24*time.Hour), func(d *types.Device) {
		d.Type = "Server"
		d.CustomType = true
	})

	if !isCurated(s.GetDevice("10.0.0.19")) {
		t.Fatal("device with CustomType: true must be curated")
	}

	if err := s.SetLimits(90, 0); err != nil {
		t.Fatalf("SetLimits: %v", err)
	}

	if _, ok := s.GetDevices()["10.0.0.19"]; !ok {
		t.Error("a device with custom type must never be pruned by retention, even a year offline")
	}
}

func TestMaxDevicesCapProtectsCustomTypeDevice(t *testing.T) {
	s := newTestStorage(t)
	now := time.Now()

	// Five offline devices (> 1h old), staggered by age.
	for i := 1; i <= 4; i++ {
		seedDevice(s, fmt.Sprintf("10.0.1.%d", i), now.Add(-time.Duration(i)*24*time.Hour), nil)
	}
	// The oldest device (.5) is curated via CustomType: true
	seedDevice(s, "10.0.1.5", now.Add(-5*24*time.Hour), func(d *types.Device) {
		d.Type = "Printer"
		d.CustomType = true
	})

	// Cap at 3 devices
	if err := s.SetLimits(0, 3); err != nil {
		t.Fatalf("SetLimits: %v", err)
	}

	devs := s.GetDevices()
	if _, ok := devs["10.0.1.5"]; !ok {
		t.Errorf("custom-typed device 10.0.1.5 was evicted by max-devices cap; should be protected as curated")
	}
}
