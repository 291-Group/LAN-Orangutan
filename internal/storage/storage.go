// Package storage handles device persistence and state management
package storage

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/291-Group/LAN-Orangutan/internal/scanner"
	"github.com/291-Group/LAN-Orangutan/internal/types"
)

// Storage manages device data persistence
type Storage struct {
	devicesFile string
	stateFile   string
	mu          sync.RWMutex
	devices     map[string]*types.Device
	state       *types.ScanState

	// retentionDays and maxDevices bound the stored device list, set from config
	// via SetLimits. Zero means the corresponding limit is off. See
	// enforceLimitsLocked for the exact rules (curated and online devices are
	// never dropped).
	retentionDays int
	maxDevices    int
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

	// One-time cleanup: drop phantom devices created from noisy IPv6 neighbour
	// data (link-local and privacy addresses) before that path was fixed, so an
	// upgraded install heals down to the real device count. Safe here because
	// New runs before the store is shared with any other goroutine.
	if s.pruneEphemeralIPv6Locked() {
		_ = s.saveDevices()
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

// GetDevices returns all devices.
//
// Each device is copied, not shared. A later scan merges results by mutating
// the stored structs in place, so handing out the live pointers would let a
// caller read a field while it is being written. The copies are snapshots the
// caller can read freely.
func (s *Storage) GetDevices() map[string]*types.Device {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make(map[string]*types.Device, len(s.devices))
	for k, v := range s.devices {
		cp := *v
		result[k] = &cp
	}
	return result
}

// GetDevice returns a single device by IP, or nil if there is none.
//
// The returned device is a copy, for the same reason as GetDevices: the stored
// struct is mutated in place by later scans.
func (s *Storage) GetDevice(ip string) *types.Device {
	s.mu.RLock()
	defer s.mu.RUnlock()
	d, ok := s.devices[ip]
	if !ok {
		return nil
	}
	cp := *d
	return &cp
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
		if device.Type == "" {
			device.Type = existing.Type
			device.CustomType = existing.CustomType
		} else if existing.CustomType && !device.CustomType && device.Type == existing.Type {
			device.CustomType = true
		}
		if device.FirstSeen.IsZero() {
			device.FirstSeen = existing.FirstSeen
		}
	}

	s.devices[device.IP] = device
	return s.saveDevices()
}

// UpdateDeviceFields updates specific fields of a device
func (s *Storage) UpdateDeviceFields(ip string, label, notes, group *string, expected *bool, deviceType *string, customType *bool) error {
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
	if expected != nil {
		device.Expected = *expected
	}
	if deviceType != nil {
		cleanType, err := scanner.ValidateType(*deviceType)
		if err != nil {
			return err
		}
		device.Type = cleanType
		if customType != nil {
			device.CustomType = *customType
		} else {
			device.CustomType = (cleanType != "")
		}
	} else if customType != nil {
		device.CustomType = *customType
	}

	return s.saveDevices()
}

// MarkAllExpected sets (or clears) the Expected flag on every stored device, so a
// user can baseline their whole network in one action and then be flagged only on
// what changes afterwards.
func (s *Storage) MarkAllExpected(expected bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, d := range s.devices {
		d.Expected = expected
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
			// A primary scan is authoritative. It replaces the probe fields
			// (WebUI and Risks reflect the latest probe, so a fixed risk stops
			// showing), but it does not wipe an identity field it happened to
			// gather less of this time: a scan without sudo has no MAC, and a
			// device that did not resolve this time keeps its known name.
			if d.MAC != "" {
				existing.MAC = d.MAC
			}
			if d.Hostname != "" {
				existing.Hostname = d.Hostname
			}
			if d.Vendor != "" {
				existing.Vendor = d.Vendor
			}
			if !existing.CustomType && d.Type != "" {
				existing.Type = d.Type
			}
			if d.ResponseTime != nil {
				existing.ResponseTime = d.ResponseTime
			}
			existing.WebUI = d.WebUI
			existing.Risks = d.Risks
			existing.LastSeen = now
		} else {
			dev := d
			s.addNewDeviceLocked(&dev, now)
		}
	}

	s.enforceLimitsLocked(now)
	return s.saveDevices()
}

