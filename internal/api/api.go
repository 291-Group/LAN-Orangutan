// Package api implements the REST API endpoints
package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
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
	h.success(w, devices)
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
	h.success(w, networks)
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
