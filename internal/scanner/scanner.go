// Package scanner handles network device discovery using nmap and arp-scan
package scanner

import (
	"context"
	"encoding/xml"
	"fmt"
	"net"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/291-Group/LAN-Orangutan/internal/network"
	"github.com/291-Group/LAN-Orangutan/internal/types"
)

// Scanner performs network scans
type Scanner struct {
	minInterval time.Duration
	// serviceDetection turns on the opt-in probe that identifies devices by
	// their open ports. Off by default; a scan stays a quiet ping sweep. It can
	// be toggled at runtime from the Settings page, and a scan may be running on
	// another goroutine when that happens, so it is guarded by mu.
	mu               sync.RWMutex
	serviceDetection bool
}

// New creates a new Scanner. serviceDetection enables the opt-in port probe
// used to identify devices more precisely.
func New(minIntervalSeconds int, serviceDetection bool) *Scanner {
	return &Scanner{
		minInterval:      time.Duration(minIntervalSeconds) * time.Second,
		serviceDetection: serviceDetection,
	}
}

// SetServiceDetection turns the opt-in port probe on or off. The change takes
// effect on the next scan.
func (s *Scanner) SetServiceDetection(on bool) {
	s.mu.Lock()
	s.serviceDetection = on
	s.mu.Unlock()
}

// serviceDetectionEnabled reports whether the port probe is currently on.
func (s *Scanner) serviceDetectionEnabled() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.serviceDetection
}

// nmapRun represents the root element of nmap XML output
type nmapRun struct {
	XMLName xml.Name   `xml:"nmaprun"`
	Hosts   []nmapHost `xml:"host"`
}

// nmapHost represents a host element in nmap XML output
type nmapHost struct {
	Status    nmapStatus    `xml:"status"`
	Addresses []nmapAddress `xml:"address"`
	Hostnames nmapHostnames `xml:"hostnames"`
	Times     nmapTimes     `xml:"times"`
}

// nmapStatus represents the host status
type nmapStatus struct {
	State string `xml:"state,attr"`
}

// nmapAddress represents an address element
type nmapAddress struct {
	Addr     string `xml:"addr,attr"`
	AddrType string `xml:"addrtype,attr"`
	Vendor   string `xml:"vendor,attr"`
}

// nmapHostnames contains hostname information
type nmapHostnames struct {
	Hostnames []nmapHostname `xml:"hostname"`
}

// nmapHostname represents a single hostname
type nmapHostname struct {
	Name string `xml:"name,attr"`
	Type string `xml:"type,attr"`
}

// nmapTimes contains timing information
type nmapTimes struct {
	SRTT string `xml:"srtt,attr"`
}

// Scan performs a network scan on the given CIDR
func (s *Scanner) Scan(ctx context.Context, cidr string) (*types.ScanResult, error) {
	// Validate CIDR
	_, _, err := net.ParseCIDR(cidr)
	if err != nil {
		return nil, fmt.Errorf("invalid CIDR: %w", err)
	}

	startTime := time.Now()

	// Tailscale gives every node its own /32, so there is no subnet to sweep
	// and a scan could only ever find this machine. Tailscale already knows the
	// whole tailnet, so ask it for the peers instead.
	if network.IsTailscaleNetwork(cidr) {
		return s.scanTailscale(cidr, startTime), nil
	}

	// Try nmap first, then arp-scan, but only where arp-scan can exist. It has
	// no Windows build, so there a failed nmap scan is the whole story, and
	// falling through would replace the real reason with "arp-scan not found".
	devices, scanner, err := s.scanWithNmap(ctx, cidr)
	if err != nil {
		nmapErr := err
		var arpErr error
		if ArpScanAvailable() {
			devices, scanner, err = s.scanWithArpScan(ctx, cidr)
			arpErr = err
		}
		if err != nil {
			return &types.ScanResult{
				Success:   false,
				Error:     ScanFailureMessage(nmapErr, arpErr),
				Network:   cidr,
				Timestamp: time.Now(),
			}, nil
		}
	}

	// Recover names for devices that reverse DNS could not name, over NetBIOS.
	// This is name resolution like the reverse DNS lookup above, so it always
	// runs, and only touches devices that still have no hostname.
	fillWindowsNames(ctx, devices)

	// With service detection on, probe each device's well-known ports to refine
	// its type and flag a web interface. This is the only step that sends more
	// than a ping or a name query, so it stays behind the opt-in flag.
	if s.serviceDetectionEnabled() {
		enrichWithServices(ctx, devices)
	}

	duration := time.Since(startTime).Seconds()

	return &types.ScanResult{
		Success:     true,
		Devices:     devices,
		DeviceCount: len(devices),
		Network:     cidr,
		Scanner:     scanner,
		Duration:    duration,
		Timestamp:   time.Now(),
	}, nil
}

