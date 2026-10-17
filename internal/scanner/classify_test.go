package scanner

import "testing"

func TestClassify(t *testing.T) {
	cases := []struct {
		name     string
		vendor   string
		hostname string
		ports    []int
		want     string
	}{
		// Hostname wins: specific and reliable.
		{"iphone by hostname", "Apple, Inc.", "Sherifs-iPhone", nil, TypePhone},
		{"macbook by hostname", "Apple, Inc.", "MacBook-Pro", nil, TypeComputer},
		{"apple tv by hostname beats TV", "Apple, Inc.", "AppleTV-Living-Room", nil, TypeMediaPlayer},
		{"printer by hostname", "", "HPB4A2-LaserJet", nil, TypePrinter},
		{"windows pc by hostname", "Intel Corporate", "DESKTOP-8F3K1", nil, TypeComputer},
		{"roku by hostname", "", "roku-ultra", nil, TypeMediaPlayer},
		{"smart tv by hostname", "Samsung Electronics", "Samsung-TV-Bedroom", nil, TypeTV},
		{"unifi ap by hostname", "", "UniFi-AP-Office", nil, TypeRouter},
		{"raspberry pi by hostname", "", "raspberrypi", nil, TypeServer},
		{"esp iot by hostname", "", "esp32-sensor", nil, TypeIoT},
		{"xbox by hostname", "", "XBOX-SYSTEMOS", nil, TypeConsole},

		// Definitive ports (printer, iPhone) classify and outrank vendor.
		{"printer by port 9100", "", "", []int{9100}, TypePrinter},
		{"iphone by lockdown port", "", "", []int{62078}, TypePhone},
		{"definitive printer port beats apple vendor", "Apple, Inc.", "", []int{9100}, TypePrinter},

		// Indicative ports (media, NAS) only classify when nothing else does.
		{"plex classifies when vendor unknown", "", "", []int{80, 32400}, TypeMediaPlayer},
		{"synology port when vendor unknown", "", "", []int{5000}, TypeServer},
		{"apple running plex stays a computer", "Apple, Inc.", "", []int{32400}, TypeComputer},
		{"generic web port does not classify", "", "", []int{80, 443}, TypeUnknown},

		// GL.iNet router, found live, by hostname and by vendor.
		{"gl-inet router by hostname", "GL Technologies (Hong Kong) Limited", "console.gl-inet.com", nil, TypeRouter},
		{"gl technologies router by vendor", "GL Technologies (Hong Kong) Limited", "", nil, TypeRouter},

		// Vendor is the broad fallback.
		{"canon printer by vendor", "Canon Inc.", "", nil, TypePrinter},
		{"ubiquiti router by vendor", "Ubiquiti Networks Inc.", "", nil, TypeRouter},
		{"synology server by vendor", "Synology Incorporated", "", nil, TypeServer},
		{"espressif iot by vendor", "Espressif Inc.", "", nil, TypeIoT},
		{"nintendo console by vendor", "Nintendo Co.,Ltd", "", nil, TypeConsole},
		{"dell computer by vendor", "Dell Inc.", "", nil, TypeComputer},
		{"apple defaults to computer without hostname", "Apple, Inc.", "", nil, TypeComputer},

		// Priority: hostname beats a conflicting vendor guess.
		{"printer hostname beats intel vendor", "Intel Corporate", "office-laserjet", nil, TypePrinter},

		// Nothing to go on.
		{"all empty is unknown", "", "", nil, TypeUnknown},
		{"unrecognized vendor is unknown", "Some Random OEM Ltd", "box-1", nil, TypeUnknown},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := Classify(c.vendor, c.hostname, c.ports)
			if got != c.want {
				t.Errorf("Classify(%q, %q, %v) = %q, want %q", c.vendor, c.hostname, c.ports, got, c.want)
			}
		})
	}
}

func TestKnownTypes(t *testing.T) {
	if len(KnownTypes) != 9 {
		t.Fatalf("expected 9 standard types, got %d: %v", len(KnownTypes), KnownTypes)
	}
	expected := []string{
		TypePhone, TypeComputer, TypePrinter, TypeTV,
		TypeMediaPlayer, TypeRouter, TypeServer, TypeIoT, TypeConsole,
	}
	for i, exp := range expected {
		if KnownTypes[i] != exp {
			t.Errorf("expected KnownTypes[%d] == %q, got %q", i, exp, KnownTypes[i])
		}
	}
}

func TestIsKnownType(t *testing.T) {
	for _, kt := range KnownTypes {
		if !IsKnownType(kt) {
			t.Errorf("expected IsKnownType(%q) to be true", kt)
		}
	}
	for _, unknown := range []string{"Smart Plug", "Thermostat", "Unknown", "", "Custom"} {
		if IsKnownType(unknown) {
			t.Errorf("expected IsKnownType(%q) to be false", unknown)
		}
	}
}

func TestValidateType(t *testing.T) {
	// Empty string is valid (auto / reset)
	got, err := ValidateType("")
	if err != nil || got != "" {
		t.Fatalf("ValidateType(\"\") = (%q, %v), want (\"\", nil)", got, err)
	}

	// Whitespace only is treated as empty
	got, err = ValidateType("   ")
	if err != nil || got != "" {
		t.Fatalf("ValidateType(\"   \") = (%q, %v), want (\"\", nil)", got, err)
	}

	// Canonical types match and return normalized
	for _, kt := range KnownTypes {
		got, err := ValidateType(kt)
		if err != nil || got != kt {
			t.Errorf("ValidateType(%q) = (%q, %v), want (%q, nil)", kt, got, err, kt)
		}
	}

	// Case-insensitive and trimmed matches
	cases := []struct {
		input string
		want  string
	}{
		{"phone", "Phone"},
		{"COMPUTER", "Computer"},
		{"  server  ", "Server"},
		{"media player", "Media Player"},
		{"game console", "Game Console"},
		{"IoT", "IoT"},
		{"iot", "IoT"},
	}
	for _, c := range cases {
		got, err := ValidateType(c.input)
		if err != nil || got != c.want {
			t.Errorf("ValidateType(%q) = (%q, %v), want (%q, nil)", c.input, got, err, c.want)
		}
	}

	// Invalid / unknown types rejected
	invalids := []string{
		"Smart Plug",
		"Outlet",
		"Thermostat",
		"Random",
		"FooBar",
	}
	for _, inv := range invalids {
		if _, err := ValidateType(inv); err == nil {
			t.Errorf("expected ValidateType(%q) to fail", inv)
		}
	}

	// Length cap exceeded (> 32 characters)
	tooLong := "ThisDeviceTypeStringIsWayTooLongToBeValid"
	if _, err := ValidateType(tooLong); err == nil {
		t.Errorf("expected error for type exceeding maxTypeLen, got nil")
	}

	// Control characters rejected
	control := "Server\x00"
	if _, err := ValidateType(control); err == nil {
		t.Errorf("expected error for control character in type, got nil")
	}
}

