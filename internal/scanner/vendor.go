package scanner

import "strings"

// macVendors maps MAC address prefixes to vendor names
var macVendors = map[string]string{
	"00:50:56": "VMware",
	"00:0C:29": "VMware",
	"08:00:27": "VirtualBox",
	"52:54:00": "QEMU/KVM",
	"B8:27:EB": "Raspberry Pi",
	"DC:A6:32": "Raspberry Pi",
	"E4:5F:01": "Raspberry Pi",
	"28:CD:C1": "Raspberry Pi",
	"D8:3A:DD": "Raspberry Pi",
	"00:E0:4C": "Realtek",
	"00:15:5D": "Microsoft Hyper-V",
	"00:23:AE": "Dell",
	"00:14:22": "Dell",
	"18:A9:9B": "Dell",
	"3C:D9:2B": "HP",
	"00:1E:0B": "HP",
	"00:21:5A": "HP",
	"00:1F:C6": "ASUSTek",
	"00:1A:92": "ASUSTek",
	"14:DA:E9": "ASUSTek",
	"00:1C:C0": "Intel",
	"00:1F:3B": "Intel",
	"3C:A9:F4": "Intel",
	"5C:B9:01": "Ubiquiti",
	"00:27:22": "Ubiquiti",
	"04:18:D6": "Ubiquiti",
	"74:83:C2": "Ubiquiti",
	"FC:EC:DA": "Ubiquiti",
	"00:18:0A": "Cisco",
	"00:1B:2A": "Cisco",
	"64:F6:9D": "Cisco",
	"00:14:BF": "Linksys",
	"00:1A:70": "Linksys",
	"C0:C1:C0": "Linksys",
	"00:1F:33": "Netgear",
	"00:22:3F": "Netgear",
	"A0:63:91": "Netgear",
	"08:86:3B": "Belkin",
	"94:10:3E": "Belkin",
	"00:26:5A": "D-Link",
	"00:1E:58": "D-Link",
	"1C:7E:E5": "D-Link",
	"00:1D:0F": "TP-Link",
	"14:CC:20": "TP-Link",
	"50:C7:BF": "TP-Link",
	"00:25:00": "Apple",
	"00:26:08": "Apple",
	"28:CF:DA": "Apple",
	"3C:15:C2": "Apple",
	"5C:F9:38": "Apple",
	"78:31:C1": "Apple",
	"18:65:90": "Samsung",
	"50:01:BB": "Samsung",
	"94:35:0A": "Samsung",
	"2C:54:91": "Microsoft",
	"7C:1E:52": "Microsoft",
}

// GetMACVendor looks up the vendor for a MAC address
func GetMACVendor(mac string) string {
	if mac == "" {
		return "Unknown"
	}

	// Normalize MAC address format
	mac = strings.ToUpper(strings.ReplaceAll(mac, "-", ":"))

	// Get prefix (first 8 characters: XX:XX:XX)
	if len(mac) < 8 {
		return "Unknown"
	}
	prefix := mac[:8]

	if vendor, ok := macVendors[prefix]; ok {
		return vendor
	}
	return "Unknown"
}
