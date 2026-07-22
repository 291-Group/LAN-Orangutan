package network

import "testing"

func TestParseNetworkList(t *testing.T) {
	tests := []struct {
		in   string
		want int
	}{
		{"192.168.1.0/24", 1},
		{"192.168.1.0/24,10.0.0.0/24", 2},
		{"192.168.1.0/24, 10.0.0.0/24", 2},
		{"192.168.1.0/24 10.0.0.0/24", 2},
		{"  192.168.1.0/24  ,  ", 1},
		{"", 0},
		{"   ", 0},
	}
	for _, tt := range tests {
		if got := ParseNetworkList(tt.in); len(got) != tt.want {
			t.Errorf("ParseNetworkList(%q) returned %d entries, want %d", tt.in, len(got), tt.want)
		}
	}
}

func TestConfiguredNetworksAreAdded(t *testing.T) {
	got := WithConfigured(nil, []string{"192.168.50.0/24"})
	if len(got) != 1 {
		t.Fatalf("expected the configured network to be added, got %d", len(got))
	}
	if got[0].CIDR != "192.168.50.0/24" {
		t.Errorf("CIDR = %q", got[0].CIDR)
	}
	if got[0].Interface != "configured" {
		t.Errorf("configured networks should be labelled as such, got %q", got[0].Interface)
	}
}

func TestDuplicatesAreNotAddedTwice(t *testing.T) {
	detected := WithConfigured(nil, []string{"192.168.50.0/24"})
	again := WithConfigured(detected, []string{"192.168.50.0/24"})

	if len(again) != 1 {
		t.Errorf("a network already present should not be added again, got %d", len(again))
	}
}

func TestInvalidCIDRsAreIgnored(t *testing.T) {
	got := WithConfigured(nil, []string{"not-a-network", "192.168.1.999/24", "192.168.50.0/24"})
	if len(got) != 1 {
		t.Fatalf("only the valid network should survive, got %d", len(got))
	}
	if got[0].CIDR != "192.168.50.0/24" {
		t.Errorf("kept the wrong network: %q", got[0].CIDR)
	}
}
