package scanner

import (
	"context"
	"net"
	"strconv"
	"sync"
	"time"

	"github.com/291-Group/LAN-Orangutan/internal/types"
)

// serviceProbePorts is the short, fixed set of ports the opt-in service probe
// checks. It is deliberately small: enough to sharpen device typing and spot a
// web interface, and never a general port scan. Every port here is one the
// classifier can act on, plus the common web ports for the web-interface flag.
var serviceProbePorts = []int{
	21,    // FTP (unencrypted, a security flag)
	22,    // SSH
	23,    // Telnet (unencrypted, a security flag)
	80,    // HTTP (web interface)
	443,   // HTTPS (web interface)
	515,   // LPD printing
	548,   // AFP (NAS)
	631,   // IPP printing
	3389,  // RDP
	5000,  // Synology DSM
	5001,  // Synology DSM (TLS)
	8080,  // alt HTTP (web interface)
	8096,  // Jellyfin
	9100,  // raw JetDirect printing
	32400, // Plex
	62078, // iOS lockdown
}

// riskyServices maps an open port to a plainly worded security concern. Kept
// deliberately high-signal: Telnet and FTP are unencrypted by design, so
// finding them open is a real red flag, not noise a router's web page would be.
// The dashboard translates these by their exact text (riskKeys in
// internal/web/handler.go), so a new or reworded entry needs a key there too.
var riskyServices = map[int]string{
	23: "Telnet is open (unencrypted remote access)",
	21: "FTP is open (unencrypted file transfer)",
}

// webPorts are the ports that, if open, mark a device as serving a web
// interface.
var webPorts = map[int]bool{80: true, 443: true, 8080: true}

// serviceProbeTimeout bounds a single TCP connection attempt. Short, because a
// device on the LAN answers in milliseconds and a closed port should not stall
// the scan waiting for it.
const serviceProbeTimeout = 400 * time.Millisecond

// serviceProbeConcurrency caps how many devices are probed at once, so a large
// network does not open thousands of sockets in the same instant.
const serviceProbeConcurrency = 24

// enrichWithServices probes each device's well-known ports and updates its type
// and web-interface flag from what it finds. It only runs when the user has
// enabled service detection. Devices are probed concurrently up to a cap; each
// device's ports are probed concurrently in turn, so the whole pass costs about
// one timeout per batch rather than one per closed port.
func enrichWithServices(ctx context.Context, devices []types.Device) {
	sem := make(chan struct{}, serviceProbeConcurrency)
	var wg sync.WaitGroup

	for i := range devices {
		wg.Add(1)
		sem <- struct{}{}
		go func(d *types.Device) {
			defer wg.Done()
			defer func() { <-sem }()

			ports := probeServices(ctx, d.IP)
			// Re-classify with the port evidence. Only overwrite when the fuller
			// signal yields a type, so a probe that finds nothing does not wipe
			// the vendor or hostname guess made earlier.
			if t := Classify(d.Vendor, d.Hostname, ports); t != "" {
				d.Type = t
			}
			d.WebUI = hasWebPort(ports)
			d.Risks = risksFromPorts(ports)
		}(&devices[i])
	}

	wg.Wait()
}

// probeServices attempts a TCP connection to each port in serviceProbePorts and
// returns those that accept one. Ports are tried concurrently with a short
// timeout, and the whole context deadline still applies so a cancelled scan
// stops promptly.
func probeServices(ctx context.Context, ip string) []int {
	var (
		mu   sync.Mutex
		open []int
		wg   sync.WaitGroup
	)

	for _, port := range serviceProbePorts {
		wg.Add(1)
		go func(p int) {
			defer wg.Done()
			dialer := net.Dialer{Timeout: serviceProbeTimeout}
			conn, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort(ip, strconv.Itoa(p)))
			if err != nil {
				return
			}
			conn.Close()
			mu.Lock()
			open = append(open, p)
			mu.Unlock()
		}(port)
	}

	wg.Wait()
	return open
}

// hasWebPort reports whether any of the probed open ports is a web port.
func hasWebPort(ports []int) bool {
	for _, p := range ports {
		if webPorts[p] {
			return true
		}
	}
	return false
}

// risksFromPorts turns the open ports into any security concerns they imply, in
// the fixed order of serviceProbePorts so the result is stable.
func risksFromPorts(ports []int) []string {
	var risks []string
	seen := make(map[int]bool, len(ports))
	for _, p := range ports {
		seen[p] = true
	}
	for _, p := range serviceProbePorts {
		if seen[p] {
			if msg, ok := riskyServices[p]; ok {
				risks = append(risks, msg)
			}
		}
	}
	return risks
}
