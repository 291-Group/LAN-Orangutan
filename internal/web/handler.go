// Package web handles the web UI and static assets
package web

import (
	"bytes"
	"embed"
	"fmt"
	"html/template"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/291-Group/LAN-Orangutan/internal/auth"
	"github.com/291-Group/LAN-Orangutan/internal/config"
	"github.com/291-Group/LAN-Orangutan/internal/i18n"
	"github.com/291-Group/LAN-Orangutan/internal/network"
	"github.com/291-Group/LAN-Orangutan/internal/scanner"
	"github.com/291-Group/LAN-Orangutan/internal/storage"
	"github.com/291-Group/LAN-Orangutan/internal/types"
	"github.com/291-Group/LAN-Orangutan/internal/uerr"
)

//go:embed static/*
var staticFS embed.FS

//go:embed templates/*
var templateFS embed.FS

// LogoSVG returns the LAN Orangutan logo, for output that has to carry it
// itself rather than load it from /static (the self-contained HTML report).
func LogoSVG() []byte {
	data, _ := staticFS.ReadFile("static/orangutan.svg")
	return data
}

// Handler handles web requests
type Handler struct {
	store     *storage.Storage
	cfg       *config.Config
	auth      *auth.Authenticator
	version   string
	templates *template.Template
	staticFS  http.Handler
}

// networkFilter builds the effective scan-target filter, preferring the UI
// overrides over the configured defaults, so the dashboard shows the same
// networks a scan will actually use.
func (h *Handler) networkFilter() network.Filter {
	return network.Filter{
		Configured:     h.store.NetworksOrDefault(h.cfg.Scanning.Networks),
		Excluded:       h.store.ExcludeNetworksOrDefault(h.cfg.Scanning.ExcludeNetworks),
		OnlyConfigured: h.store.OnlyConfiguredOrDefault(h.cfg.Scanning.OnlyConfiguredNetworks),
	}
}

// PageData holds data passed to templates
type PageData struct {
	// Lang is the visitor's language and Dir its writing direction ("ltr" or
	// "rtl"); Languages feeds the language picker and I18N carries the strings
	// the page scripts need, already in Lang. Filled by withLang.
	Lang      string
	Dir       string
	Languages []i18n.Language
	I18N      map[string]string

	Title        string
	Theme        string
	Version      string
	Devices      []*DeviceView
	Networks     []types.Network
	Tailscale    types.TailscaleStatus
	Stats        types.DeviceStats
	Groups       []string
	Locations    []string
	CurrentGroup string
	Types        []string
	Error        string

	// AuthEnabled reports whether a password is configured, so pages can show
	// a sign out link only when there is a session to end.
	AuthEnabled bool

	// TokenAccessOff reports that API tokens are refused (enable_api is off),
	// so the Settings page can say so instead of offering to create one.
	TokenAccessOff bool

	// MinPasswordLength is shown on the setup page and enforced by the browser.
	MinPasswordLength int

	// LastScanAgo and LastScanAt describe when the device list was last
	// refreshed. Both are empty when nothing has been scanned yet.
	LastScanAgo string
	LastScanAt  string

	// NetworkWarning explains that this instance cannot see the local network,
	// which happens in a container without host networking. Empty when fine.
	NetworkWarning string

	// LastScanUnix lets the page keep the relative time ticking without a
	// reload. A server-rendered "1 min ago" would otherwise still claim one
	// minute an hour later, which is the very confusion the footer exists to
	// prevent.
	LastScanUnix int64

	// Flagged is how many devices expose a risky service, and Moved how many
	// have changed IP. They fill the dashboard summary strip with information
	// worth glancing at rather than empty space.
	Flagged int
	Moved   int
	// Unexpected is how many devices are online but not marked expected, and
	// Missing how many expected devices are currently offline. Both are zero until
	// the user baselines their network by marking devices as expected.
	Unexpected int
	Missing    int

	// ContinuousScan reports whether the server re-scans on its own. When true
	// the dashboard shows a "live" cue so the last-scan time does not read as
	// stale data when it is actually being kept current.
	ContinuousScan bool
}

// DeviceView is a device with computed display properties
type DeviceView struct {
	*types.Device
	Status       string
	StatusClass  string
	TimeAgo      string
	LastSeenUnix int64

	// Vendor shadows the stored value so it can be resolved for records that
	// predate the built-in manufacturer database.
	Vendor string

	// Type shadows the stored value so a device recorded before classification
	// existed still shows a type, inferred from its vendor and hostname now.
	Type string
}

