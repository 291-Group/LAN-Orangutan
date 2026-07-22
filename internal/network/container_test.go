package network

import (
	"testing"

	"github.com/291-Group/LAN-Orangutan/internal/types"
)

func TestDockerPrivateRangeDetection(t *testing.T) {
	tests := []struct {
		cidr    string
		private bool
	}{
		{"172.17.0.0/16", true},    // default docker bridge
		{"172.18.0.0/16", true},    // user-defined bridge
		{"192.168.65.0/24", true},  // Docker Desktop VM
		{"10.88.0.0/16", true},     // podman
		{"192.168.1.0/24", false},  // a real home LAN
		{"192.168.68.0/22", false}, // a real home LAN
		{"10.0.0.0/24", false},     // a real LAN
		{"not-a-cidr", false},
	}
	for _, tt := range tests {
		if got := isDockerPrivate(tt.cidr); got != tt.private {
			t.Errorf("isDockerPrivate(%q) = %v, want %v", tt.cidr, got, tt.private)
		}
	}
}

func TestIsolationWarningOnlyWhenIsolated(t *testing.T) {
	// Outside a container there is nothing to warn about, whatever the
	// networks look like.
	if w := IsolationWarning([]types.Network{{CIDR: "172.17.0.0/16"}}); !InContainer() && w != "" {
		t.Error("no warning should be produced when not running in a container")
	}
}

func TestRealNetworkMeansNotIsolated(t *testing.T) {
	// A container that can see a real LAN, as with host networking, is fine.
	networks := []types.Network{
		{CIDR: "172.17.0.0/16"},
		{CIDR: "192.168.1.0/24"},
	}
	if IsolatedFromLAN(networks) {
		t.Error("a visible real network means the container is not isolated")
	}
}

func TestExcludeContainerNetworks(t *testing.T) {
	in := []types.Network{
		{CIDR: "192.168.1.0/24"},  // real LAN
		{CIDR: "172.17.0.0/16"},   // docker0 bridge
		{CIDR: "192.168.68.0/22"}, // real LAN
		{CIDR: "172.18.0.0/16"},   // user-defined docker bridge
		{CIDR: "192.168.65.0/24"}, // Docker Desktop VM
	}
	got := ExcludeContainerNetworks(in)

	if len(got) != 2 {
		t.Fatalf("expected only the two real networks, got %d: %v", len(got), got)
	}
	for _, n := range got {
		if n.CIDR != "192.168.1.0/24" && n.CIDR != "192.168.68.0/22" {
			t.Errorf("unexpected network kept: %s", n.CIDR)
		}
	}
}

func TestConfiguredContainerNetworkIsStillHonoured(t *testing.T) {
	// Filtering applies to detection, not to a network the user asked for.
	got := WithConfigured(nil, []string{"172.17.0.0/16"})
	if len(got) != 1 || got[0].CIDR != "172.17.0.0/16" {
		t.Errorf("an explicitly configured network should survive, got %v", got)
	}
}
