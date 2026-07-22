// Package api implements the REST API endpoints
package api

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/291-Group/LAN-Orangutan/internal/config"
	"github.com/291-Group/LAN-Orangutan/internal/network"
	"github.com/291-Group/LAN-Orangutan/internal/scanner"
	"github.com/291-Group/LAN-Orangutan/internal/storage"
	"github.com/291-Group/LAN-Orangutan/internal/types"
)

// Handler handles API requests
type Handler struct {
	store   *storage.Storage
	cfg     *config.Config
	scanner *scanner.Scanner

	// jobMu guards job, which holds the most recent background scan. Only one
	// scan runs at a time.
	jobMu sync.Mutex
	job   *scanJob
}

// NewHandler creates a new API handler
func NewHandler(store *storage.Storage, cfg *config.Config) *Handler {
	return &Handler{
		store:   store,
		cfg:     cfg,
		scanner: scanner.New(cfg.Scanning.MinScanInterval),
	}
}

// ServeHTTP implements http.Handler
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Set JSON content type for all API responses
	w.Header().Set("Content-Type", "application/json")

	// CSRF protection for mutating requests
	if r.Method == http.MethodPost || r.Method == http.MethodPut || r.Method == http.MethodDelete {
		contentType := r.Header.Get("Content-Type")
		xRequestedWith := r.Header.Get("X-Requested-With")

		if !strings.Contains(contentType, "application/json") && xRequestedWith != "XMLHttpRequest" {
			h.error(w, http.StatusForbidden, "CSRF protection: requires Content-Type: application/json or X-Requested-With header")
			return
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
	case path == "networks":
		h.handleNetworks(w, r)
	case path == "scan":
		h.handleScan(w, r)
	case path == "scan/start":
		h.handleScanStart(w, r)
	case path == "scan/progress":
		h.handleScanProgress(w, r)
	case path == "scan/cancel":
		h.handleScanCancel(w, r)
	case path == "tailscale":
		h.handleTailscale(w, r)
	case path == "stats":
		h.handleStats(w, r)
	case path == "status":
		h.handleStatus(w, r)
	case path == "settings":
		h.handleSettings(w, r)
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

	h.success(w, devices)
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
		"Notes", "Group", "First Seen", "Last Seen", "Status",
	})

	for _, d := range sorted {
		status := "offline"
		if d.IsOnline() {
			status = "online"
		}
		_ = cw.Write([]string{
			d.IP, d.MAC, d.Hostname, scanner.ResolveVendor(d.Vendor, d.MAC), d.Label, d.Notes, d.Group,
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
			h.error(w, http.StatusBadRequest, "ip parameter required")
			return
		}
		device := h.store.GetDevice(ip)
		if device == nil {
			h.error(w, http.StatusNotFound, "device not found")
			return
		}
		h.success(w, device)

	case http.MethodPost:
		var req struct {
			IP    string  `json:"ip"`
			Label *string `json:"label"`
			Notes *string `json:"notes"`
			Group *string `json:"group"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			h.error(w, http.StatusBadRequest, "invalid JSON")
			return
		}

		// Get IP from query if not in body
		ip := req.IP
		if ip == "" {
			ip = r.URL.Query().Get("ip")
		}
		if ip == "" {
			h.error(w, http.StatusBadRequest, "ip required")
			return
		}

		if err := h.store.UpdateDeviceFields(ip, req.Label, req.Notes, req.Group); err != nil {
			h.error(w, http.StatusNotFound, err.Error())
			return
		}
		h.success(w, map[string]string{"message": "device updated"})

	case http.MethodDelete:
		ip := r.URL.Query().Get("ip")
		if ip == "" {
			h.error(w, http.StatusBadRequest, "ip parameter required")
			return
		}
		if err := h.store.DeleteDevice(ip); err != nil {
			h.error(w, http.StatusNotFound, err.Error())
			return
		}
		h.success(w, map[string]string{"message": "device deleted"})

	default:
		h.error(w, http.StatusMethodNotAllowed, "method not allowed")
	}
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
	h.success(w, network.WithConfigured(networks, h.cfg.Scanning.Networks))
}

// handleScan handles GET /api/scan
func (h *Handler) handleScan(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.error(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	cidr := r.URL.Query().Get("network")
	if cidr == "" {
		h.error(w, http.StatusBadRequest, "network parameter required")
		return
	}

	// "all" scans every detected network, matching the CLI's behaviour
	if strings.EqualFold(cidr, "all") {
		h.scanAllNetworks(w, r)
		return
	}

	// Validate CIDR
	if !network.ValidateCIDR(cidr) {
		h.error(w, http.StatusBadRequest, "invalid CIDR format")
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
	if err := h.store.MergeDevices(result.Devices); err != nil {
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
	detected = network.WithConfigured(detected, h.cfg.Scanning.Networks)
	if err != nil {
		h.error(w, http.StatusInternalServerError, "failed to detect networks: "+err.Error())
		return
	}
	if len(detected) == 0 {
		h.error(w, http.StatusNotFound, "no networks detected")
		return
	}

	result := scanAllResult{
		Networks:     make([]networkScanSummary, 0, len(detected)),
		NetworkCount: len(detected),
		Timestamp:    time.Now(),
	}

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
			return nil, errors.New("invalid CIDR format")
		}
		return []string{cidr}, nil
	}

	detected, err := network.DetectNetworks()
	detected = network.WithConfigured(detected, h.cfg.Scanning.Networks)
	if err != nil {
		return nil, errors.New("failed to detect networks: " + err.Error())
	}
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
		h.error(w, http.StatusBadRequest, "network parameter required")
		return
	}

	networks, err := h.resolveScanTargets(cidr)
	if err != nil {
		h.error(w, http.StatusBadRequest, err.Error())
		return
	}

	h.jobMu.Lock()
	defer h.jobMu.Unlock()

	if h.job != nil && h.job.isRunning() {
		h.error(w, http.StatusConflict, "a scan is already running")
		return
	}

	h.job = h.startScanJob(networks)
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

	if err := h.store.MergeDevices(result.Devices); err != nil {
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

// handleSettings handles GET/POST /api/settings
func (h *Handler) handleSettings(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		settings := map[string]interface{}{
			"theme":          h.cfg.UI.Theme,
			"scan_interval":  h.cfg.Scanning.ScanInterval,
			"retention_days": h.cfg.Storage.RetentionDays,
		}
		h.success(w, settings)

	case http.MethodPost:
		// Settings update would require config file write
		// For now, return not implemented
		h.error(w, http.StatusNotImplemented, "settings update not yet implemented")

	default:
		h.error(w, http.StatusMethodNotAllowed, "method not allowed")
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