// NewHandler creates a new web handler
func NewHandler(store *storage.Storage, cfg *config.Config, authn *auth.Authenticator, version string) *Handler {
	// Parse templates with custom functions
	funcMap := template.FuncMap{
		"timeAgo":  timeAgo,
		"lower":    strings.ToLower,
		"typeIcon": typeIcon,
		// t and tn translate: {{t $.Lang "common.settings"}}, with placeholder
		// pairs after the key, and {{tn $.Lang "common.time.dayAgo" .Days}} for
		// text that changes with a number.
		"t":  i18n.T,
		"tn": i18n.N,
		// th is t for the few strings whose placeholders take markup (a <kbd>,
		// a <code>); see th below. raw marks a literal snippet written in the
		// template itself as that markup.
		"th":  th,
		"raw": func(s string) template.HTML { return template.HTML(s) },
		// tparts splits a translated string at one placeholder, so a template
		// can put its own markup there: {{$p := tparts $.Lang "x" "when"}}
		// {{index $p 0}}<span>...</span>{{index $p 1}}. The text on either side
		// is then escaped like any other value.
		"tparts": func(lang, key, placeholder string) []string {
			before, after, _ := strings.Cut(i18n.T(lang, key), "{"+placeholder+"}")
			return []string{before, after}
		},
		// tailscaleStatus shows Tailscale's state in the visitor's language.
		"tailscaleStatus": func(lang string, ts types.TailscaleStatus) string {
			if k := ts.StatusKey(); k != "" {
				return i18n.T(lang, "common.tailscale."+k)
			}
			return ts.StatusLabel()
		},
		// riskText translates a security finding. Findings are stored as English
		// text in the device record, so known ones are matched back to a key.
		"riskText": func(lang, risk string) string {
			if k, ok := riskKeys[risk]; ok {
				return i18n.T(lang, k)
			}
			return risk
		},
		// langPicker hands the picker partial what it needs from any page.
		"langPicker": func(p any, compact bool) map[string]any {
			d := pageDataOf(p)
			return map[string]any{"Lang": d.Lang, "Languages": d.Languages, "Compact": compact}
		},
		// typeName shows a device type in the visitor's language. The stored
		// value stays English, since it is data that filters and the API use.
		"typeName": func(lang, deviceType string) string {
			if deviceType == "" {
				return ""
			}
			if s := i18n.T(lang, "common.type."+deviceType); s != "common.type."+deviceType {
				return s
			}
			return deviceType
		},
	}

	tmpl := template.Must(template.New("").Funcs(funcMap).ParseFS(templateFS, "templates/*.html"))

	// Create static file server
	staticSub, _ := staticSubFS()
	staticHandler := newStaticHandler(staticSub)

	return &Handler{
		store:     store,
		cfg:       cfg,
		auth:      authn,
		version:   version,
		templates: tmpl,
		staticFS:  staticHandler,
	}
}

// StaticHandler serves the embedded static assets.
//
// Exposed separately so the server can leave static files unauthenticated,
// which the login page needs in order to style itself.
func (h *Handler) StaticHandler() http.Handler {
	return h.staticFS
}

// HandleSetup renders the first run page and creates the initial password.
//
// onPasswordSet persists the new hash. It is supplied by the caller so this
// package does not need to know where credentials live.
func (h *Handler) HandleSetup(onPasswordSet func(hash string) error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Once a password exists there is nothing to set up, and this page must
		// not become a way to overwrite it.
		if !h.auth.NeedsSetup() {
			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		}

		switch r.Method {
		case http.MethodGet:
			h.renderSetup(w, r, "", http.StatusOK)

		case http.MethodPost:
			lang := i18n.FromRequest(r)
			if err := r.ParseForm(); err != nil {
				h.renderSetup(w, r, i18n.T(lang, "auth.unreadableSubmission"), http.StatusBadRequest)
				return
			}

			password := r.FormValue("password")
			if password != r.FormValue("confirm") {
				h.renderSetup(w, r, i18n.T(lang, "auth.passwordsDoNotMatch"), http.StatusBadRequest)
				return
			}

			if err := auth.ValidatePassword(password); err != nil {
				h.renderSetup(w, r, passwordError(lang, err), http.StatusBadRequest)
				return
			}

			hash, err := h.auth.SetPassword(password)
			if err != nil {
				h.renderSetup(w, r, passwordError(lang, err), http.StatusBadRequest)
				return
			}

			if err := onPasswordSet(hash); err != nil {
				// The password is live in memory but could not be saved, so it
				// would vanish on restart. Say so rather than pretend.
				h.renderSetup(w, r,
					i18n.T(lang, "auth.passwordNotSaved", "error", err.Error()),
					http.StatusInternalServerError)
				return
			}

			// Sign the user straight in rather than bouncing them to a login
			// form for the password they just typed twice.
			if token, ok := h.auth.StartSession(); ok {
				h.auth.SetCookie(w, r, token)
			}
			http.Redirect(w, r, "/", http.StatusSeeOther)

		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}
}

