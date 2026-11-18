// Package api implements the REST API endpoints
package api

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"log"
	"net"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/291-Group/LAN-Orangutan/internal/auth"
	"github.com/291-Group/LAN-Orangutan/internal/config"
	"github.com/291-Group/LAN-Orangutan/internal/i18n"
	"github.com/291-Group/LAN-Orangutan/internal/network"
	"github.com/291-Group/LAN-Orangutan/internal/notify"
	"github.com/291-Group/LAN-Orangutan/internal/report"
	"github.com/291-Group/LAN-Orangutan/internal/scanner"
	"github.com/291-Group/LAN-Orangutan/internal/snapshot"
	"github.com/291-Group/LAN-Orangutan/internal/storage"
	"github.com/291-Group/LAN-Orangutan/internal/types"
	"github.com/291-Group/LAN-Orangutan/internal/uerr"
)

// Handler handles API requests
type Handler struct {
	store   *storage.Storage
	cfg     *config.Config
	scanner *scanner.Scanner

	// auth is held so the API token can be created and revoked through the
	// API itself, which is how the Settings page does it.
	auth *auth.Authenticator

	// jobMu guards job, which holds the most recent background scan. Only one
	// scan runs at a time.
	jobMu sync.Mutex
	job   *scanJob

	// scanCtx is the parent of every scan job's context, so cancelling it at
	// shutdown stops any scan in flight. scanWG tracks the running scan
	// goroutines so shutdown can wait for them to unwind rather than abandon a
	// scan mid-write.
	scanCtx context.Context
	scanWG  sync.WaitGroup
}

// NewHandler creates a new API handler
func NewHandler(store *storage.Storage, cfg *config.Config, authn *auth.Authenticator) *Handler {
	// Honour a saved UI override for service detection at startup, falling
	// back to the config default.
	s := scanner.New(
		cfg.Scanning.MinScanInterval,
		store.ServiceDetectionEnabled(cfg.Scanning.EnableServiceDetection),
	)
	s.SetRoutedTiming(cfg.Scanning.NmapMaxRTTTimeout, cfg.Scanning.NmapMaxRetries)
	return &Handler{
		store:   store,
		cfg:     cfg,
		auth:    authn,
		scanner: s,
		scanCtx: context.Background(),
	}
}

// SetScanContext sets the parent context for all scan jobs. Cancelling it stops
// any scan in flight, so call it with the server's shutdown context before
// starting the server.
func (h *Handler) SetScanContext(ctx context.Context) { h.scanCtx = ctx }

// WaitForScans blocks until any running scan goroutine has returned. Call it at
// shutdown, after the scan context has been cancelled, so a scan is not left
// half-written.
func (h *Handler) WaitForScans() { h.scanWG.Wait() }

// ServeHTTP implements http.Handler
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Set JSON content type for all API responses
	w.Header().Set("Content-Type", "application/json")

	// CSRF protection for mutating requests.
	//
	// Cross-site request forgery works because a browser attaches the session
	// cookie to a request another site made it send. A browser never attaches
	// an Authorization header on its own, so a request the auth middleware
	// accepted on the strength of a valid API token cannot have been forged
	// that way, and this check would only produce a confusing error for a
	// script that is already proving who it is. The header alone is not
	// enough: a request can carry one and still have been let in by its
	// session cookie, so ask the authenticator which credential it accepted.
	if r.Method == http.MethodPost || r.Method == http.MethodPut || r.Method == http.MethodDelete {
		if h.auth == nil || !h.auth.TokenAuthenticated(r) {
			contentType := r.Header.Get("Content-Type")
			xRequestedWith := r.Header.Get("X-Requested-With")

			if !strings.Contains(contentType, "application/json") && xRequestedWith != "XMLHttpRequest" {
				h.error(w, http.StatusForbidden, "CSRF protection: requires Content-Type: application/json or X-Requested-With header")
				return
			}
		}
	}

	// Route to handler
	path := strings.TrimPrefix(r.URL.Path, "/api/")
	path = strings.TrimSuffix(path, "/")

	switch {
	case path == "devices":
		h.handleDevices(w, r)
	case path == "device":
		h.handleDevice(w, r)
	case path == "devices/mark-expected":
		h.handleMarkAllExpected(w, r)
	case path == "networks":
		h.handleNetworks(w, r)
	case path == "groups":
		h.handleGroups(w, r)
	case path == "locations":
		h.handleLocations(w, r)
	case path == "scan":
		h.handleScan(w, r)
	case path == "scan/start":
		h.handleScanStart(w, r)
	case path == "scan/progress":
		h.handleScanProgress(w, r)
	case path == "scan/cancel":
		h.handleScanCancel(w, r)
	case path == "scan/continuous":
		h.handleScanContinuous(w, r)
	case path == "scan/service-detection":
		h.handleScanServiceDetection(w, r)
	case path == "scan/interval":
		h.handleScanInterval(w, r)
	case path == "scan/networks":
		h.handleScanNetworks(w, r)
	case path == "tailscale":
		h.handleTailscale(w, r)
	case path == "tailscale/connect":
		h.handleTailscaleConnect(w, r)
	case path == "tailscale/disconnect":
		h.handleTailscaleDisconnect(w, r)
	case path == "stats":
		h.handleStats(w, r)
	case path == "status":
		h.handleStatus(w, r)
	case path == "settings":
		h.handleSettings(w, r)
	case path == "token":
		h.handleAPIToken(w, r)
	case path == "snapshots":
		h.handleSnapshots(w, r)
	case path == "snapshots/compare":
		h.handleSnapshotCompare(w, r)
	case path == "backup":
		h.handleBackup(w, r)
	case path == "backup/restore":
		h.handleBackupRestore(w, r)
	case path == "report":
		h.handleReport(w, r)
	case path == "webhooks":
		h.handleWebhooks(w, r)
	case path == "webhooks/test":
		h.handleWebhookTest(w, r)
	default:
		h.error(w, http.StatusNotFound, "endpoint not found")
	}
}

