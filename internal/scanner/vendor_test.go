package scanner

import (
	"strings"
	"testing"
)

func TestKnownVendorsResolve(t *testing.T) {
	// Well-known assignments that should be present in any current copy of the
	// IEEE registry. These are checked by substring because organisations
	// change their registered names over time.
	tests := []struct {
		mac      string
		contains string
	}{
		{"B8:27:EB:12:34:56", "Raspberry Pi"},
		{"DC:A6:32:00:00:01", "Raspberry Pi"},
		{"00:50:56:AA:BB:CC", "VMware"},
		{"08:00:27:11:22:33", "PCS Systemtechnik"}, // VirtualBox
		{"00:1B:63:84:45:E6", "Apple"},
		{"3C:D9:2B:00:00:01", "Hewlett Packard"},
		{"00:18:0A:00:00:01", "Cisco"},
	}

	for _, tt := range tests {
		got := GetMACVendor(tt.mac)
		if got == "Unknown" {
			t.Errorf("GetMACVendor(%s) = Unknown, expected something containing %q", tt.mac, tt.contains)
			continue
		}
		if !strings.Contains(strings.ToLower(got), strings.ToLower(tt.contains)) {
			t.Errorf("GetMACVendor(%s) = %q, expected it to mention %q", tt.mac, got, tt.contains)
		}
	}
}

func TestVendorLookupAcceptsAllFormats(t *testing.T) {
	// nmap, arp and Windows all format MAC addresses differently.
	forms := []string{
		"B8:27:EB:12:34:56",
		"b8:27:eb:12:34:56",
		"B8-27-EB-12-34-56",
		"b827eb123456",
		"B827EB123456",
	}

	want := GetMACVendor(forms[0])
	if want == "Unknown" {
		t.Fatal("expected the reference address to resolve")
	}

	for _, form := range forms {
		if got := GetMACVendor(form); got != want {
			t.Errorf("GetMACVendor(%q) = %q, want %q", form, got, want)
		}
	}
}

func TestUnknownAndMalformedAddresses(t *testing.T) {
	tests := []string{
		"",
		"xx",
		"not-a-mac",
		"12:34",             // too short to carry a prefix
		"FF:FF:FF:FF:FF:FF", // broadcast, not a registered assignment
	}

	for _, mac := range tests {
		if got := GetMACVendor(mac); got != "Unknown" {
			t.Errorf("GetMACVendor(%q) = %q, want Unknown", mac, got)
		}
	}
}

func TestDatabaseIsSubstantial(t *testing.T) {
	// Guards against the embedded file being empty or truncated, which would
	// otherwise show up only as every device reading "Unknown".
	ouiOnce.Do(loadOUI)

	if len(ouiVendors) < 30000 {
		t.Fatalf("only %d vendors loaded; the embedded registry looks incomplete", len(ouiVendors))
	}
}

func TestLocallyAdministeredDetection(t *testing.T) {
	tests := []struct {
		mac  string
		want bool
	}{
		{"B8:27:EB:12:34:56", false}, // Raspberry Pi, globally assigned
		{"00:50:56:AA:BB:CC", false}, // VMware, globally assigned
		{"02:00:00:00:00:01", true},  // bit 1 set
		{"06:11:22:33:44:55", true},
		{"0A:11:22:33:44:55", true},
		{"0E:11:22:33:44:55", true},
		{"", false},
	}

	for _, tt := range tests {
		if got := IsLocallyAdministered(tt.mac); got != tt.want {
			t.Errorf("IsLocallyAdministered(%q) = %v, want %v", tt.mac, got, tt.want)
		}
	}
}

func TestResolveVendorFillsInMissingValues(t *testing.T) {
	tests := []struct {
		name        string
		stored      string
		mac         string
		wantLookup  bool
		wantLiteral string
	}{
		{name: "record from an older version has no vendor", stored: "", mac: "B8:27:EB:11:22:33", wantLookup: true},
		{name: "older version stored Unknown", stored: "Unknown", mac: "B8:27:EB:11:22:33", wantLookup: true},
		{name: "an existing value is trusted", stored: "Custom Name", mac: "B8:27:EB:11:22:33", wantLiteral: "Custom Name"},
		{name: "no MAC to look up, keep what we have", stored: "", mac: "", wantLiteral: ""},
		{name: "unregistered MAC keeps the stored value", stored: "Unknown", mac: "02:00:00:00:00:01", wantLiteral: "Unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ResolveVendor(tt.stored, tt.mac)
			if tt.wantLookup {
				if got == "" || got == "Unknown" {
					t.Errorf("ResolveVendor(%q, %q) = %q, expected a looked-up manufacturer", tt.stored, tt.mac, got)
				}
				return
			}
			if got != tt.wantLiteral {
				t.Errorf("ResolveVendor(%q, %q) = %q, want %q", tt.stored, tt.mac, got, tt.wantLiteral)
			}
		})
	}
}
