// Package snapshot compares two device lists to report what changed on the
// network between them.
package snapshot

import (
	"sort"
	"strings"

	"github.com/291-Group/LAN-Orangutan/internal/types"
)

// Diff is the result of comparing an earlier device list with a later one.
type Diff struct {
	Added   []types.Device `json:"added"`
	Removed []types.Device `json:"removed"`
	Changed []DeviceChange `json:"changed"`
}

// DeviceChange is a device present in both lists whose details differ, with
// the names of the fields that changed. Field names are the JSON names of the
// device fields: ip, hostname, vendor, type, custom_hostname, label, group,
// location and expected.
type DeviceChange struct {
	Before types.Device `json:"before"`
	After  types.Device `json:"after"`
	Fields []string     `json:"fields"`
}

// key identifies a device across two lists. MAC is preferred because it is
// stable when a device only changes IP (so that reads as a change, not a
// removal plus an addition); IP is the fallback for a device with no MAC.
func key(d types.Device) string {
	if d.MAC != "" {
		return "mac:" + strings.ToLower(d.MAC)
	}
	return "ip:" + d.IP
}

// changedFields lists the fields that differ between two matched devices:
// what the scan saw (address, name, vendor, type) and what the user set
// (custom hostname, label, group, location, expected), so a comparison shows
// both kinds of change. LastSeen always moves and notes are free text, so
// neither is compared.
func changedFields(before, after types.Device) []string {
	var f []string
	add := func(name string, b, a string) {
		if b != a {
			f = append(f, name)
		}
	}
	add("ip", before.IP, after.IP)
	add("hostname", before.Hostname, after.Hostname)
	add("vendor", before.Vendor, after.Vendor)
	add("type", before.Type, after.Type)
	add("custom_hostname", before.CustomHostname, after.CustomHostname)
	add("label", before.Label, after.Label)
	add("group", before.Group, after.Group)
	add("location", before.Location, after.Location)
	if before.Expected != after.Expected {
		f = append(f, "expected")
	}
	return f
}

// FieldValue returns the value of a changed field on d, as a string, so the
// CLI and the web page can print "label: old -> new" without each keeping its
// own switch over the field names.
func FieldValue(d types.Device, field string) string {
	switch field {
	case "ip":
		return d.IP
	case "hostname":
		return d.Hostname
	case "vendor":
		return d.Vendor
	case "type":
		return d.Type
	case "custom_hostname":
		return d.CustomHostname
	case "label":
		return d.Label
	case "group":
		return d.Group
	case "location":
		return d.Location
	case "expected":
		if d.Expected {
			return "yes"
		}
		return "no"
	}
	return ""
}

// Compare reports what appeared, disappeared, and changed going from the earlier
// list to the later one. The result slices are never nil and are sorted by IP so
// the output is stable.
func Compare(before, after []types.Device) Diff {
	beforeByKey := make(map[string]types.Device, len(before))
	for _, d := range before {
		beforeByKey[key(d)] = d
	}
	afterByKey := make(map[string]types.Device, len(after))
	for _, d := range after {
		afterByKey[key(d)] = d
	}

	diff := Diff{Added: []types.Device{}, Removed: []types.Device{}, Changed: []DeviceChange{}}

	for k, a := range afterByKey {
		b, ok := beforeByKey[k]
		if !ok {
			diff.Added = append(diff.Added, a)
			continue
		}
		if fields := changedFields(b, a); len(fields) > 0 {
			diff.Changed = append(diff.Changed, DeviceChange{Before: b, After: a, Fields: fields})
		}
	}
	for k, b := range beforeByKey {
		if _, ok := afterByKey[k]; !ok {
			diff.Removed = append(diff.Removed, b)
		}
	}

	sort.Slice(diff.Added, func(i, j int) bool { return diff.Added[i].IP < diff.Added[j].IP })
	sort.Slice(diff.Removed, func(i, j int) bool { return diff.Removed[i].IP < diff.Removed[j].IP })
	sort.Slice(diff.Changed, func(i, j int) bool { return diff.Changed[i].After.IP < diff.Changed[j].After.IP })

	return diff
}