// th is t for the few strings whose placeholders take markup of their own (a
// <kbd>, a <code>, a <span> with an id). The catalogue text is compiled in,
// never user input, but a placeholder value might not be, so every argument
// is escaped unless the template marked it as markup with raw.
func th(lang, key string, args ...any) template.HTML {
	safe := make([]any, len(args))
	for i, a := range args {
		switch v := a.(type) {
		case template.HTML:
			safe[i] = string(v)
		case string:
			if i%2 == 0 {
				safe[i] = v // a placeholder name
			} else {
				safe[i] = template.HTMLEscapeString(v)
			}
		default:
			safe[i] = template.HTMLEscapeString(fmt.Sprint(v))
		}
	}
	return template.HTML(i18n.T(lang, key, safe...))
}

// passwordError shows why auth.ValidatePassword turned a password down, in the
// visitor's language. The error names the rule that failed and the limit it
// applied, so the message is translated from those; anything else comes back
// as the error's own text.
func passwordError(lang string, err error) string {
	ue, ok := uerr.Of(err)
	if !ok {
		return err.Error()
	}
	switch ue.Code {
	case "password.tooShort":
		return i18n.N(lang, "auth.passwordTooShort", intArg(ue.Args["min"], auth.MinPasswordLength))
	case "password.tooLong":
		return i18n.N(lang, "auth.passwordTooLong", intArg(ue.Args["max"], auth.MaxPasswordBytes))
	}
	if key := "js-app.app.error." + ue.Code; i18n.T(i18n.Default, key) != key {
		args := make([]any, 0, 2*len(ue.Args))
		for k, v := range ue.Args {
			args = append(args, k, v)
		}
		return i18n.T(lang, key, args...)
	}
	return err.Error()
}

// intArg reads a number out of an error's placeholders, or falls back.
func intArg(v any, fallback int) int {
	if n, ok := v.(int); ok {
		return n
	}
	return fallback
}