// MergeSupplemental folds in devices from a secondary source, such as mDNS or
// IPv6 neighbor discovery, that carries only part of a device's picture. It
// fills gaps on a known device but never clobbers what the primary scan found,
// and never touches the probe fields, so an mDNS answer with no MAC cannot erase
// a MAC the scan discovered, nor clear a web or risk flag.
func (s *Storage) MergeSupplemental(discovered []types.Device) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	for _, d := range discovered {
		// A link-local IPv6 address is never a device (same reasoning as
		// MergeIPv6Neighbors); skip it so a supplemental source like mDNS cannot
		// create a phantom keyed by fe80::.
		if parsed := net.ParseIP(d.IP); parsed != nil && parsed.To4() == nil && parsed.IsLinkLocalUnicast() {
			continue
		}
		if existing, ok := s.devices[d.IP]; ok {
			if existing.MAC == "" && d.MAC != "" {
				existing.MAC = d.MAC
			}
			if existing.Hostname == "" && d.Hostname != "" {
				existing.Hostname = d.Hostname
			}
			if existing.Vendor == "" && d.Vendor != "" {
				existing.Vendor = d.Vendor
			}
			if !existing.CustomType && existing.Type == "" && d.Type != "" {
				existing.Type = d.Type
			}
			existing.LastSeen = now
		} else {
			dev := d
			s.addNewDeviceLocked(&dev, now)
		}
	}

	s.enforceLimitsLocked(now)
	return s.saveDevices()
}

// MergeIPv6Neighbors folds IPv6 neighbour-discovery results into the inventory.
// The IPv6 neighbour cache is noisy: it is full of link-local and rotating
// privacy addresses, each often behind a randomised MAC, so a device sighted
// over IPv6 can look brand new every time. Using it to create a device per
// address is what produces hundreds of phantom entries. Instead it ENRICHES a
// device already found by the reliable IPv4/ARP scan, matched by MAC across
// address families. A new device is created only for a routable address whose
// MAC is a real (universally-administered) hardware address not already known --
// a genuine IPv6-only device. Link-local, and randomised-MAC addresses with no
// match, are dropped, so privacy addresses cannot pile up.
func (s *Storage) MergeIPv6Neighbors(discovered []types.Device) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	changed := false
	for _, d := range discovered {
		parsed := net.ParseIP(d.IP)
		if parsed == nil || parsed.To4() != nil || parsed.IsLinkLocalUnicast() || d.MAC == "" {
			continue // not a routable IPv6 address we can attribute
		}

		// Enrich the device we already know by this MAC rather than creating a
		// second entry for its IPv6 address.
		if existing := s.findAnyByMACLocked(d.MAC); existing != nil {
			if existing.Hostname == "" && d.Hostname != "" {
				existing.Hostname = d.Hostname
			}
			if existing.Vendor == "" && d.Vendor != "" {
				existing.Vendor = d.Vendor
			}
			if !existing.CustomType && existing.Type == "" && d.Type != "" {
				existing.Type = d.Type
			}
			// Record the routable IPv6 address on the IPv4-primary device so a
			// dual-stack device can show both. Skipped when the device we matched
			// is itself keyed by an IPv6 address, since its IP already shows it.
			if isIPv4(existing.IP) {
				existing.IPv6 = d.IP
			}
			existing.LastSeen = now
			changed = true
			continue
		}

		// No match: create a device only for a real hardware MAC (a genuine
		// IPv6-only device). A randomised MAC with no match is a privacy
		// address, not a device -- skip it so it cannot accumulate.
		if !isRandomizedMAC(d.MAC) {
			dev := d
			s.addNewDeviceLocked(&dev, now)
			changed = true
		}
	}

	if changed {
		s.enforceLimitsLocked(now)
		return s.saveDevices()
	}
	return nil
}

