// Package storage handles device persistence and state management
package storage

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/291-Group/LAN-Orangutan/internal/types"
)

// Storage manages device data persistence
type Storage struct {
	devicesFile string
	stateFile   string
	mu          sync.RWMutex
	devices     map[string]*types.Device
	state       *types.ScanState
}

// New creates a new Storage instance
func New(devicesFile, stateFile string) (*Storage, error) {
	s := &Storage{
		devicesFile: devicesFile,
		stateFile:   stateFile,
		devices:     make(map[string]*types.Device),
		state: &types.ScanState{
			LastScan:     make(map[string]time.Time),
			LastDuration: make(map[string]float64),
		},
	}

	// Ensure directories exist
	if err := os.MkdirAll(filepath.Dir(devicesFile), 0755); err != nil {
		return nil, fmt.Errorf("failed to create data directory: %w", err)
	}

	// Load existing data. Name the file in the error: a damaged file is
	// something the user has to go and look at, and "invalid character 'h'"
	// on its own does not say where to look.
	if err := s.loadDevices(); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("could not read the device list at %s: %w", devicesFile, err)
	}
	if err := s.loadState(); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("could not read the scan history at %s: %w", stateFile, err)
	}

	return s, nil
}

// loadDevices reads devices from the JSON file
func (s *Storage) loadDevices() error {
	data, err := os.ReadFile(s.devicesFile)
	if err != nil {
		return err
	}

	if len(data) == 0 {
		return nil
	}

	return json.Unmarshal(data, &s.devices)
}

// loadState reads scan state from the JSON file
func (s *Storage) loadState() error {
	data, err := os.ReadFile(s.stateFile)
	if err != nil {
		return err
	}

	if len(data) == 0 {
		return nil
	}

	if err := json.Unmarshal(data, &s.state); err != nil {
		return err
	}

	// State files written before durations were tracked have no such map, so
	// recreate it rather than leave a nil map that cannot be written to.
	if s.state.LastScan == nil {
		s.state.LastScan = make(map[string]time.Time)
	}
	if s.state.LastDuration == nil {
		s.state.LastDuration = make(map[string]float64)
	}
	return nil
}

// saveDevices writes devices to the JSON file atomically
func (s *Storage) saveDevices() error {
	data, err := json.MarshalIndent(s.devices, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal devices: %w", err)
	}

	return atomicWrite(s.devicesFile, data)
}

// saveState writes scan state to the JSON file atomically
func (s *Storage) saveState() error {
	data, err := json.MarshalIndent(s.state, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal state: %w", err)
	}

	return atomicWrite(s.stateFile, data)
}

// atomicWrite writes data to a file atomically using a temp file
func atomicWrite(path string, data []byte) error {
	dir := filepath.Dir(path)
	tempFile, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	tempPath := tempFile.Name()

	// Clean up temp file on error
	defer func() {
		if tempPath != "" {
			os.Remove(tempPath)
		}
	}()

	if _, err := tempFile.Write(data); err != nil {
		tempFile.Close()
		return fmt.Errorf("failed to write temp file: %w", err)
	}

	if err := tempFile.Sync(); err != nil {
		tempFile.Close()
		return fmt.Errorf("failed to sync temp file: %w", err)
	}

	if err := tempFile.Close(); err != nil {
		return fmt.Errorf("failed to close temp file: %w", err)
	}

	if err := os.Rename(tempPath, path); err != nil {
		return fmt.Errorf("failed to rename temp file: %w", err)
	}

	tempPath = "" // Prevent cleanup of renamed file
	return nil
}

// GetDevices returns all devices
func (s *Storage) GetDevices() map[string]*types.Device {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make(map[string]*types.Device, len(s.devices))
	for k, v := range s.devices {
		result[k] = v
	}
	return result
}

// GetDevice returns a single device by IP
func (s *Storage) GetDevice(ip string) *types.Device {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.devices[ip]
}

// UpdateDevice updates or creates a device
func (s *Storage) UpdateDevice(device *types.Device) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Preserve existing user data if device exists
	if existing, ok := s.devices[device.IP]; ok {
		if device.Label == "" {
			device.Label = existing.Label
		}
		if device.Notes == "" {
			device.Notes = existing.Notes
		}
		if device.Group == "" {
			device.Group = existing.Group
		}
		if device.FirstSeen.IsZero() {
			device.FirstSeen = existing.FirstSeen
		}
	}

	s.devices[device.IP] = device
	return s.saveDevices()
}

// UpdateDeviceFields updates specific fields of a device
func (s *Storage) UpdateDeviceFields(ip string, label, notes, group *string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	device, ok := s.devices[ip]
	if !ok {
		return fmt.Errorf("device not found: %s", ip)
	}

	if label != nil {
		device.Label = *label
	}
	if notes != nil {
		device.Notes = *notes
	}
	if group != nil {
		device.Group = *group
	}

	return s.saveDevices()
}

// DeleteDevice removes a device by IP
func (s *Storage) DeleteDevice(ip string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.devices[ip]; !ok {
		return fmt.Errorf("device not found: %s", ip)
	}

	delete(s.devices, ip)
	return s.saveDevices()
}

// MergeDevices merges discovered devices with existing data
func (s *Storage) MergeDevices(discovered []types.Device) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	for _, d := range discovered {
		if existing, ok := s.devices[d.IP]; ok {
			// Update existing device, preserve user data
			existing.MAC = d.MAC
			existing.Hostname = d.Hostname
			existing.Vendor = d.Vendor
			existing.LastSeen = now
			existing.ResponseTime = d.ResponseTime
		} else {
			// New device
			d.FirstSeen = now
			d.LastSeen = now
			s.devices[d.IP] = &d
		}
	}

	return s.saveDevices()
}

// GetLastScan returns the last scan time for a network
func (s *Storage) GetLastScan(network string) time.Time {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.state.LastScan[network]
}

// GetMostRecentScan returns the time of the most recent scan of any network.
//
// The device list is only as current as the last scan, so the dashboard shows
// this alongside the per-device "last seen" times. Without it, a device last
// seen during a scan hours ago still reads as though it were just checked.
//
// The zero time means nothing has ever been scanned.
func (s *Storage) GetMostRecentScan() time.Time {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var latest time.Time
	for _, t := range s.state.LastScan {
		if t.After(latest) {
			latest = t
		}
	}
	return latest
}

// SetLastScan updates the last scan time for a network
func (s *Storage) SetLastScan(network string, t time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.state.LastScan[network] = t
	return s.saveState()
}

// GetLastDuration returns how long the previous scan of a network took, in
// seconds. It returns 0 when the network has not been scanned before.
func (s *Storage) GetLastDuration(network string) float64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.state.LastDuration[network]
}

// SetLastDuration records how long a scan of a network took, in seconds.
func (s *Storage) SetLastDuration(network string, seconds float64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.state.LastDuration == nil {
		s.state.LastDuration = make(map[string]float64)
	}
	s.state.LastDuration[network] = seconds
	return s.saveState()
}

// GetStats returns device statistics
func (s *Storage) GetStats() types.DeviceStats {
	s.mu.RLock()
	defer s.mu.RUnlock()

	stats := types.DeviceStats{
		Groups: make(map[string]int),
	}

	for _, d := range s.devices {
		stats.Total++
		if d.IsOnline() {
			stats.Online++
		} else {
			stats.Offline++
		}
		if d.Group != "" {
			stats.Groups[d.Group]++
		}
	}

	return stats
}