// renderSetup draws the first run page, optionally with an error message.
func (h *Handler) renderSetup(w http.ResponseWriter, r *http.Request, message string, status int) {
	data := PageData{
		Title:             "Welcome - LAN Orangutan",
		Theme:             h.cfg.UI.Theme,
		Version:           h.version,
		Error:             message,
		MinPasswordLength: auth.MinPasswordLength,
	}
	withLang(r, &data)

	var buf bytes.Buffer
	if err := h.templates.ExecuteTemplate(&buf, "setup.html", data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	buf.WriteTo(w)
}

// HandleLogin renders the sign in form and processes submissions.
func (h *Handler) HandleLogin(w http.ResponseWriter, r *http.Request) {
	// Send first run visitors to setup rather than a login form for a password
	// that does not exist yet.
	if h.auth.NeedsSetup() {
		http.Redirect(w, r, auth.SetupPath, http.StatusSeeOther)
		return
	}

	// With no password configured there is nothing to sign in to.
	if !h.auth.Enabled() {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	switch r.Method {
	case http.MethodGet:
		h.renderLogin(w, r, "")

	case http.MethodPost:
		if err := r.ParseForm(); err != nil {
			h.renderLogin(w, r, i18n.T(i18n.FromRequest(r), "auth.unreadableSubmission"))
			return
		}

		if h.auth.LockedOut(r.RemoteAddr) {
			h.renderLogin(w, r, i18n.T(i18n.FromRequest(r), "auth.tooManyAttempts"))
			return
		}

		token, ok := h.auth.Login(r.RemoteAddr, r.FormValue("password"))
		if !ok {
			h.renderLogin(w, r, i18n.T(i18n.FromRequest(r), "auth.incorrectPassword"))
			return
		}

		h.auth.SetCookie(w, r, token)
		http.Redirect(w, r, "/", http.StatusSeeOther)

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// HandleLogout ends the current session.
func (h *Handler) HandleLogout(w http.ResponseWriter, r *http.Request) {
	h.auth.Logout(r)
	h.auth.ClearCookie(w, r)
	http.Redirect(w, r, auth.LoginPath, http.StatusSeeOther)
}

// renderLogin draws the sign in page, optionally with an error message.
func (h *Handler) renderLogin(w http.ResponseWriter, r *http.Request, message string) {
	data := PageData{
		Title:   "Sign in - LAN Orangutan",
		Theme:   h.cfg.UI.Theme,
		Version: h.version,
		Error:   message,
	}
	withLang(r, &data)

	var buf bytes.Buffer
	if err := h.templates.ExecuteTemplate(&buf, "login.html", data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	// A failed attempt is not a successful page load; say so for any client
	// that pays attention to the status code.
	if message != "" {
		w.WriteHeader(http.StatusUnauthorized)
	}
	buf.WriteTo(w)
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
	case "/credits":
		h.handleCredits(w, r)
	default:
		http.NotFound(w, r)
	}
}

// handleIndex renders the main dashboard
func (h *Handler) handleIndex(w http.ResponseWriter, r *http.Request) {
	lang := i18n.FromRequest(r)

	// Get devices
	devices := h.store.GetDevices()

	// Convert to view models
	var deviceViews []*DeviceView
	var flagged, moved, unexpected, missing int

	for _, d := range devices {
		if len(d.Risks) > 0 {
			flagged++
		}
		if len(d.AddressHistory) > 0 {
			moved++
		}
		if d.IsOnline() && !d.Expected {
			unexpected++
		}
		if d.Expected && !d.IsOnline() {
			missing++
		}
		// Devices recorded by an older version have no vendor stored, so look it
		// up now rather than showing "Unknown" until a rescan.
		resolvedVendor := displayVendor(scanner.ResolveVendor(d.Vendor, d.MAC))

		dv := &DeviceView{
			Device:       d,
			TimeAgo:      timeAgo(lang, d.LastSeen),
			LastSeenUnix: d.LastSeen.Unix(),
			Vendor:       resolvedVendor,
			Type:         d.Type,
		}
		// Backfill a type for records that predate classification, from the same
		// vendor and hostname the row already shows. Port evidence is not
		// available here, so this is the default path, not the opt-in probe.
		if dv.Type == "" {
			dv.Type = scanner.Classify(resolvedVendor, d.Hostname, nil)
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
	}

	// Sort by IP
	sort.Slice(deviceViews, func(i, j int) bool {
		return ipToLong(deviceViews[i].IP) < ipToLong(deviceViews[j].IP)
	})

	// The group list the dropdowns are built from: the user's own list if they
	// have customised it, otherwise the built-in defaults. GetGroups already
	// folds in any group a device carries that the list is missing, so this
	// covers everything in use as well as everything on offer. Declaration order
	// is kept, so a user's ordering is not re-sorted out from under them.
	groups := h.store.GetGroups()

	// Standard taxonomy types for device type selection
	typesList := scanner.KnownTypes

	// Get networks
	networks, _ := network.DetectNetworks()
	networks = network.WithConfigured(networks, h.networkFilter())

	// Get Tailscale status
	tailscale := network.GetTailscaleStatus()

	// Get stats
	stats := h.store.GetStats()

	data := PageData{
		Title:          "LAN Orangutan",
		Theme:          h.cfg.UI.Theme,
		Devices:        deviceViews,
		Networks:       networks,
		Tailscale:      tailscale,
		Stats:          stats,
		Groups:         groups,
		Locations:      h.store.GetLocations(),
		Types:          typesList,
		AuthEnabled:    h.auth.Enabled(),
		Flagged:        flagged,
		Moved:          moved,
		Unexpected:     unexpected,
		Missing:        missing,
		Version:        h.version,
		ContinuousScan: h.store.ContinuousScanEnabled(h.cfg.Scanning.ContinuousScan),
	}
	withLang(r, &data)

	if network.IsolationWarning(networks) != "" {
		data.NetworkWarning = i18n.T(data.Lang, "common.networkIsolation")
	}

	// The table is only as current as the last scan. Say so, so that a "last
	// seen" time is read against when the data was actually gathered.
	if lastScan := h.store.GetMostRecentScan(); !lastScan.IsZero() {
		data.LastScanAgo = timeAgo(data.Lang, lastScan)
		if data.Lang == i18n.Default {
			data.LastScanAt = lastScan.Format("Jan 2, 2006 at 3:04 PM")
		} else {
			data.LastScanAt = lastScan.Format("2006-01-02 15:04")
		}
		data.LastScanUnix = lastScan.Unix()
	}

	// Buffer the template output to avoid superfluous WriteHeader on error
	var buf bytes.Buffer
	if err := h.templates.ExecuteTemplate(&buf, "index.html", data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	buf.WriteTo(w)
}

// handleSettings renders the settings page
func (h *Handler) handleSettings(w http.ResponseWriter, r *http.Request) {
	// Get Tailscale status
	tailscale := network.GetTailscaleStatus()

	// Get stats
	stats := h.store.GetStats()

	data := PageData{
		Title:          "Settings - LAN Orangutan",
		Theme:          h.cfg.UI.Theme,
		Version:        h.version,
		Tailscale:      tailscale,
		Stats:          stats,
		AuthEnabled:    h.auth.Enabled(),
		TokenAccessOff: !h.auth.TokenAccessEnabled(),
	}
	withLang(r, &data)

	// Buffer the template output to avoid superfluous WriteHeader on error
	var buf bytes.Buffer
	if err := h.templates.ExecuteTemplate(&buf, "settings.html", data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	buf.WriteTo(w)
}

// timeAgo returns a human-readable time difference in lang.
func timeAgo(lang string, t time.Time) string {
	if t.IsZero() {
		return i18n.T(lang, "common.time.never")
	}

	diff := time.Since(t)

	switch {
	case diff < time.Minute:
		return i18n.T(lang, "common.time.justNow")
	case diff < time.Hour:
		return i18n.N(lang, "common.time.minAgo", int(diff.Minutes()))
	case diff < 24*time.Hour:
		return i18n.N(lang, "common.time.hrAgo", int(diff.Hours()))
	case diff < 7*24*time.Hour:
		return i18n.N(lang, "common.time.dayAgo", int(diff.Hours()/24))
	default:
		// Month names would need translating too; a numeric date reads the
		// same everywhere.
		if lang == i18n.Default {
			return t.Format("Jan 2, 2006")
		}
		return t.Format("2006-01-02")
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

// osNames spells the operating systems Tailscale reports for its peers the
// way their makers do. Tailscale gives them in lower case ("linux", "iOS" as
// "ios"), and the vendor column shows them beside proper manufacturer names.
var osNames = map[string]string{
	"linux":   "Linux",
	"windows": "Windows",
	"macos":   "macOS",
	"ios":     "iOS",
	"android": "Android",
	"freebsd": "FreeBSD",
	"openbsd": "OpenBSD",
	"tvos":    "tvOS",
}

// displayVendor returns vendor as the dashboard shows it.
func displayVendor(vendor string) string {
	if name, ok := osNames[strings.ToLower(vendor)]; ok {
		return name
	}
	return vendor
}

// withLang fills the language fields of a page from the visitor's choice.
func withLang(r *http.Request, d *PageData) {
	lang := i18n.FromRequest(r)
	d.Lang = lang
	d.Dir = i18n.Get(lang).Dir
	d.Languages = i18n.Languages
	d.I18N = i18n.Script(lang)
}

// pageDataOf finds the PageData in whatever a page hands a shared partial: the
// PageData itself, a pointer to it, or a struct that embeds it (the Thanks
// page). page has a value receiver, so all three satisfy hasPage.
func pageDataOf(p any) PageData {
	type hasPage interface{ page() PageData }
	if hp, ok := p.(hasPage); ok {
		return hp.page()
	}
	return PageData{Lang: i18n.Default, Languages: i18n.Languages}
}

func (d PageData) page() PageData { return d }

// riskKeys maps the English findings the service probe records (see
// scanner/probe.go) to their catalogue keys.
var riskKeys = map[string]string{
	"Telnet is open (unencrypted remote access)": "common.risk.telnet",
	"FTP is open (unencrypted file transfer)":    "common.risk.ftp",
}