// addNewDeviceLocked stores a device newly seen at its IP. If one with the same
// MAC exists at another IP in the same address family, it moved: its identity
// and user data carry across and the change is recorded. Callers hold s.mu.
func (s *Storage) addNewDeviceLocked(d *types.Device, now time.Time) {
	if d.MAC != "" {
		if oldIP, old := s.findByMACLocked(d.MAC, d.IP); old != nil {
			d.Label = old.Label
			d.Notes = old.Notes
			d.Group = old.Group
			d.Expected = old.Expected
			d.FirstSeen = old.FirstSeen
			if old.CustomType || d.Type == "" {
				d.Type = old.Type
				d.CustomType = old.CustomType
			}
			d.AddressHistory = appendAddressChange(old.AddressHistory, oldIP, now)
			delete(s.devices, oldIP)
		}
	}
	if d.FirstSeen.IsZero() {
		d.FirstSeen = now
	}
	d.LastSeen = now
	s.devices[d.IP] = d
}

// findByMACLocked returns the IP and device of a stored device with the given
// MAC at an IP other than excludeIP, or "" and nil if there is none. Callers
// must hold s.mu.
//
// The match is confined to the same address family. A dual-stack device has an
// IPv4 and an IPv6 address at once, and those are not a "move" from one to the
// other: matching across families would wrongly collapse the two into one when
// IPv6 discovery runs alongside an IPv4 scan.
func (s *Storage) findByMACLocked(mac, excludeIP string) (string, *types.Device) {
	wantV4 := isIPv4(excludeIP)
	for ip, dev := range s.devices {
		if ip != excludeIP && dev.MAC == mac && isIPv4(ip) == wantV4 {
			return ip, dev
		}
	}
	return "", nil
}

func isIPv4(ip string) bool {
	parsed := net.ParseIP(ip)
	return parsed != nil && parsed.To4() != nil
}

// findAnyByMACLocked returns any stored device with the given MAC, regardless of
// address family, or nil. Used to attach an IPv6 sighting to the device already
// known from the IPv4 scan. Callers hold s.mu.
func (s *Storage) findAnyByMACLocked(mac string) *types.Device {
	if mac == "" {
		return nil
	}
	for _, dev := range s.devices {
		if dev.MAC == mac {
			return dev
		}
	}
	return nil
}

// isRandomizedMAC reports whether a MAC is locally administered (the
// second-least-significant bit of the first octet is set), which is how phones
// and laptops mark privacy-randomised addresses. Such a MAC cannot reliably
// identify a device across sightings.
func isRandomizedMAC(mac string) bool {
	hw, err := net.ParseMAC(mac)
	if err != nil || len(hw) == 0 {
		return false
	}
	return hw[0]&0x02 != 0
}

// pruneEphemeralIPv6Locked removes phantom devices that noisy IPv6 neighbour
// data created before that path was fixed: link-local addresses, and privacy
// (randomised-MAC) IPv6 addresses, which are not real, persistent devices. It
// runs on load so existing installs heal down to the real device count; once
// healed it is a no-op, because no code path creates such entries any more.
//
// It never removes a device the user has curated (a label, notes or a group),
// so a real device that happens to be keyed by such an address keeps its data
// instead of being wiped and re-created on every restart. Returns whether
// anything was removed. Callers hold s.mu.
func (s *Storage) pruneEphemeralIPv6Locked() bool {
	removed := false
	for ip, dev := range s.devices {
		parsed := net.ParseIP(ip)
		if parsed == nil || parsed.To4() != nil {
			continue // not IPv6
		}
		if isCurated(dev) {
			continue // curated by the user: never auto-delete
		}
		if parsed.IsLinkLocalUnicast() || (dev.MAC != "" && isRandomizedMAC(dev.MAC)) {
			delete(s.devices, ip)
			removed = true
		}
	}
	return removed
}

// isCurated reports whether the user has personalised a device with a label,
// notes, a group, expected status, or a custom device type. Curated devices
// are never removed by any automatic cleanup: the user told us this one
// matters, so retention, the device cap, and the IPv6 phantom prune all
// leave it alone.
func isCurated(d *types.Device) bool {
	return d.Label != "" || d.Notes != "" || d.Group != "" || d.Expected || d.CustomType
}

