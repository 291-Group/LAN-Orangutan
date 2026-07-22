package scanner

import (
	"bufio"
	"bytes"
	"compress/gzip"
	_ "embed"
	"strings"
	"sync"
)

// ouiData is the IEEE MAC address registry, compressed, as
// "PREFIX<tab>Organization" lines.
//
// It is embedded rather than fetched so vendor lookups work with no network
// access, which matters for a tool often run on an isolated network.
// Regenerate it with `make oui`.
//
//go:embed oui.txt.gz
var ouiData []byte

var (
	ouiOnce    sync.Once
	ouiVendors map[string]string
)

// loadOUI decompresses and indexes the registry on first use.
//
// Parsing roughly forty thousand entries costs a few milliseconds, so it is
// done lazily: commands that never look up a vendor never pay for it.
func loadOUI() {
	ouiVendors = make(map[string]string, 40000)

	zr, err := gzip.NewReader(bytes.NewReader(ouiData))
	if err != nil {
		return
	}
	defer zr.Close()

	scanner := bufio.NewScanner(zr)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		prefix, name, ok := strings.Cut(scanner.Text(), "\t")
		if !ok {
			continue
		}
		ouiVendors[prefix] = name
	}
}

// normaliseMAC reduces a MAC address to bare uppercase hex, accepting the
// aa:bb:cc, AA-BB-CC and aabbcc forms that different tools produce.
func normaliseMAC(mac string) string {
	var sb strings.Builder
	sb.Grow(12)
	for _, r := range mac {
		switch {
		case r >= '0' && r <= '9', r >= 'A' && r <= 'F':
			sb.WriteRune(r)
		case r >= 'a' && r <= 'f':
			sb.WriteRune(r - 'a' + 'A')
		}
	}
	return sb.String()
}

// GetMACVendor returns the manufacturer registered to a MAC address.
//
// It reports "Unknown" when the address is empty, malformed, or in a range that
// is not publicly registered.
func GetMACVendor(mac string) string {
	normalised := normaliseMAC(mac)
	if len(normalised) < 6 {
		return "Unknown"
	}

	ouiOnce.Do(loadOUI)

	if vendor, ok := ouiVendors[normalised[:6]]; ok {
		return vendor
	}
	return "Unknown"
}

// IsLocallyAdministered reports whether a MAC address was generated rather than
// assigned to a manufacturer.
//
// Phones and laptops randomise their address to avoid being tracked between
// networks. Such an address has no manufacturer to look up, which is worth
// distinguishing from a vendor we simply do not recognise.
func IsLocallyAdministered(mac string) bool {
	normalised := normaliseMAC(mac)
	if len(normalised) < 2 {
		return false
	}

	firstOctet := hexByte(normalised[0], normalised[1])

	// Bit 1 of the first octet is the locally-administered flag.
	return firstOctet&0x02 != 0
}

// hexByte combines two hex digits into a byte. Callers must pass characters
// that normaliseMAC has already validated.
func hexByte(hi, lo byte) byte {
	return hexNibble(hi)<<4 | hexNibble(lo)
}

func hexNibble(c byte) byte {
	if c >= '0' && c <= '9' {
		return c - '0'
	}
	return c - 'A' + 10
}

// ResolveVendor returns the manufacturer to display for a device.
//
// The vendor is normally recorded when a device is discovered, but records
// created by an older version predate the full registry and hold an empty or
// "Unknown" value. Falling back to a live lookup means an upgrade shows real
// manufacturer names immediately, rather than only after the next scan.
func ResolveVendor(stored, mac string) string {
	if stored != "" && stored != "Unknown" {
		return stored
	}
	if mac == "" {
		return stored
	}
	if vendor := GetMACVendor(mac); vendor != "Unknown" {
		return vendor
	}
	return stored
}
