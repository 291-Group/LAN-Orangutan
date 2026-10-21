package scanner

import (
	"testing"

	"github.com/291-Group/LAN-Orangutan/internal/types"
)

func TestParseProcARP(t *testing.T) {
	out := `IP address       HW type     Flags       HW address            Mask     Device
192.168.1.5      0x1         0x2         aa:bb:cc:dd:ee:ff     *        eth0
192.168.1.6      0x1         0x0         00:00:00:00:00:00     *        eth0
192.168.1.7      0x1         0x2         00:00:00:00:00:00     *        eth0
not-an-ip        0x1         0x2         aa:bb:cc:dd:ee:ff     *        eth0`

	got := parseProcARP(out)
	if len(got) != 1 {
		t.Fatalf("expected only the complete valid neighbor, got %+v", got)
	}
	if got[0].IP != "192.168.1.5" || got[0].MAC != "AA:BB:CC:DD:EE:FF" {
		t.Errorf("unexpected neighbor: %+v", got[0])
	}
}

func TestParseARPOutput(t *testing.T) {
	out := `? (192.168.1.5) at aa:bb:cc:dd:ee:ff on en0 ifscope [ethernet]
? (192.168.1.6) at (incomplete) on en0 ifscope [ethernet]
Interface: 10.0.0.1 --- 0x13
  10.0.0.7          00-11-22-33-44-55     dynamic
  10.0.0.8          invalid               dynamic`

	got := parseARPOutput(out)
	if len(got) != 2 {
		t.Fatalf("expected the complete macOS and Windows neighbors, got %+v", got)
	}
	want := map[string]string{
		"192.168.1.5": "AA:BB:CC:DD:EE:FF",
		"10.0.0.7":    "00:11:22:33:44:55",
	}
	for _, device := range got {
		if mac, ok := want[device.IP]; !ok || mac != device.MAC {
			t.Errorf("unexpected neighbor: %+v", device)
		}
	}
}

func TestFilterIPv4NeighborsToScannedNetworks(t *testing.T) {
	devices := []types.Device{
		{IP: "192.168.1.5", MAC: "AA:BB:CC:DD:EE:FF"},
		{IP: "192.168.1.5", MAC: "AA:BB:CC:DD:EE:FF"},
		{IP: "192.168.2.5", MAC: "00:11:22:33:44:55"},
		{IP: "2001:db8::5", MAC: "00:11:22:33:44:66"},
	}

	got := filterIPv4Neighbors(devices, []string{"192.168.1.0/24", "not-a-cidr"})
	if len(got) != 1 || got[0].IP != "192.168.1.5" {
		t.Fatalf("expected one neighbor inside the scanned IPv4 network, got %+v", got)
	}
	if got := filterIPv4Neighbors(devices, nil); len(got) != 0 {
		t.Errorf("no selected networks must yield no neighbors, got %+v", got)
	}
}