// handleDevices handles GET /api/devices
func (h *Handler) handleDevices(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.error(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	devices := h.store.GetDevices()

	if r.URL.Query().Get("format") == "csv" {
		h.writeDevicesCSV(w, devices)
		return
	}

	out := make(map[string]deviceOut, len(devices))
	for ip, d := range devices {
		out[ip] = withStatus(d)
	}
	h.success(w, out)
}

// deviceOut is a device as the API returns it: the stored record plus whether
// it is online now, which a script would otherwise have to work out from
// last_seen and the one-hour rule itself.
type deviceOut struct {
	*types.Device
	Online bool `json:"online"`
}

func withStatus(d *types.Device) deviceOut {
	return deviceOut{Device: d, Online: d.IsOnline()}
}

// writeDevicesCSV sends the device list as a downloadable CSV file.
//
// The columns match `orangutan export`, so a file saved from the browser and
// one saved from the command line are interchangeable.
func (h *Handler) writeDevicesCSV(w http.ResponseWriter, devices map[string]*types.Device) {
	sorted := make([]*types.Device, 0, len(devices))
	for _, d := range devices {
		sorted = append(sorted, d)
	}
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].IP < sorted[j].IP })

	// Set before writing: headers are ignored once the body has started.
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="devices.csv"`)

	cw := csv.NewWriter(w)
	defer cw.Flush()

	_ = cw.Write([]string{
		"IP Address", "MAC Address", "Hostname", "Vendor", "Label",
		"Notes", "Group", "Location", "First Seen", "Last Seen", "Status",
	})

	for _, d := range sorted {
		status := "offline"
		if d.IsOnline() {
			status = "online"
		}
		_ = cw.Write([]string{
			d.IP, d.MAC, d.DisplayHostname(), scanner.ResolveVendor(d.Vendor, d.MAC), d.Label, d.Notes, d.Group, d.Location,
			d.FirstSeen.Format("2006-01-02 15:04:05"),
			d.LastSeen.Format("2006-01-02 15:04:05"),
			status,
		})
	}
}

// handleDevice handles GET/POST/DELETE /api/device
func (h *Handler) handleDevice(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		ip := r.URL.Query().Get("ip")
		if ip == "" {
			h.fail(w, http.StatusBadRequest, errIPRequired)
			return
		}
		device := h.store.GetDevice(ip)
		if device == nil {
			h.fail(w, http.StatusNotFound, uerr.Newf("device.notFound", "no device at %s", "ip", ip))
			return
		}
		h.success(w, withStatus(device))

	case http.MethodPost:
		var req struct {
			IP             string  `json:"ip"`
			Label          *string `json:"label"`
			Notes          *string `json:"notes"`
			Group          *string `json:"group"`
			Expected       *bool   `json:"expected"`
			Type           *string `json:"type"`
			CustomType     *bool   `json:"custom_type"`
			CustomHostname *string `json:"custom_hostname"`
			Location       *string `json:"location"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			h.fail(w, http.StatusBadRequest, errInvalidBody)
			return
		}

		// Get IP from query if not in body
		ip := req.IP
		if ip == "" {
			ip = r.URL.Query().Get("ip")
		}
		if ip == "" {
			h.fail(w, http.StatusBadRequest, errIPRequired)
			return
		}

		if req.Type != nil {
			cleanType, err := scanner.ValidateType(*req.Type)
			if err != nil {
				h.fail(w, http.StatusBadRequest, err)
				return
			}
			req.Type = &cleanType
		}

		if err := h.store.UpdateDeviceFields(ip, req.Label, req.Notes, req.Group, req.Expected, req.Type, req.CustomType, req.CustomHostname, req.Location); err != nil {
			h.fail(w, inputStatus(err), err)
			return
		}
		h.success(w, map[string]string{"message": "device updated"})

	case http.MethodDelete:
		ip := r.URL.Query().Get("ip")
		if ip == "" {
			h.fail(w, http.StatusBadRequest, errIPRequired)
			return
		}
		if err := h.store.DeleteDevice(ip); err != nil {
			h.fail(w, http.StatusNotFound, err)
			return
		}
		h.success(w, map[string]string{"message": "device deleted"})

	default:
		h.error(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// handleMarkAllExpected handles POST /api/devices/mark-expected, setting or
// clearing the Expected flag on every device at once to baseline the network.
func (h *Handler) handleMarkAllExpected(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		h.error(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var body struct {
		Expected bool `json:"expected"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		h.fail(w, http.StatusBadRequest, errInvalidBody)
		return
	}
	if err := h.store.MarkAllExpected(body.Expected); err != nil {
		h.error(w, http.StatusInternalServerError, "could not update devices: "+err.Error())
		return
	}
	h.success(w, map[string]bool{"expected": body.Expected})
}

// handleNetworks handles GET /api/networks
func (h *Handler) handleNetworks(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.error(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	networks, err := network.DetectNetworks()
	if err != nil {
		h.error(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.success(w, uniqueNetworks(network.WithConfigured(networks, h.networkFilter())))
}

// uniqueNetworks drops a network listed twice (two interfaces on the same
// subnet report the same CIDR), keeping the first.
func uniqueNetworks(nets []types.Network) []types.Network {
	seen := make(map[string]bool, len(nets))
	out := nets[:0:0]
	for _, n := range nets {
		if seen[n.CIDR] {
			continue
		}
		seen[n.CIDR] = true
		out = append(out, n)
	}
	return out
}

// handleScan handles GET /api/scan
func (h *Handler) handleScan(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.error(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	cidr := r.URL.Query().Get("network")
	if cidr == "" {
		h.fail(w, http.StatusBadRequest, errNetworkRequired)
		return
	}

	// "all" scans every detected network, matching the CLI's behaviour
	if strings.EqualFold(cidr, "all") {
		h.scanAllNetworks(w, r)
		return
	}

	// Validate CIDR
	if !network.ValidateCIDR(cidr) {
		h.fail(w, http.StatusBadRequest, invalidNetwork(cidr))
		return
	}

	// Check rate limit
	lastScan := h.store.GetLastScan(cidr)
	canScan, waitTime := h.scanner.CheckRateLimit(lastScan)
	if !canScan {
		h.error(w, http.StatusTooManyRequests,
			"rate limited, wait "+waitTime.Round(time.Second).String())
		return
	}

	// Tell the webhooks what this scan changed, once it is over. Started only
	// now, past every check that can reject the request, so a refused scan
	// never advances the notifier's state.
	defer notify.Begin(h.store).Finish()

	// Perform scan
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
	defer cancel()

	result, err := h.scanner.Scan(ctx, cidr)
	if err != nil {
		h.error(w, http.StatusInternalServerError, err.Error())
		return
	}

	if !result.Success {
		h.error(w, http.StatusInternalServerError, result.Error)
		return
	}

	// Merge devices into storage
	if err := h.store.MergeScan(cidr, result.Devices); err != nil {
		h.error(w, http.StatusInternalServerError, "failed to save devices")
		return
	}

	// Update last scan time
	h.store.SetLastScan(cidr, time.Now())

	h.success(w, result)
}

// networkScanSummary reports the outcome of scanning a single network as part
// of a scan-all request.
type networkScanSummary struct {
	Network     string  `json:"network"`
	Status      string  `json:"status"` // scanned, skipped or failed
	DeviceCount int     `json:"device_count"`
	Duration    float64 `json:"duration"`
	Error       string  `json:"error,omitempty"`
}

// scanAllResult aggregates the per-network outcomes of a scan-all request.
type scanAllResult struct {
	Success      bool                 `json:"success"`
	Networks     []networkScanSummary `json:"networks"`
	NetworkCount int                  `json:"network_count"`
	ScannedCount int                  `json:"scanned_count"`
	DeviceCount  int                  `json:"device_count"`
	Timestamp    time.Time            `json:"timestamp"`
}

// scanAllNetworks scans every detected network. A network that is rate limited
// or fails is reported in the response rather than failing the whole request,
// so one bad interface cannot mask results from the others.
func (h *Handler) scanAllNetworks(w http.ResponseWriter, r *http.Request) {
	detected, err := network.DetectNetworks()
	if err != nil {
		h.error(w, http.StatusInternalServerError, "failed to detect networks: "+err.Error())
		return
	}
	detected = uniqueNetworks(network.WithConfigured(detected, h.networkFilter()))
	if len(detected) == 0 {
		h.error(w, http.StatusNotFound, "no networks detected")
		return
	}

	// Tell the webhooks what this scan changed, once it is over (see
	// handleScan for why this waits until the request is accepted).
	defer notify.Begin(h.store).Finish()

	result := scanAllResult{
		Networks:     make([]networkScanSummary, 0, len(detected)),
		NetworkCount: len(detected),
		Timestamp:    time.Now(),
	}

	// The networks that actually completed, for the ARP-table pass below.
	var scannedCIDRs []string

	for _, n := range detected {
		summary := networkScanSummary{Network: n.CIDR}

		lastScan := h.store.GetLastScan(n.CIDR)
		if canScan, waitTime := h.scanner.CheckRateLimit(lastScan); !canScan {
			summary.Status = "skipped"
			summary.Error = "rate limited, wait " + waitTime.Round(time.Second).String()
			result.Networks = append(result.Networks, summary)
			continue
		}

		scan, err := h.scanNetwork(r.Context(), n.CIDR)
		if err != nil {
			summary.Status = "failed"
			summary.Error = err.Error()
			result.Networks = append(result.Networks, summary)
			continue
		}

		summary.Status = "scanned"
		summary.DeviceCount = scan.DeviceCount
		summary.Duration = scan.Duration
		result.Networks = append(result.Networks, summary)
		result.ScannedCount++
		result.DeviceCount += scan.DeviceCount
		scannedCIDRs = append(scannedCIDRs, n.CIDR)
	}

	// Supplemental sources, once for the whole request. IPv6 neighbors (an IPv6
	// subnet cannot be swept) enrich devices by MAC rather than being listed per
	// address, so they go through MergeIPv6Neighbors. mDNS announcements merge as
	// an ordinary secondary source.
	// The ARP table holds devices this machine has spoken to that did not answer
	// nmap's ARP probe during the sweep, which is ordinary for hardware that
	// sleeps. Restricted to the networks just scanned so container bridges and
	// VPN peers cannot appear as LAN devices.
	if arp := h.scanner.DiscoverARP(r.Context(), scannedCIDRs); len(arp) > 0 {
		if err := h.store.MergeSupplemental(arp); err == nil {
			result.Networks = append(result.Networks, networkScanSummary{
				Network: "ARP table", Status: "scanned", DeviceCount: len(arp),
			})
			result.ScannedCount++
		}
	}
	// An IPv6 network in the list was already scanned through the neighbour
	// table, so running the neighbour pass again would count its devices twice.
	if !network.HasIPv6Network(scannedCIDRs) {
		if ipv6 := h.scanner.DiscoverIPv6(r.Context()); len(ipv6) > 0 {
			if err := h.store.MergeIPv6Neighbors(ipv6); err == nil {
				result.Networks = append(result.Networks, networkScanSummary{
					Network: "IPv6 neighbors", Status: "scanned", DeviceCount: len(ipv6),
				})
				result.ScannedCount++
				result.DeviceCount += len(ipv6)
			}
		}
	}
	if mdns := h.scanner.DiscoverMDNS(r.Context()); len(mdns) > 0 {
		if err := h.store.MergeSupplemental(mdns); err == nil {
			result.Networks = append(result.Networks, networkScanSummary{
				Network: "mDNS", Status: "scanned", DeviceCount: len(mdns),
			})
			result.ScannedCount++
			result.DeviceCount += len(mdns)
		}
	}

	// Only a request where nothing at all could be scanned counts as a failure.
	result.Success = result.ScannedCount > 0
	h.success(w, result)
}

// resolveScanTargets turns the network parameter into the list of networks to
// scan. "all" expands to every detected network, matching the CLI.
func (h *Handler) resolveScanTargets(cidr string) ([]string, error) {
	if !strings.EqualFold(cidr, "all") {
		if !network.ValidateCIDR(cidr) {
			return nil, invalidNetwork(cidr)
		}
		return []string{cidr}, nil
	}

	detected, err := network.DetectNetworks()
	if err != nil {
		return nil, errors.New("failed to detect networks: " + err.Error())
	}
	detected = uniqueNetworks(network.WithConfigured(detected, h.networkFilter()))
	if len(detected) == 0 {
		return nil, errors.New("no networks detected")
	}

	networks := make([]string, 0, len(detected))
	for _, n := range detected {
		networks = append(networks, n.CIDR)
	}
	return networks, nil
}

// handleScanStart handles POST /api/scan/start, which begins a scan in the
// background and returns immediately. Scanning a large network takes minutes,
// so the UI starts a job and polls /api/scan/progress rather than holding a
// request open for the duration.
func (h *Handler) handleScanStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		h.error(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	cidr := r.URL.Query().Get("network")
	if cidr == "" {
		h.fail(w, http.StatusBadRequest, errNetworkRequired)
		return
	}

	networks, err := h.resolveScanTargets(cidr)
	if err != nil {
		h.fail(w, http.StatusBadRequest, err)
		return
	}

	h.jobMu.Lock()
	defer h.jobMu.Unlock()

	if h.job != nil && h.job.isRunning() {
		// A scan is already running, most often the background scanner. Rather
		// than rejecting the user's click with a 409, attach to that scan and
		// adopt it as user-initiated so the progress overlay follows it and the
		// user gets the fresh results it is already producing.
		h.job.adopt()
		h.success(w, h.job.snapshot())
		return
	}

	h.job = h.startScanJob(networks, false)
	h.success(w, h.job.snapshot())
}

// handleScanProgress handles GET /api/scan/progress.
func (h *Handler) handleScanProgress(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.error(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	h.jobMu.Lock()
	job := h.job
	h.jobMu.Unlock()

	if job == nil {
		h.success(w, scanProgress{Status: "idle", Percent: percentUnknown})
		return
	}
	h.success(w, job.snapshot())
}

// handleScanCancel handles POST /api/scan/cancel.
func (h *Handler) handleScanCancel(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		h.error(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	h.jobMu.Lock()
	job := h.job
	h.jobMu.Unlock()

	if job == nil || !job.isRunning() {
		h.error(w, http.StatusConflict, "no scan is running")
		return
	}

	job.cancel()
	h.success(w, map[string]string{"message": "scan cancelled"})
}

// handleScanContinuous reports (GET) or sets (POST) whether the server keeps
// re-scanning the networks on its own. The choice is persisted so it survives a
// restart, letting a user scan once and sit on the snapshot without editing the
// config or restarting.
func (h *Handler) handleScanContinuous(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.success(w, map[string]bool{"enabled": h.store.ContinuousScanEnabled(h.cfg.Scanning.ContinuousScan)})
	case http.MethodPost:
		var body struct {
			Enabled bool `json:"enabled"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			h.fail(w, http.StatusBadRequest, errInvalidBody)
			return
		}
		if err := h.store.SetContinuousScan(body.Enabled); err != nil {
			h.error(w, http.StatusInternalServerError, "could not save setting: "+err.Error())
			return
		}
		// Turning it on kicks off a scan right away, so the user sees fresh
		// results instead of waiting up to a full interval for the next tick.
		// Runs in the background; the one-scan-at-a-time guard and the
		// per-network rate limit keep it from piling up or hammering.
		if body.Enabled {
			go h.runBackgroundScan()
		}
		h.success(w, map[string]bool{"enabled": body.Enabled})
	default:
		h.error(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// handleScanServiceDetection handles GET/POST /api/scan/service-detection, the
// runtime toggle for the opt-in port probe. Like continuous scan, the choice is
// saved as an override in the data dir and survives a restart; the change is
// pushed to the live scanner and takes effect on the next scan.
func (h *Handler) handleScanServiceDetection(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.success(w, map[string]bool{"enabled": h.store.ServiceDetectionEnabled(h.cfg.Scanning.EnableServiceDetection)})
	case http.MethodPost:
		var body struct {
			Enabled bool `json:"enabled"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			h.fail(w, http.StatusBadRequest, errInvalidBody)
			return
		}
		if err := h.store.SetServiceDetection(body.Enabled); err != nil {
			h.error(w, http.StatusInternalServerError, "could not save setting: "+err.Error())
			return
		}
		h.scanner.SetServiceDetection(body.Enabled)
		h.success(w, map[string]bool{"enabled": body.Enabled})
	default:
		h.error(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// Bounds for the background scan interval override (seconds).
const (
	minScanIntervalSeconds = 10
	maxScanIntervalSeconds = 86400 // one day
)

// handleScanInterval handles GET/POST /api/scan/interval, the runtime setting for
// how often the background scanner runs, in seconds. Saved as an override in the
// data dir; the scanner re-reads it before each cycle, so a change takes effect
// on the next automatic scan.
func (h *Handler) handleScanInterval(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.success(w, map[string]int{
			"seconds": h.store.ScanIntervalOrDefault(h.cfg.Scanning.ScanInterval),
			"default": h.cfg.Scanning.ScanInterval,
		})
	case http.MethodPost:
		// A pointer, so a body that omits "seconds" (or misspells it) is rejected
		// rather than decoding to 0 and being clamped up to the floor. Silently
		// turning a malformed request into a 10-second scan loop is the wrong
		// kind of forgiving. Matches how the storage limits are handled below.
		var body struct {
			Seconds *int `json:"seconds"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			h.fail(w, http.StatusBadRequest, errInvalidBody)
			return
		}
		if body.Seconds == nil {
			h.fail(w, http.StatusBadRequest, uerr.New("seconds.required", "seconds is required"))
			return
		}
		secs := clampInt(*body.Seconds, minScanIntervalSeconds, maxScanIntervalSeconds)
		if err := h.store.SetScanInterval(secs); err != nil {
			h.error(w, http.StatusInternalServerError, "could not save setting: "+err.Error())
			return
		}
		h.success(w, map[string]int{"seconds": secs, "default": h.cfg.Scanning.ScanInterval})
	default:
		h.error(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// handleScanNetworks handles GET/POST /api/scan/networks, the runtime settings
// for which networks to scan: an extra include list, an exclude list, and
// whether to scan only the configured ones. Saved as overrides in the data dir
// and read fresh when scan targets are resolved, so a change applies on the next
// scan. GET also returns the auto-detected networks so the editor can offer real
// choices rather than a blank box.
func (h *Handler) handleScanNetworks(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.success(w, h.networkSettingsView())

	case http.MethodPost:
		var body struct {
			Networks        *[]string `json:"networks"`
			ExcludeNetworks *[]string `json:"exclude_networks"`
			OnlyConfigured  *bool     `json:"only_configured"`
			Reset           bool      `json:"reset"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			h.fail(w, http.StatusBadRequest, errInvalidBody)
			return
		}
		if body.Reset {
			if err := h.store.SetNetworkOverrides(nil, nil, nil); err != nil {
				h.error(w, http.StatusInternalServerError, "could not save settings: "+err.Error())
				return
			}
			h.success(w, h.networkSettingsView())
			return
		}
		if body.Networks != nil {
			cleaned, err := validateCIDRs(*body.Networks)
			if err != nil {
				h.fail(w, http.StatusBadRequest, err)
				return
			}
			body.Networks = &cleaned
		}
		if body.ExcludeNetworks != nil {
			cleaned, err := validateCIDRs(*body.ExcludeNetworks)
			if err != nil {
				h.fail(w, http.StatusBadRequest, err)
				return
			}
			body.ExcludeNetworks = &cleaned
		}
		if err := h.store.SetNetworkOverrides(body.Networks, body.ExcludeNetworks, body.OnlyConfigured); err != nil {
			h.error(w, http.StatusInternalServerError, "could not save settings: "+err.Error())
			return
		}
		h.success(w, h.networkSettingsView())

	default:
		h.error(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// networkSettingsView returns the effective scan-target lists, whether an
// override is in effect, and the currently detected networks for the editor to
// offer as choices.
func (h *Handler) networkSettingsView() map[string]interface{} {
	f := h.networkFilter()
	detected := []string{}
	if nets, err := network.DetectNetworks(); err == nil {
		for _, n := range uniqueNetworks(nets) {
			detected = append(detected, n.CIDR)
		}
	}
	nOver, eOver, oOver := h.store.GetNetworkOverrides()
	// Never return a nil slice: the JSON should be [] so the UI can treat it as
	// an empty list rather than null.
	return map[string]interface{}{
		"networks":         nonNil(f.Configured),
		"exclude_networks": nonNil(f.Excluded),
		"only_configured":  f.OnlyConfigured,
		"detected":         detected,
		"overridden":       nOver != nil || eOver != nil || oOver != nil,
	}
}

// handleGroups handles GET/POST /api/groups, the device-group list shown in the
// dashboard dropdowns and the Settings page.
//
// Groups were a fixed list baked into the template even though storage has
// always accepted any value, so a user could not name a group after their own
// kit. The list now lives in the scan state (nil = the built-in defaults) and is
// edited here. A rename rewrites the group on every device that used it and a
// delete clears it, so the list and the devices stay consistent.
func (h *Handler) handleGroups(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.success(w, h.groupsView())

	case http.MethodPost:
		var body struct {
			Action  string `json:"action"`
			Name    string `json:"name"`
			NewName string `json:"new_name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			h.fail(w, http.StatusBadRequest, errInvalidBody)
			return
		}

		var err error
		switch body.Action {
		case "add":
			err = h.store.AddGroup(body.Name)
		case "rename":
			err = h.store.RenameGroup(body.Name, body.NewName)
		case "delete":
			err = h.store.DeleteGroup(body.Name)
		case "reset":
			err = h.store.SetGroups(nil)
		default:
			h.fail(w, http.StatusBadRequest, invalidAction("add, rename, delete, reset"))
			return
		}
		if err != nil {
			// These are all user-input problems (empty name, duplicate, unknown
			// group), so they are a 400 rather than a server error.
			h.fail(w, http.StatusBadRequest, err)
			return
		}
		h.success(w, h.groupsView())

	default:
		h.error(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// groupsView returns the effective group list, whether it has been customised,
// and how many devices carry each group so the UI can warn before a destructive
// edit.
func (h *Handler) groupsView() map[string]interface{} {
	return map[string]interface{}{
		"groups":     nonNil(h.store.GetGroups()),
		"defaults":   storage.DefaultGroups(),
		"customised": h.store.GroupsCustomised(),
		"counts":     h.store.GroupCounts(),
	}
}

// validateCIDRs checks each entry parses as a CIDR, returning them trimmed and
// de-duplicated, or an error naming the first bad one. Blank entries are dropped.
func validateCIDRs(list []string) ([]string, error) {
	seen := map[string]bool{}
	out := []string{}
	for _, raw := range list {
		c := strings.TrimSpace(raw)
		if c == "" {
			continue
		}
		if _, _, err := net.ParseCIDR(c); err != nil {
			return nil, invalidNetwork(c)
		}
		if !seen[c] {
			seen[c] = true
			out = append(out, c)
		}
	}
	return out, nil
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

// handleSnapshots handles GET/POST/DELETE /api/snapshots: list saved snapshots,
// save the current device list as a new one, or delete one by id.
func (h *Handler) handleSnapshots(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		metas, err := h.store.ListSnapshots()
		if err != nil {
			h.error(w, http.StatusInternalServerError, "could not list snapshots: "+err.Error())
			return
		}
		h.success(w, metas)

	case http.MethodPost:
		var body struct {
			Name string `json:"name"`
		}
		// An empty body is fine; the name then defaults to the date and time.
		_ = json.NewDecoder(r.Body).Decode(&body)
		meta, err := h.store.SaveSnapshot(body.Name)
		if err != nil {
			h.error(w, http.StatusInternalServerError, "could not save snapshot: "+err.Error())
			return
		}
		h.success(w, meta)

	case http.MethodDelete:
		id := r.URL.Query().Get("id")
		if id == "" {
			h.fail(w, http.StatusBadRequest, uerr.New("id.required", "id parameter required"))
			return
		}
		if err := h.store.DeleteSnapshot(id); err != nil {
			h.fail(w, http.StatusNotFound, err)
			return
		}
		h.success(w, map[string]string{"message": "snapshot deleted"})

	default:
		h.error(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// handleSnapshotCompare handles GET /api/snapshots/compare?from=<id>&to=<id|current>,
// reporting what appeared, disappeared, and changed from the earlier list to the
// later one. "to=current" compares against the live device list.
func (h *Handler) handleSnapshotCompare(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.error(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	from := r.URL.Query().Get("from")
	to := r.URL.Query().Get("to")
	if from == "" || to == "" {
		h.fail(w, http.StatusBadRequest, uerr.New("compare.required", "from and to parameters are required"))
		return
	}

	fromSnap, err := h.store.GetSnapshot(from)
	if err != nil {
		h.error(w, http.StatusNotFound, "'from' snapshot not found")
		return
	}

	var after []types.Device
	if to == "current" {
		for _, d := range h.store.GetDevices() {
			after = append(after, *d)
		}
	} else {
		toSnap, err := h.store.GetSnapshot(to)
		if err != nil {
			h.error(w, http.StatusNotFound, "'to' snapshot not found")
			return
		}
		after = toSnap.Devices
	}

	h.success(w, snapshot.Compare(fromSnap.Devices, after))
}

// handleBackup handles GET /api/backup, sending the whole device list, settings
// overrides, and snapshots as a single downloadable JSON file. The password is
// not included.
func (h *Handler) handleBackup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.error(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	data, err := json.MarshalIndent(h.store.ExportBackup(), "", "  ")
	if err != nil {
		h.error(w, http.StatusInternalServerError, "could not build backup")
		return
	}
	filename := "lan-orangutan-backup-" + time.Now().Format("2006-01-02") + ".json"
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)
	_, _ = w.Write(data)
}

// handleBackupRestore handles POST /api/backup/restore, replacing the current
// devices, settings and snapshots with those in the uploaded backup. It is
// destructive; the UI confirms first.
func (h *Handler) handleBackupRestore(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		h.error(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var backup types.Backup
	if err := json.NewDecoder(r.Body).Decode(&backup); err != nil {
		h.fail(w, http.StatusBadRequest, uerr.New("backup.invalid", "that is not a valid backup file"))
		return
	}
	// A real backup always carries a creation time. Requiring it guards against
	// restoring some unrelated JSON file and silently wiping everything.
	if backup.CreatedAt.IsZero() {
		h.fail(w, http.StatusBadRequest, uerr.New("backup.notOrangutan", "that file does not look like a LAN Orangutan backup"))
		return
	}
	if err := h.store.ImportBackup(backup); err != nil {
		h.error(w, http.StatusInternalServerError, "could not restore: "+err.Error())
		return
	}
	// Re-apply the settings that are pushed into live components (the storage
	// limits and the scanner's service-detection flag) from the restored state.
	if err := h.store.SetLimits(
		h.store.RetentionDaysOrDefault(h.cfg.Storage.RetentionDays),
		h.store.MaxDevicesOrDefault(h.cfg.Storage.MaxDevices),
	); err != nil {
		h.error(w, http.StatusInternalServerError, "restored, but could not apply limits: "+err.Error())
		return
	}
	h.scanner.SetServiceDetection(h.store.ServiceDetectionEnabled(h.cfg.Scanning.EnableServiceDetection))
	h.success(w, map[string]string{"message": "backup restored"})
}

// applyLiveSettings pushes the settings that are held by live components (the
// storage limits and the scanner's service-detection flag) from the saved
// state, so a change made by another process takes effect without a restart.
func (h *Handler) applyLiveSettings() {
	if err := h.store.SetLimits(
		h.store.RetentionDaysOrDefault(h.cfg.Storage.RetentionDays),
		h.store.MaxDevicesOrDefault(h.cfg.Storage.MaxDevices),
	); err != nil {
		log.Printf("could not apply storage limits: %v", err)
	}
	h.scanner.SetServiceDetection(h.store.ServiceDetectionEnabled(h.cfg.Scanning.EnableServiceDetection))
}

// handleReport handles GET /api/report, rendering a self-contained HTML summary
// of the network as a single downloadable file to save or share.
func (h *Handler) handleReport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.error(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	networkCount := 0
	if nets, err := network.DetectNetworks(); err == nil {
		networkCount = len(network.WithConfigured(nets, h.networkFilter()))
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="lan-orangutan-report-`+time.Now().Format("2006-01-02")+`.html"`)
	// The download is written in the visitor's language, like the page it was
	// saved from.
	data := report.NewData(h.store.GetDevices(), h.store.GetStats(), networkCount, time.Now())
	data.Lang = i18n.FromRequest(r)
	_ = report.Render(w, data)
}

// scanNetwork scans a single network and merges the results into storage.
func (h *Handler) scanNetwork(ctx context.Context, cidr string) (*types.ScanResult, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	result, err := h.scanner.Scan(ctx, cidr)
	if err != nil {
		return nil, err
	}
	if !result.Success {
		return nil, errors.New(result.Error)
	}

	if err := h.store.MergeScan(cidr, result.Devices); err != nil {
		return nil, errors.New("failed to save devices")
	}
	h.store.SetLastScan(cidr, time.Now())
	// Remember how long this took so the next scan of the same network can show
	// a progress estimate based on real measured time.
	h.store.SetLastDuration(cidr, result.Duration)

	return result, nil
}

// handleTailscale handles GET /api/tailscale
func (h *Handler) handleTailscale(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.error(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	status := network.GetTailscaleStatus()
	h.success(w, status)
}

// handleTailscaleConnect handles POST /api/tailscale/connect.
//
// It is behind the same authentication and CSRF checks as every other mutating
// endpoint. A successful call may still report that the machine needs to sign
// in, in which case the response carries a login URL.
func (h *Handler) handleTailscaleConnect(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		h.error(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	result, err := network.ConnectTailscale()
	if err != nil {
		h.error(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	h.success(w, result)
}

// handleTailscaleDisconnect handles POST /api/tailscale/disconnect.
func (h *Handler) handleTailscaleDisconnect(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		h.error(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	if err := network.DisconnectTailscale(); err != nil {
		h.error(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	h.success(w, map[string]string{"message": "disconnected"})
}

// handleStats handles GET /api/stats
func (h *Handler) handleStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.error(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	stats := h.store.GetStats()
	h.success(w, stats)
}

// handleStatus handles GET /api/status
func (h *Handler) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.error(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	status := map[string]interface{}{
		"server":    "running",
		"timestamp": time.Now().Format(time.RFC3339),
		"config": map[string]interface{}{
			"port":        h.cfg.Server.Port,
			"bind":        h.cfg.Server.BindAddress,
			"api_enabled": h.cfg.Server.EnableAPI,
		},
	}
	h.success(w, status)
}

// Bounds for the editable numeric settings. A value of 0 turns the limit off
// (keep devices forever / no cap); the upper bounds guard against absurd input.
const (
	maxRetentionDays = 36500 // 100 years
	maxMaxDevicesCap = 1000000
)

// handleSettings handles GET/POST /api/settings.
//
// Settings the user edits from the dashboard are stored as overrides in the data
// directory, never written back to config.ini, so a file the user maintains by
// hand is left untouched. GET reports each value, its config default, and whether
// an override is in effect; POST saves overrides and applies them immediately.
// Only the storage limits are editable so far; scan interval and theme are
// reported for information.
func (h *Handler) handleSettings(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.success(w, h.settingsView())

	case http.MethodPost:
		// Both fields are sent every time: a number sets the override, JSON null
		// clears it (falling back to the config default).
		var body struct {
			RetentionDays *int `json:"retention_days"`
			MaxDevices    *int `json:"max_devices"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			h.fail(w, http.StatusBadRequest, errInvalidBody)
			return
		}
		if body.RetentionDays != nil {
			v := clampInt(*body.RetentionDays, 0, maxRetentionDays)
			body.RetentionDays = &v
		}
		if body.MaxDevices != nil {
			v := clampInt(*body.MaxDevices, 0, maxMaxDevicesCap)
			body.MaxDevices = &v
		}
		if err := h.store.SetLimitOverrides(body.RetentionDays, body.MaxDevices); err != nil {
			h.error(w, http.StatusInternalServerError, "could not save settings: "+err.Error())
			return
		}
		// Apply immediately so the new retention window and cap take effect now,
		// not just on the next scan.
		if err := h.store.SetLimits(
			h.store.RetentionDaysOrDefault(h.cfg.Storage.RetentionDays),
			h.store.MaxDevicesOrDefault(h.cfg.Storage.MaxDevices),
		); err != nil {
			h.error(w, http.StatusInternalServerError, "could not apply settings: "+err.Error())
			return
		}
		h.success(w, h.settingsView())

	default:
		h.error(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// settingsView builds the current effective settings for the UI: each editable
// value, its config default, and whether a UI override is in effect.
func (h *Handler) settingsView() map[string]interface{} {
	rOver, mOver := h.store.GetLimitOverrides()
	field := func(override *int, def int) map[string]interface{} {
		return map[string]interface{}{
			"value":      valueOr(override, def),
			"default":    def,
			"overridden": override != nil,
		}
	}
	return map[string]interface{}{
		"retention_days": field(rOver, h.cfg.Storage.RetentionDays),
		"max_devices":    field(mOver, h.cfg.Storage.MaxDevices),
		// Reported for information; changed through /api/scan/interval.
		"scan_interval": h.store.ScanIntervalOrDefault(h.cfg.Scanning.ScanInterval),
		"theme":         h.cfg.UI.Theme,
	}
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func valueOr(p *int, def int) int {
	if p != nil {
		return *p
	}
	return def
}

// networkFilter builds the effective scan-target filter, preferring the UI
// overrides over the configured defaults.
func (h *Handler) networkFilter() network.Filter {
	return network.Filter{
		Configured:     h.store.NetworksOrDefault(h.cfg.Scanning.Networks),
		Excluded:       h.store.ExcludeNetworksOrDefault(h.cfg.Scanning.ExcludeNetworks),
		OnlyConfigured: h.store.OnlyConfiguredOrDefault(h.cfg.Scanning.OnlyConfiguredNetworks),
	}
}

// success sends a successful JSON response
func (h *Handler) success(w http.ResponseWriter, data interface{}) {
	resp := types.APIResponse{
		Success: true,
		Data:    data,
	}
	json.NewEncoder(w).Encode(resp)
}

// error sends an error JSON response
func (h *Handler) error(w http.ResponseWriter, status int, message string) {
	w.WriteHeader(status)
	resp := types.APIResponse{
		Success: false,
		Error:   message,
	}
	json.NewEncoder(w).Encode(resp)
}

// The request problems this package detects itself, with the codes the web
// interface translates (error.<code> in the js-app catalogue). Storage,
// scanner and notify report theirs the same way, so a handler forwards those
// unchanged through fail.
var (
	errInvalidBody     = uerr.New("request.invalid", "invalid request body")
	errIPRequired      = uerr.New("ip.required", "ip parameter required")
	errNetworkRequired = uerr.New("network.required", "network parameter required")
)

func invalidNetwork(value string) error {
	return uerr.Newf("network.invalid", "%q is not a valid network (use CIDR form, e.g. 192.168.1.0/24)", "value", value)
}

func invalidAction(actions string) error {
	return uerr.Newf("action.invalid", "action must be one of: %s", "actions", actions)
}

// inputStatus is the status for an error from a device update: an unknown
// device is 404, every other complaint is about the submitted values.
func inputStatus(err error) int {
	if ue, ok := uerr.Of(err); ok && ue.Code == "device.notFound" {
		return http.StatusNotFound
	}
	return http.StatusBadRequest
}

// fail reports err to the client. A uerr.Error also carries its code and
// placeholders, so the page can translate it; anything else is sent as its
// English text alone.
func (h *Handler) fail(w http.ResponseWriter, status int, err error) {
	resp := types.APIResponse{Success: false, Error: err.Error()}
	if ue, ok := uerr.Of(err); ok {
		resp.Code = ue.Code
		resp.Args = ue.Args
	}
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(resp)
}
