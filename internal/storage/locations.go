package storage

import (
	"sort"
	"strings"
	"unicode"

	"github.com/291-Group/LAN-Orangutan/internal/uerr"
)

// maxLocationNameLen matches the group limit, for the same reason: the column
// and the dropdowns stay usable.
const maxLocationNameLen = 32

// GetLocations returns the location list: the user's own entries, plus any
// location set on a device that the list does not mention (from a restored
// backup, or set from the CLI), so no device holds a value the UI cannot show.
func (s *Storage) GetLocations() []string {
	defer s.readLock()()
	return s.effectiveLocationsLocked()
}

// LocationCounts returns how many devices are in each location, so the UI can
// warn before a delete or rename that affects real devices.
func (s *Storage) LocationCounts() map[string]int {
	defer s.readLock()()
	counts := make(map[string]int)
	for _, d := range s.devices {
		if d.Location != "" {
			counts[d.Location]++
		}
	}
	return counts
}

func (s *Storage) effectiveLocationsLocked() []string {
	locs := append([]string(nil), s.state.Locations...)
	seen := make(map[string]bool, len(locs))
	for _, l := range locs {
		seen[strings.ToLower(l)] = true
	}
	var extra []string
	for _, d := range s.devices {
		if d.Location == "" || seen[strings.ToLower(d.Location)] {
			continue
		}
		seen[strings.ToLower(d.Location)] = true
		extra = append(extra, d.Location)
	}
	sort.Strings(extra)
	return append(locs, extra...)
}

// AddLocation appends a location to the list.
func (s *Storage) AddLocation(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.syncLocked()

	clean, err := validateLocationName(name)
	if err != nil {
		return err
	}
	locs := s.effectiveLocationsLocked()
	for _, l := range locs {
		if strings.EqualFold(l, clean) {
			return uerr.Newf("location.exists", "location %q already exists", "name", clean)
		}
	}
	s.state.Locations = append(locs, clean)
	return s.saveState()
}

// RenameLocation renames a location and moves every device in it, the same
// way a group rename works. Renaming onto another existing location is refused,
// since that would merge two places without the user asking for it.
func (s *Storage) RenameLocation(oldName, newName string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.syncLocked()

	clean, err := validateLocationName(newName)
	if err != nil {
		return err
	}
	oldName = strings.TrimSpace(oldName)
	locs := s.effectiveLocationsLocked()
	idx := -1
	for i, l := range locs {
		if strings.EqualFold(l, oldName) {
			idx = i
			break
		}
	}
	if idx < 0 {
		return uerr.Newf("location.notFound", "location %q not found", "name", oldName)
	}
	for i, l := range locs {
		if i != idx && strings.EqualFold(l, clean) {
			return uerr.Newf("location.exists", "location %q already exists", "name", clean)
		}
	}

	from := locs[idx]
	locs[idx] = clean
	s.state.Locations = locs
	if err := s.saveState(); err != nil {
		return err
	}
	changed := false
	for _, d := range s.devices {
		if strings.EqualFold(d.Location, from) {
			d.Location = clean
			changed = true
		}
	}
	if changed {
		return s.saveDevices()
	}
	return nil
}

// DeleteLocation removes a location and clears it from its devices. The
// devices themselves are kept.
func (s *Storage) DeleteLocation(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.syncLocked()

	name = strings.TrimSpace(name)
	locs := s.effectiveLocationsLocked()
	kept := make([]string, 0, len(locs))
	found := false
	for _, l := range locs {
		if strings.EqualFold(l, name) {
			found = true
			continue
		}
		kept = append(kept, l)
	}
	if !found {
		return uerr.Newf("location.notFound", "location %q not found", "name", name)
	}
	s.state.Locations = kept
	if err := s.saveState(); err != nil {
		return err
	}
	changed := false
	for _, d := range s.devices {
		if strings.EqualFold(d.Location, name) {
			d.Location = ""
			changed = true
		}
	}
	if changed {
		return s.saveDevices()
	}
	return nil
}

// validateLocationName trims and checks a location name. An empty name is an
// error here; clearing a device's location is done by setting it to "".
func validateLocationName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", uerr.New("location.empty", "location name cannot be empty")
	}
	if len([]rune(name)) > maxLocationNameLen {
		return "", uerr.Newf("location.tooLong", "location name cannot be longer than %d characters", "max", maxLocationNameLen)
	}
	for _, r := range name {
		if unicode.IsControl(r) {
			return "", uerr.New("location.controlChars", "location name cannot contain control characters")
		}
	}
	return name, nil
}
