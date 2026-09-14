package snapshot

import (
	"testing"

	"github.com/291-Group/LAN-Orangutan/internal/types"
)

func TestCompareAddedRemovedChanged(t *testing.T) {
	before := []types.Device{
		{IP: "192.168.1.5", MAC: "aa:bb:cc:00:00:01", Hostname: "nas"},
		{IP: "192.168.1.6", MAC: "aa:bb:cc:00:00:02", Hostname: "printer"},
		{IP: "192.168.1.7", MAC: "aa:bb:cc:00:00:03"},
	}
	after := []types.Device{
		{IP: "192.168.1.5", MAC: "aa:bb:cc:00:00:01", Hostname: "nas"},     // unchanged
		{IP: "192.168.1.9", MAC: "aa:bb:cc:00:00:02", Hostname: "printer"}, // moved IP (same MAC)
		// .7 is gone
		{IP: "192.168.1.20", MAC: "aa:bb:cc:00:00:04", Hostname: "laptop"}, // new
	}
	d := Compare(before, after)

	if len(d.Added) != 1 || d.Added[0].IP != "192.168.1.20" {
		t.Errorf("added = %+v, want just the laptop", d.Added)
	}
	if len(d.Removed) != 1 || d.Removed[0].IP != "192.168.1.7" {
		t.Errorf("removed = %+v, want just .7", d.Removed)
	}
	if len(d.Changed) != 1 {
		t.Fatalf("changed = %+v, want 1", d.Changed)
	}
	ch := d.Changed[0]
	if ch.Before.IP != "192.168.1.6" || ch.After.IP != "192.168.1.9" {
		t.Errorf("changed before/after IP wrong: %+v", ch)
	}
	if len(ch.Fields) != 1 || ch.Fields[0] != "ip" {
		t.Errorf("changed fields = %v, want [ip]", ch.Fields)
	}
}

func TestCompareMatchesByMACNotIP(t *testing.T) {
	// Same device, MAC stable, IP moved: exactly one "changed", not an add plus a
	// remove.
	before := []types.Device{{IP: "10.0.0.5", MAC: "aa:bb:cc:dd:ee:ff"}}
	after := []types.Device{{IP: "10.0.0.99", MAC: "aa:bb:cc:dd:ee:ff"}}
	d := Compare(before, after)
	if len(d.Added) != 0 || len(d.Removed) != 0 {
		t.Errorf("a MAC-stable IP move should not add or remove: added %v removed %v", d.Added, d.Removed)
	}
	if len(d.Changed) != 1 || d.Changed[0].Fields[0] != "ip" {
		t.Errorf("want one ip change, got %+v", d.Changed)
	}
}

func TestCompareNoMACMatchesByIP(t *testing.T) {
	before := []types.Device{{IP: "10.0.0.5", Hostname: "a"}}
	after := []types.Device{{IP: "10.0.0.5", Hostname: "b"}}
	d := Compare(before, after)
	if len(d.Changed) != 1 || d.Changed[0].Fields[0] != "hostname" {
		t.Errorf("want one hostname change, got %+v", d.Changed)
	}
}

func TestCompareIgnoresUserFieldsAndLastSeen(t *testing.T) {
	// Label/notes/group and LastSeen are not network changes, so a device that
	// only differs in those is not reported as changed.
	before := []types.Device{{IP: "10.0.0.5", MAC: "aa:bb:cc:dd:ee:01", Label: "old"}}
	after := []types.Device{{IP: "10.0.0.5", MAC: "aa:bb:cc:dd:ee:01", Label: "new", Group: "Server"}}
	d := Compare(before, after)
	if len(d.Changed) != 0 {
		t.Errorf("label/group-only differences should not count as changed, got %+v", d.Changed)
	}
}

func TestCompareEmptyLists(t *testing.T) {
	d := Compare(nil, nil)
	if d.Added == nil || d.Removed == nil || d.Changed == nil {
		t.Error("result slices should be non-nil (empty)")
	}
	if len(d.Added)+len(d.Removed)+len(d.Changed) != 0 {
		t.Error("empty comparison should report no changes")
	}
}
