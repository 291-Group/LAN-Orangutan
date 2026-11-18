// Package notify turns what a scan changed into device events and posts them
// to the user's webhooks.
//
// Events are found by comparing the device list before and after a scan, so
// each one fires once per real transition rather than on every scan that
// happens to see the same state. "Missing" is the exception: a device goes
// missing by time passing, not by anything a scan sees, so it is checked
// against the time of the previous check instead.
package notify

import (
	"net"
	"sort"
	"strings"
	"time"

	"github.com/291-Group/LAN-Orangutan/internal/types"
	"github.com/291-Group/LAN-Orangutan/internal/uerr"
)

// The device events a webhook can subscribe to.
const (
	// EventNew fires the first time a device is ever seen.
	EventNew = "device.new"
	// EventUnexpected fires when a device that is not marked expected comes
	// online: either new, or back after being offline.
	EventUnexpected = "device.unexpected"
	// EventMissing fires when an expected device goes offline, meaning it has
	// not been seen for an hour.
	EventMissing = "device.missing"
	// EventMoved fires when a device, matched by its MAC, shows up at a new IP.
	EventMoved = "device.moved"
	// EventTest is sent only by the Send test button and `webhook test`.
	EventTest = "test"
)

// AllEvents lists the events a webhook can subscribe to, in display order.
var AllEvents = []string{EventNew, EventUnexpected, EventMissing, EventMoved}

// offlineAfter matches types.Device.IsOnline: a device not seen for this long
// counts as offline.
const offlineAfter = time.Hour

// Change is one device event found by comparing two device lists.
type Change struct {
	Event  string
	Device types.Device
	// PreviousIP is the address a moved device was at before. Empty for every
	// other event.
	PreviousIP string
	Time       time.Time
}

// NormalizeEvents accepts event names in full ("device.new") or short ("new")
// form and returns them in full form, de-duplicated and in display order. At
// least one event is required, since a webhook that receives nothing is
// pointless.
func NormalizeEvents(events []string) ([]string, error) {
	want := make(map[string]bool, len(events))
	for _, e := range events {
		typed := strings.TrimSpace(e)
		e = strings.ToLower(typed)
		if e == "" {
			continue
		}
		if !strings.HasPrefix(e, "device.") {
			e = "device." + e
		}
		if !isEvent(e) {
			return nil, uerr.Newf("event.unknown",
				"unknown event %q (choose from new, unexpected, missing, moved, or the full form such as device.new)",
				"event", typed)
		}
		want[e] = true
	}
	out := make([]string, 0, len(want))
	for _, e := range AllEvents {
		if want[e] {
			out = append(out, e)
		}
	}
	if len(out) == 0 {
		return nil, uerr.New("event.none", "choose at least one event")
	}
	return out, nil
}

func isEvent(e string) bool {
	for _, known := range AllEvents {
		if e == known {
			return true
		}
	}
	return false
}

// ShortEvent drops the "device." prefix, for compact display in the CLI.
func ShortEvent(e string) string {
	return strings.TrimPrefix(e, "device.")
}

func onlineAt(d *types.Device, at time.Time) bool {
	return at.Sub(d.LastSeen) < offlineAfter
}

// Detect compares the device list from before a scan (taken at beforeAt) with
// the list after it (taken at afterAt) and returns the new, unexpected and moved
// events. Devices are keyed by IP, as storage keys them; a device that changed
// address is recognised from the address history storage records when it
// matches a MAC at a new IP.
//
// An empty "before" list is treated as a baseline: the very first scan of an
// empty inventory finds everything, and reporting every device on the network
// as new would be noise rather than news.
func Detect(before, after map[string]*types.Device, beforeAt, afterAt time.Time) []Change {
	if len(before) == 0 {
		return nil
	}

	ips := make([]string, 0, len(after))
	for ip := range after {
		ips = append(ips, ip)
	}
	sort.Strings(ips)

	var changes []Change
	for _, ip := range ips {
		cur := after[ip]
		if ephemeral(cur) {
			continue
		}
		prior := before[ip]

		if prior == nil {
			if old := movedFrom(cur, before); old != "" {
				prior = before[old]
				changes = append(changes, Change{Event: EventMoved, Device: *cur, PreviousIP: old, Time: afterAt})
			}
		}
		if prior == nil {
			changes = append(changes, Change{Event: EventNew, Device: *cur, Time: afterAt})
		}
		// Unexpected means "came online", so a device that was already online
		// before the scan does not fire again however many scans see it.
		if !cur.Expected && onlineAt(cur, afterAt) && (prior == nil || !onlineAt(prior, beforeAt)) {
			changes = append(changes, Change{Event: EventUnexpected, Device: *cur, Time: afterAt})
		}
	}
	return changes
}

// ephemeral reports whether a device is an IPv6 privacy address: an IPv6
// address behind a randomised (locally administered) MAC. Such an address
// rotates, so it is not a lasting device, and storage drops these entries the
// next time it loads. Reporting them would announce the same phone as "new"
// every time its address changes. This mirrors the rule storage prunes by.
func ephemeral(d *types.Device) bool {
	ip := net.ParseIP(d.IP)
	if ip == nil || ip.To4() != nil || d.MAC == "" {
		return false
	}
	hw, err := net.ParseMAC(d.MAC)
	return err == nil && len(hw) > 0 && hw[0]&0x02 != 0
}

// movedFrom returns the IP a device moved away from during the scan, or "".
// Storage records a move by appending the old address to the device's history
// and deleting the old entry, so a device at a new IP whose latest history
// entry is an address that held the same MAC before the scan has moved.
func movedFrom(cur *types.Device, before map[string]*types.Device) string {
	n := len(cur.AddressHistory)
	if n == 0 || cur.MAC == "" {
		return ""
	}
	old := cur.AddressHistory[n-1].IP
	if prev := before[old]; prev != nil && strings.EqualFold(prev.MAC, cur.MAC) {
		return old
	}
	return ""
}

// DetectMissing returns a missing event for every expected device that crossed
// the offline threshold in the window (since, now]. Checking a window rather
// than "is it offline now" is what makes the event fire once per
// disappearance: the next check starts where this one ended. A zero since
// means there has never been a check, which is a baseline and reports nothing,
// so turning notifications on does not replay every device already gone.
func DetectMissing(devices map[string]*types.Device, since, now time.Time) []Change {
	if since.IsZero() || !now.After(since) {
		return nil
	}
	ips := make([]string, 0, len(devices))
	for ip := range devices {
		ips = append(ips, ip)
	}
	sort.Strings(ips)

	var changes []Change
	for _, ip := range ips {
		d := devices[ip]
		if !d.Expected || d.LastSeen.IsZero() {
			continue
		}
		crossed := d.LastSeen.Add(offlineAfter)
		if crossed.After(since) && !crossed.After(now) {
			changes = append(changes, Change{Event: EventMissing, Device: *d, Time: now})
		}
	}
	return changes
}

// dedupeKey identifies a change for suppressing repeats: the same event for
// the same device (by MAC when known) and, for a move, the same addresses.
func dedupeKey(c Change) string {
	id := strings.ToLower(c.Device.MAC)
	if id == "" {
		id = c.Device.IP
	}
	return c.Event + "|" + id + "|" + c.PreviousIP + "|" + c.Device.IP
}
