// Package web handles the web UI and static assets
package web

import (
	"embed"
	"fmt"
	"html/template"
	"io/fs"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/291-Group/LAN-Orangutan/internal/config"
	"github.com/291-Group/LAN-Orangutan/internal/network"
	"github.com/291-Group/LAN-Orangutan/internal/storage"
	"github.com/291-Group/LAN-Orangutan/internal/types"
)

//go:embed static/*
var staticFS embed.FS

//go:embed templates/*
var templateFS embed.FS

// Handler handles web requests
type Handler struct {
	store     *storage.Storage
	cfg       *config.Config
	templates *template.Template
	staticFS  http.Handler
}

// PageData holds data passed to templates
type PageData struct {
	Title        string
	Theme        string
	Devices      []*DeviceView
	Networks     []types.Network
	Tailscale    types.TailscaleStatus
	Stats        types.DeviceStats
	Groups       []string
	CurrentGroup string
	Error        string
}

// DeviceView is a device with computed display properties
type DeviceView struct {
	*types.Device
	Status      string
	StatusClass string
	TimeAgo     string
}

// NewHandler creates a new web handler
func NewHandler(store *storage.Storage, cfg *config.Config) *Handler {
	// Parse templates with custom functions
	funcMap := template.FuncMap{
		"timeAgo": timeAgo,
		"lower":   strings.ToLower,
	}

	tmpl := template.Must(template.New("").Funcs(funcMap).ParseFS(templateFS, "templates/*.html"))

	// Create static file server
	staticSub, _ := fs.Sub(staticFS, "static")
	staticHandler := http.StripPrefix("/static/", http.FileServer(http.FS(staticSub)))

	return &Handler{
		store:     store,
		cfg:       cfg,
		templates: tmpl,
		staticFS:  staticHandler,
	}
}

// ServeHTTP implements http.Handler
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path

	// Static files
	if strings.HasPrefix(path, "/static/") {
		h.staticFS.ServeHTTP(w, r)
		return
	}

	// Routes
	switch path {
	case "/", "/index.html":
		h.handleIndex(w, r)
	case "/settings", "/settings.html":
		h.handleSettings(w, r)
	default:
		http.NotFound(w, r)
	}
}

// handleIndex renders the main dashboard
func (h *Handler) handleIndex(w http.ResponseWriter, r *http.Request) {
	// Get devices
	devices := h.store.GetDevices()

	// Convert to view models
	var deviceViews []*DeviceView
	groupSet := make(map[string]bool)

	for _, d := range devices {
		dv := &DeviceView{
			Device:  d,
			TimeAgo: timeAgo(d.LastSeen),
		}

		if d.IsRecent() {
			dv.Status = "online"
			dv.StatusClass = "status-online"
		} else if d.IsOnline() {
			dv.Status = "seen"
			dv.StatusClass = "status-seen"
		} else {
			dv.Status = "offline"
			dv.StatusClass = "status-offline"
		}

		deviceViews = append(deviceViews, dv)

		if d.Group != "" {
			groupSet[d.Group] = true
		}
	}

	// Sort by IP
	sort.Slice(deviceViews, func(i, j int) bool {
		return ipToLong(deviceViews[i].IP) < ipToLong(deviceViews[j].IP)
	})

	// Get groups
	var groups []string
	for g := range groupSet {
		groups = append(groups, g)
	}
	sort.Strings(groups)

	// Get networks
	networks, _ := network.DetectNetworks()

	// Get Tailscale status
	tailscale := network.GetTailscaleStatus()

	// Get stats
	stats := h.store.GetStats()

	data := PageData{
		Title:     "LAN Orangutan",
		Theme:     h.cfg.UI.Theme,
		Devices:   deviceViews,
		Networks:  networks,
		Tailscale: tailscale,
		Stats:     stats,
		Groups:    groups,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := h.templates.ExecuteTemplate(w, "index.html", data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// handleSettings renders the settings page
func (h *Handler) handleSettings(w http.ResponseWriter, r *http.Request) {
	// Get Tailscale status
	tailscale := network.GetTailscaleStatus()

	// Get stats
	stats := h.store.GetStats()

	data := PageData{
		Title:     "Settings - LAN Orangutan",
		Theme:     h.cfg.UI.Theme,
		Tailscale: tailscale,
		Stats:     stats,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := h.templates.ExecuteTemplate(w, "settings.html", data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// timeAgo returns a human-readable time difference
func timeAgo(t time.Time) string {
	if t.IsZero() {
		return "never"
	}

	diff := time.Since(t)

	switch {
	case diff < time.Minute:
		return "just now"
	case diff < time.Hour:
		mins := int(diff.Minutes())
		if mins == 1 {
			return "1 min ago"
		}
		return fmt.Sprintf("%d min ago", mins)
	case diff < 24*time.Hour:
		hours := int(diff.Hours())
		if hours == 1 {
			return "1 hr ago"
		}
		return fmt.Sprintf("%d hr ago", hours)
	case diff < 7*24*time.Hour:
		days := int(diff.Hours() / 24)
		if days == 1 {
			return "1 day ago"
		}
		return fmt.Sprintf("%d days ago", days)
	default:
		return t.Format("Jan 2, 2006")
	}
}

// ipToLong converts an IP string to a sortable integer
func ipToLong(ip string) int64 {
	parts := strings.Split(ip, ".")
	if len(parts) != 4 {
		return 0x7FFFFFFFFFFFFFFF
	}

	var result int64
	for i, p := range parts {
		var n int
		for _, c := range p {
			if c >= '0' && c <= '9' {
				n = n*10 + int(c-'0')
			}
		}
		result |= int64(n) << (24 - 8*i)
	}
	return result
}