// SetLimits records the retention window (in days) and the maximum device count,
// normally from config, and applies them once immediately so an existing install
// heals on startup rather than only on the next scan. A zero value turns that
// limit off. Call it once after New, before the store is shared with request
// handlers.
func (s *Storage) SetLimits(retentionDays, maxDevices int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.retentionDays = retentionDays
	s.maxDevices = maxDevices
	if s.enforceLimitsLocked(time.Now()) {
		return s.saveDevices()
	}
	return nil
}

// enforceLimitsLocked applies the storage limits and reports whether it removed
// anything. First retention: drop devices last seen longer ago than
// retentionDays. Then the cap: if the list still exceeds maxDevices, evict the
// devices with the oldest LastSeen until it fits.
//
// Two kinds of device are never removed by either step: ones the user has
// curated (isCurated), and ones that are currently online. So the cleanup only
// ever reclaims stale, untouched entries, and the count can legitimately sit
// above maxDevices when that many devices are pinned by those rules -- keeping
// real, in-use data matters more than honouring a number. A zero limit is off.
// Callers hold s.mu.
func (s *Storage) enforceLimitsLocked(now time.Time) bool {
	removed := false

	if s.retentionDays > 0 {
		cutoff := now.Add(-time.Duration(s.retentionDays) * 24 * time.Hour)
		for ip, dev := range s.devices {
			if isCurated(dev) || now.Sub(dev.LastSeen) < time.Hour {
				continue
			}
			if dev.LastSeen.Before(cutoff) {
				delete(s.devices, ip)
				removed = true
			}
		}
	}

	if s.maxDevices > 0 && len(s.devices) > s.maxDevices {
		// Collect the devices eligible for eviction (not curated, not online),
		// oldest LastSeen first, and drop from the front until we are within the
		// cap or run out of eligible devices.
		evictable := make([]string, 0, len(s.devices))
		for ip, dev := range s.devices {
			if isCurated(dev) || now.Sub(dev.LastSeen) < time.Hour {
				continue
			}
			evictable = append(evictable, ip)
		}
		sort.Slice(evictable, func(i, j int) bool {
			return s.devices[evictable[i]].LastSeen.Before(s.devices[evictable[j]].LastSeen)
		})
		for _, ip := range evictable {
			if len(s.devices) <= s.maxDevices {
				break
			}
			delete(s.devices, ip)
			removed = true
		}
	}

	return removed
}

// maxAddressHistory bounds how many previous addresses a device keeps, so the
// history cannot grow without limit on a device that changes IP often.
const maxAddressHistory = 10

func appendAddressChange(history []types.AddressChange, ip string, at time.Time) []types.AddressChange {
	history = append(history, types.AddressChange{IP: ip, ChangedAt: at})
	if len(history) > maxAddressHistory {
		history = history[len(history)-maxAddressHistory:]
	}
	return history
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

// ContinuousScanEnabled reports whether background scanning should run. It
// honours a runtime override the user set from the UI, and falls back to the
// configured default when no override has been saved.
func (s *Storage) ContinuousScanEnabled(configDefault bool) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.state.ContinuousScan != nil {
		return *s.state.ContinuousScan
	}
	return configDefault
}

// SetContinuousScan saves the user's runtime override for background scanning so
// it survives a restart.
func (s *Storage) SetContinuousScan(enabled bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state.ContinuousScan = &enabled
	return s.saveState()
}

// RetentionDaysOrDefault resolves the retention window: the UI override if one
// has been saved, otherwise the configured default.
func (s *Storage) RetentionDaysOrDefault(configDefault int) int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.state.RetentionDays != nil {
		return *s.state.RetentionDays
	}
	return configDefault
}

// MaxDevicesOrDefault resolves the device cap: the UI override if one has been
// saved, otherwise the configured default.
func (s *Storage) MaxDevicesOrDefault(configDefault int) int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.state.MaxDevices != nil {
		return *s.state.MaxDevices
	}
	return configDefault
}