// scanTailscale lists the devices reachable over Tailscale.
func (s *Scanner) scanTailscale(cidr string, startTime time.Time) *types.ScanResult {
	devices := network.GetTailscaleDevices()
	if devices == nil {
		return &types.ScanResult{
			Success:   false,
			Error:     "Tailscale is not connected",
			Network:   cidr,
			Timestamp: time.Now(),
		}
	}

	return &types.ScanResult{
		Success:     true,
		Devices:     devices,
		DeviceCount: len(devices),
		Network:     cidr,
		Scanner:     "tailscale",
		Duration:    time.Since(startTime).Seconds(),
		Timestamp:   time.Now(),
	}
}

// scanWithNmap performs a scan using nmap
func (s *Scanner) scanWithNmap(ctx context.Context, cidr string) ([]types.Device, string, error) {
	// Resolve nmap by path rather than name: NmapPath also checks the standard
	// install locations, so an install that PATH has not picked up yet still
	// works instead of reporting the tool missing.
	nmapBin, err := NmapPath()
	if err != nil {
		return nil, "", err
	}

	// Run nmap with ping scan and XML output
	cmd := exec.CommandContext(ctx, nmapBin, "-sn", "-oX", "-", cidr)
	output, err := cmd.Output()
	if err != nil {
		return nil, "", fmt.Errorf("nmap failed: %w", err)
	}

	// Parse XML output
	var result nmapRun
	if err := xml.Unmarshal(output, &result); err != nil {
		return nil, "", fmt.Errorf("failed to parse nmap output: %w", err)
	}

	var devices []types.Device
	for _, host := range result.Hosts {
		if host.Status.State != "up" {
			continue
		}

		device := types.Device{}

		// Extract addresses
		for _, addr := range host.Addresses {
			switch addr.AddrType {
			case "ipv4":
				device.IP = addr.Addr
			case "mac":
				device.MAC = addr.Addr
				if addr.Vendor != "" {
					device.Vendor = addr.Vendor
				}
			}
		}

		if device.IP == "" {
			continue
		}

		// Get vendor from MAC if not set
		if device.Vendor == "" && device.MAC != "" {
			device.Vendor = GetMACVendor(device.MAC)
		}

		// Extract hostname
		for _, hostname := range host.Hostnames.Hostnames {
			if hostname.Name != "" {
				device.Hostname = hostname.Name
				break
			}
		}

		// Try reverse DNS if no hostname
		if device.Hostname == "" {
			device.Hostname = reverseDNS(device.IP)
		}

		// Parse response time
		if host.Times.SRTT != "" {
			if srtt, err := parseResponseTime(host.Times.SRTT); err == nil {
				device.ResponseTime = &srtt
			}
		}

		// Infer the device type from the vendor and hostname. Port signals are
		// not available here yet; a ping scan finds no open ports. Opt-in
		// service detection will pass them in a later step.
		device.Type = Classify(device.Vendor, device.Hostname, nil)

		devices = append(devices, device)
	}

	return devices, "nmap", nil
}

