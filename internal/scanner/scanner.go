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
	"time"

	"github.com/291-Group/LAN-Orangutan/internal/types"
)

// Scanner performs network scans
type Scanner struct {
	minInterval time.Duration
}

// New creates a new Scanner
func New(minIntervalSeconds int) *Scanner {
	return &Scanner{
		minInterval: time.Duration(minIntervalSeconds) * time.Second,
	}
}

// nmapRun represents the root element of nmap XML output
type nmapRun struct {
	XMLName xml.Name   `xml:"nmaprun"`
	Hosts   []nmapHost `xml:"host"`
}

// nmapHost represents a host element in nmap XML output
type nmapHost struct {
	Status    nmapStatus      `xml:"status"`
	Addresses []nmapAddress   `xml:"address"`
	Hostnames nmapHostnames   `xml:"hostnames"`
	Times     nmapTimes       `xml:"times"`
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

	// Try nmap first
	devices, scanner, err := s.scanWithNmap(ctx, cidr)
	if err != nil {
		// Fallback to arp-scan
		devices, scanner, err = s.scanWithArpScan(ctx, cidr)
		if err != nil {
			return &types.ScanResult{
				Success:   false,
				Error:     err.Error(),
				Network:   cidr,
				Timestamp: time.Now(),
			}, nil
		}
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

// scanWithNmap performs a scan using nmap
func (s *Scanner) scanWithNmap(ctx context.Context, cidr string) ([]types.Device, string, error) {
	// Check if nmap is available
	if _, err := exec.LookPath("nmap"); err != nil {
		return nil, "", fmt.Errorf("nmap not found")
	}

	// Run nmap with ping scan and XML output
	cmd := exec.CommandContext(ctx, "nmap", "-sn", "-oX", "-", cidr)
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
