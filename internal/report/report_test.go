package report

import (
	"strings"
	"testing"
	"time"

	"github.com/291-Group/LAN-Orangutan/internal/types"
)

func TestRenderContainsDevicesAndStats(t *testing.T) {
	var b strings.Builder
	err := Render(&b, Data{
		GeneratedAt:  time.Date(2026, 1, 2, 15, 4, 0, 0, time.UTC),
		Total:        2,
		Online:       1,
		Offline:      1,
		NetworkCount: 1,
		Devices: []types.Device{
			{IP: "192.168.1.5", MAC: "aa:bb:cc:00:00:01", Hostname: "nas", Vendor: "Synology", LastSeen: time.Now()},
			{IP: "192.168.1.6", LastSeen: time.Now()},
		},
	})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	out := b.String()
	for _, want := range []string{"<!doctype html>", "Network Report", "192.168.1.5", "nas", "Synology", "<b>2</b>", "</body>", "</html>"} {
		if !strings.Contains(out, want) {
			t.Errorf("report missing %q", want)
		}
	}
	// A value with markup must be escaped, not injected.
	var b2 strings.Builder
	_ = Render(&b2, Data{Devices: []types.Device{{IP: "10.0.0.1", Hostname: "<script>x</script>", LastSeen: time.Now()}}})
	if strings.Contains(b2.String(), "<script>x</script>") {
		t.Error("hostname was not HTML-escaped in the report")
	}
}