// scanWithArpScan performs a scan using arp-scan
func (s *Scanner) scanWithArpScan(ctx context.Context, cidr string) ([]types.Device, string, error) {
	// Check if arp-scan is available
	if _, err := exec.LookPath("arp-scan"); err != nil {
		return nil, "", fmt.Errorf("arp-scan not found")
	}

	// Extract interface from CIDR if possible
	iface := getInterfaceForCIDR(ctx, cidr)

	// Run arp-scan
	args := []string{"--localnet", "-q"}
	if iface != "" {
		args = append(args, "-I", iface)
	}
	cmd := exec.CommandContext(ctx, "arp-scan", args...)
	output, err := cmd.Output()
	if err != nil {
		return nil, "", fmt.Errorf("arp-scan failed: %w", err)
	}

	// Parse output (format: IP\tMAC\tVendor)
	var devices []types.Device
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "Interface:") || strings.HasPrefix(line, "Starting") || strings.HasPrefix(line, "Ending") {
			continue
		}

		parts := strings.Split(line, "\t")
		if len(parts) < 2 {
			continue
		}

		ip := strings.TrimSpace(parts[0])
		mac := strings.TrimSpace(parts[1])

		// Validate IP
		if net.ParseIP(ip) == nil {
			continue
		}

		device := types.Device{
			IP:  ip,
			MAC: mac,
		}

		// Get vendor
		if len(parts) >= 3 {
			device.Vendor = strings.TrimSpace(parts[2])
		}
		if device.Vendor == "" {
			device.Vendor = GetMACVendor(mac)
		}

		// Try reverse DNS
		device.Hostname = reverseDNS(ip)

		device.Type = Classify(device.Vendor, device.Hostname, nil)

		devices = append(devices, device)
	}

	return devices, "arp-scan", nil
}

// reverseDNS performs a reverse DNS lookup
func reverseDNS(ip string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	done := make(chan string, 1)
	go func() {
		names, err := net.LookupAddr(ip)
		if err != nil || len(names) == 0 {
			done <- ""
			return
		}
		// Remove trailing dot
		hostname := strings.TrimSuffix(names[0], ".")
		done <- hostname
	}()

	select {
	case <-ctx.Done():
		return ""
	case hostname := <-done:
		return hostname
	}
}

// parseResponseTime parses nmap SRTT value (microseconds) to milliseconds
func parseResponseTime(srtt string) (float64, error) {
	var usec int
	if _, err := fmt.Sscanf(srtt, "%d", &usec); err != nil {
		return 0, err
	}
	return float64(usec) / 1000.0, nil
}

// CheckRateLimit checks if a scan can proceed based on rate limiting
func (s *Scanner) CheckRateLimit(lastScan time.Time) (bool, time.Duration) {
	if lastScan.IsZero() {
		return true, 0
	}

	elapsed := time.Since(lastScan)
	if elapsed >= s.minInterval {
		return true, 0
	}

	return false, s.minInterval - elapsed
}

// getInterfaceForCIDR tries to determine which network interface to use for a given CIDR
func getInterfaceForCIDR(ctx context.Context, cidr string) string {
	targetIP := strings.Split(cidr, "/")[0]

	switch runtime.GOOS {
	case "linux":
		// Linux: use ip route get
		cmd := exec.CommandContext(ctx, "ip", "route", "get", targetIP)
		if output, err := cmd.Output(); err == nil {
			parts := strings.Fields(string(output))
			for i, p := range parts {
				if p == "dev" && i+1 < len(parts) {
					return parts[i+1]
				}
			}
		}

	case "darwin":
		// macOS: use route get
		cmd := exec.CommandContext(ctx, "route", "-n", "get", targetIP)
		if output, err := cmd.Output(); err == nil {
			lines := strings.Split(string(output), "\n")
			for _, line := range lines {
				if strings.Contains(line, "interface:") {
					parts := strings.Fields(line)
					if len(parts) >= 2 {
						return parts[len(parts)-1]
					}
				}
			}
		}
	}

	// Fallback: try to find an interface that matches the CIDR using Go's net package
	_, cidrNet, err := net.ParseCIDR(cidr)
	if err != nil {
		return ""
	}

	ifaces, err := net.Interfaces()
	if err != nil {
		return ""
	}

	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			if ipNet, ok := addr.(*net.IPNet); ok {
				if cidrNet.Contains(ipNet.IP) {
					return iface.Name
				}
			}
		}
	}

	return ""
}