// GetLimitOverrides returns the saved UI overrides for the storage limits, or nil
// for a limit that is following the config default. The returned pointers are
// copies, so the caller can read them without holding the lock.
func (s *Storage) GetLimitOverrides() (retentionDays, maxDevices *int) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.state.RetentionDays != nil {
		v := *s.state.RetentionDays
		retentionDays = &v
	}
	if s.state.MaxDevices != nil {
		v := *s.state.MaxDevices
		maxDevices = &v
	}
	return
}

// SetLimitOverrides saves the UI overrides for the storage limits so they survive
// a restart. A nil pointer clears that override, falling back to the config
// default. It only persists the choice; the caller applies it with SetLimits.
func (s *Storage) SetLimitOverrides(retentionDays, maxDevices *int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state.RetentionDays = retentionDays
	s.state.MaxDevices = maxDevices
	return s.saveState()
}

// ServiceDetectionEnabled reports whether the opt-in port probe should run,
// honouring a UI override and falling back to the configured default.
func (s *Storage) ServiceDetectionEnabled(configDefault bool) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.state.EnableServiceDetection != nil {
		return *s.state.EnableServiceDetection
	}
	return configDefault
}

// SetServiceDetection saves the user's runtime override for the port probe so it
// survives a restart.
func (s *Storage) SetServiceDetection(enabled bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state.EnableServiceDetection = &enabled
	return s.saveState()
}

// ScanIntervalOrDefault resolves how often the background scanner runs, in
// seconds: the UI override if one has been saved, otherwise the config default.
func (s *Storage) ScanIntervalOrDefault(configDefault int) int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.state.ScanInterval != nil {
		return *s.state.ScanInterval
	}
	return configDefault
}

// SetScanInterval saves the user's runtime override for the background scan
// interval (in seconds) so it survives a restart.
func (s *Storage) SetScanInterval(seconds int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state.ScanInterval = &seconds
	return s.saveState()
}

// NetworksOrDefault resolves the include list (extra CIDRs to scan): the UI
// override if one has been saved, otherwise the configured default. The returned
// slice is a copy.
func (s *Storage) NetworksOrDefault(configDefault []string) []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.state.Networks != nil {
		return append([]string(nil), *s.state.Networks...)
	}
	return configDefault
}

// ExcludeNetworksOrDefault resolves the exclude list (CIDRs to hide) the same
// way as NetworksOrDefault.
func (s *Storage) ExcludeNetworksOrDefault(configDefault []string) []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.state.ExcludeNetworks != nil {
		return append([]string(nil), *s.state.ExcludeNetworks...)
	}
	return configDefault
}

// OnlyConfiguredOrDefault resolves whether to scan only the configured networks:
// the UI override if set, otherwise the configured default.
func (s *Storage) OnlyConfiguredOrDefault(configDefault bool) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.state.OnlyConfiguredNetworks != nil {
		return *s.state.OnlyConfiguredNetworks
	}
	return configDefault
}

// GetNetworkOverrides returns the saved UI overrides for the scan-target lists,
// or nil for a setting that is following the config default. Slices are copied.
func (s *Storage) GetNetworkOverrides() (networks, exclude *[]string, onlyConfigured *bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.state.Networks != nil {
		v := append([]string(nil), *s.state.Networks...)
		networks = &v
	}
	if s.state.ExcludeNetworks != nil {
		v := append([]string(nil), *s.state.ExcludeNetworks...)
		exclude = &v
	}
	if s.state.OnlyConfiguredNetworks != nil {
		v := *s.state.OnlyConfiguredNetworks
		onlyConfigured = &v
	}
	return
}

// SetNetworkOverrides saves the UI overrides for the scan-target lists so they
// survive a restart. A nil pointer clears that override, falling back to the
// config default.
func (s *Storage) SetNetworkOverrides(networks, exclude *[]string, onlyConfigured *bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state.Networks = networks
	s.state.ExcludeNetworks = exclude
	s.state.OnlyConfiguredNetworks = onlyConfigured
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
