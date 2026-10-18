package storage

import (
	"fmt"
	"sort"
	"strings"
	"unicode"
)

// defaultGroups is the built-in group list, in the order it is offered. It is
// only a starting point: the user can add, rename and remove entries, at which
// point the whole list lives in the scan state instead.
var defaultGroups = []string{"Server", "Desktop", "Laptop", "Mobile", "IoT", "Network", "Pi"}

// maxGroupNameLen bounds a group name so the dropdown and the table column stay
// usable. Generous enough for anything descriptive, short enough that one entry
// cannot distort the UI.
const maxGroupNameLen = 32

// DefaultGroups returns a copy of the built-in group list.
func DefaultGroups() []string {
	return append([]string(nil), defaultGroups...)
}

// GetGroups returns the effective group list: the user's own if they have
// customised it, otherwise the built-in defaults.
//
// Any group actually set on a device is appended if it is missing from the list.
// Without that, a group that arrived with a restored backup, or that was removed
// from the list by hand, would leave devices carrying a value the UI cannot show
// or select, and a later edit would silently drop it.
func (s *Storage) GetGroups() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.effectiveGroupsLocked()
}

// GroupsCustomised reports whether the user has replaced the built-in list, so
// the UI can offer "reset to defaults" only when it would do something.
func (s *Storage) GroupsCustomised() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.state.Groups != nil
}

// GroupCounts returns how many devices carry each group, so the UI can warn
// before a delete or a rename that affects real devices.
func (s *Storage) GroupCounts() map[string]int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	counts := make(map[string]int)
	for _, d := range s.devices {
		if d.Group != "" {
			counts[d.Group]++
		}
	}
	return counts
}

// effectiveGroupsLocked builds the list the UI should show. Callers hold s.mu.
func (s *Storage) effectiveGroupsLocked() []string {
	var groups []string
	if s.state.Groups != nil {
		groups = append(groups, *s.state.Groups...)
	} else {
		groups = append(groups, defaultGroups...)
	}

	seen := make(map[string]bool, len(groups))
	for _, g := range groups {
		seen[strings.ToLower(g)] = true
	}
	// Pick up any group in use that the list does not mention, so no device is
	// left holding a value the UI cannot represent.
	var extra []string
	for _, d := range s.devices {
		if d.Group == "" || seen[strings.ToLower(d.Group)] {
			continue
		}
		seen[strings.ToLower(d.Group)] = true
		extra = append(extra, d.Group)
	}
	sort.Strings(extra)
	return append(groups, extra...)
}

// SetGroups replaces the whole group list. An empty slice is a valid list (the
// user removed every group); nil resets to the built-in defaults.
func (s *Storage) SetGroups(groups []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if groups == nil {
		s.state.Groups = nil
		return s.saveState()
	}

	cleaned, err := validateGroupNames(groups)
	if err != nil {
		return err
	}
	s.state.Groups = &cleaned
	return s.saveState()
}

// AddGroup appends a new group to the list.
func (s *Storage) AddGroup(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	clean, err := validateGroupName(name)
	if err != nil {
		return err
	}
	groups := s.effectiveGroupsLocked()
	for _, g := range groups {
		if strings.EqualFold(g, clean) {
			return fmt.Errorf("group %q already exists", clean)
		}
	}
	groups = append(groups, clean)
	s.state.Groups = &groups
	return s.saveState()
}

// RenameGroup renames a group and rewrites it on every device that used it, so
// the list and the devices can never drift apart.
//
// Renaming to a name that differs only in case is allowed (it is the same group
// being re-spelled); renaming onto a different existing group is not, because
// that is a merge, and silently merging two groups would lose the distinction
// without the user asking for it.
func (s *Storage) RenameGroup(oldName, newName string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	clean, err := validateGroupName(newName)
	if err != nil {
		return err
	}
	oldName = strings.TrimSpace(oldName)

	groups := s.effectiveGroupsLocked()
	idx := -1
	for i, g := range groups {
		if strings.EqualFold(g, oldName) {
			idx = i
			break
		}
	}
	if idx < 0 {
		return fmt.Errorf("group %q not found", oldName)
	}
	for i, g := range groups {
		if i != idx && strings.EqualFold(g, clean) {
			return fmt.Errorf("group %q already exists", clean)
		}
	}

	from := groups[idx]
	groups[idx] = clean
	s.state.Groups = &groups
	if err := s.saveState(); err != nil {
		return err
	}

	changed := false
	for _, d := range s.devices {
		if strings.EqualFold(d.Group, from) {
			d.Group = clean
			changed = true
		}
	}
	if changed {
		return s.saveDevices()
	}
	return nil
}

// DeleteGroup removes a group from the list and clears it from every device that
// carried it, leaving those devices ungrouped rather than pointing at a group
// that no longer exists.
func (s *Storage) DeleteGroup(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	name = strings.TrimSpace(name)
	groups := s.effectiveGroupsLocked()
	kept := make([]string, 0, len(groups))
	found := false
	for _, g := range groups {
		if strings.EqualFold(g, name) {
			found = true
			continue
		}
		kept = append(kept, g)
	}
	if !found {
		return fmt.Errorf("group %q not found", name)
	}

	s.state.Groups = &kept
	if err := s.saveState(); err != nil {
		return err
	}

	changed := false
	for _, d := range s.devices {
		if strings.EqualFold(d.Group, name) {
			d.Group = ""
			changed = true
		}
	}
	if changed {
		return s.saveDevices()
	}
	return nil
}

// validateGroupName trims and checks a single name.
func validateGroupName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", fmt.Errorf("group name cannot be empty")
	}
	if len([]rune(name)) > maxGroupNameLen {
		return "", fmt.Errorf("group name cannot be longer than %d characters", maxGroupNameLen)
	}
	// Control characters would break the dropdown and the table without being
	// visible to whoever typed them.
	for _, r := range name {
		if unicode.IsControl(r) {
			return "", fmt.Errorf("group name cannot contain control characters")
		}
	}
	return name, nil
}

// validateGroupNames cleans a whole list and rejects duplicates.
func validateGroupNames(names []string) ([]string, error) {
	cleaned := make([]string, 0, len(names))
	seen := make(map[string]bool, len(names))
	for _, n := range names {
		clean, err := validateGroupName(n)
		if err != nil {
			return nil, err
		}
		if seen[strings.ToLower(clean)] {
			return nil, fmt.Errorf("duplicate group %q", clean)
		}
		seen[strings.ToLower(clean)] = true
		cleaned = append(cleaned, clean)
	}
	return cleaned, nil
}
