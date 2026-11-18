// Package report renders a self-contained HTML summary of the network that can
// be saved and shared as a single file (no external assets).
package report

import (
	"bytes"
	"encoding/base64"
	"html/template"
	"io"
	"net"
	"sort"
	"time"

	"github.com/291-Group/LAN-Orangutan/internal/brand"
	"github.com/291-Group/LAN-Orangutan/internal/i18n"
	"github.com/291-Group/LAN-Orangutan/internal/scanner"
	"github.com/291-Group/LAN-Orangutan/internal/types"
)

// Data is everything the report template needs. Devices are expected already
// sorted and with their vendor resolved by the caller. Lang is the language
// the report is written in; empty means English, which the command line uses.
type Data struct {
	Lang         string
	GeneratedAt  time.Time
	Total        int
	Online       int
	Offline      int
	NetworkCount int
	Devices      []types.Device
}

// NewData builds report data from the stored devices, so the web download and
// "orangutan report" produce the same file. Vendors are resolved and devices
// are sorted by address numerically (10.0.0.2 before 10.0.0.10).
func NewData(devices map[string]*types.Device, stats types.DeviceStats, networkCount int, now time.Time) Data {
	list := make([]types.Device, 0, len(devices))
	for _, d := range devices {
		dc := *d
		dc.Vendor = scanner.ResolveVendor(dc.Vendor, dc.MAC)
		list = append(list, dc)
	}
	sort.Slice(list, func(i, j int) bool { return ipLess(list[i].IP, list[j].IP) })
	return Data{
		GeneratedAt:  now,
		Total:        stats.Total,
		Online:       stats.Online,
		Offline:      stats.Offline,
		NetworkCount: networkCount,
		Devices:      list,
	}
}

// ipLess orders IPv4 before IPv6 and each numerically, with anything that does
// not parse last.
func ipLess(a, b string) bool {
	ka, kb := ipKey(a), ipKey(b)
	if ka == nil || kb == nil {
		if ka == nil && kb == nil {
			return a < b
		}
		return kb == nil
	}
	return bytes.Compare(ka, kb) < 0
}

func ipKey(s string) []byte {
	ip := net.ParseIP(s)
	if ip == nil {
		return nil
	}
	if v4 := ip.To4(); v4 != nil {
		return append([]byte{4}, v4...)
	}
	return append([]byte{6}, ip.To16()...)
}

var tmpl = template.Must(template.New("report").Funcs(template.FuncMap{
	"t":  i18n.T,
	"tn": i18n.N,
	// typeName shows a device type in the report's language, as the
	// dashboard does; a type with no translation shows as stored.
	"typeName": func(lang, deviceType string) string {
		if s := i18n.T(lang, "common.type."+deviceType); s != "common.type."+deviceType {
			return s
		}
		return deviceType
	},
}).Parse(reportHTML))

// logoURI is the app logo as a data URI, so the saved report shows the real
// logo with no external assets, and looks the same on every platform (an
// emoji would render differently everywhere, or not at all).
var logoURI = template.URL("data:image/svg+xml;base64," + base64.StdEncoding.EncodeToString(brand.LogoSVG()))

// Render writes the HTML report to w.
func Render(w io.Writer, d Data) error {
	if d.Lang == "" {
		d.Lang = i18n.Default
	}
	return tmpl.Execute(w, struct {
		Data
		Dir  string
		Logo template.URL
	}{d, i18n.Get(d.Lang).Dir, logoURI})
}

const reportHTML = `<!doctype html>
<html lang="{{.Lang}}"{{if eq .Dir "rtl"}} dir="rtl"{{end}}>
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>LAN Orangutan: {{t $.Lang "report.networkReport"}}</title>
<style>
  :root { --fg:#16202e; --muted:#56637a; --border:#e4ecf6; --bg:#ffffff; --green:#2c9e6b; --grey:#8592a6; }
  * { box-sizing: border-box; }
  body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; color: var(--fg); background: var(--bg); margin: 0; padding: 2rem; line-height: 1.5; }
  .wrap { max-width: 980px; margin: 0 auto; }
  h1 { font-size: 1.5rem; margin: 0 0 0.25rem; display: flex; align-items: center; gap: 0.6rem; }
  h1 img { width: 36px; height: 36px; flex: none; }
  .sub { color: var(--muted); font-size: 0.9rem; margin: 0 0 1.5rem; }
  .stats { display: flex; flex-wrap: wrap; gap: 1.75rem; margin: 0 0 1.5rem; padding: 1rem 1.25rem; border: 1px solid var(--border); border-radius: 12px; }
  .stat b { font-size: 1.35rem; }
  .stat span { color: var(--muted); font-size: 0.85rem; margin-left: 0.35rem; }
  table { width: 100%; border-collapse: collapse; font-size: 0.85rem; }
  th, td { text-align: start; padding: 0.5rem 0.6rem; border-bottom: 1px solid var(--border); vertical-align: top; }
  th { color: var(--muted); font-weight: 600; text-transform: uppercase; font-size: 0.72rem; letter-spacing: 0.03em; }
  td.mono { font-family: 'SF Mono', 'Fira Code', monospace; }
  [dir="rtl"] td.mono { direction: ltr; unicode-bidi: isolate; }
  .dot { display: inline-block; width: 8px; height: 8px; border-radius: 50%; }
  .dot.on { background: var(--green); }
  .dot.off { background: transparent; border: 1.5px solid var(--grey); }
  .muted { color: var(--muted); }
  footer { margin-top: 2rem; color: var(--muted); font-size: 0.8rem; }
  @media print { body { padding: 0; } .stats { break-inside: avoid; } thead { display: table-header-group; } }
</style>
</head>
<body>
<div class="wrap">
  <h1><img src="{{.Logo}}" alt="" width="36" height="36">LAN Orangutan: {{t $.Lang "report.networkReport"}}</h1>
  <p class="sub">{{t $.Lang "report.generated" "date" (.GeneratedAt.Format "Mon, 02 Jan 2006") "time" (.GeneratedAt.Format "15:04")}}</p>
  <div class="stats">
    <div class="stat"><b>{{.Total}}</b><span>{{tn $.Lang "report.devices" .Total}}</span></div>
    <div class="stat"><b>{{.Online}}</b><span>{{t $.Lang "report.online"}}</span></div>
    <div class="stat"><b>{{.Offline}}</b><span>{{t $.Lang "report.offline"}}</span></div>
    <div class="stat"><b>{{.NetworkCount}}</b><span>{{tn $.Lang "report.networks" .NetworkCount}}</span></div>
  </div>
  <table>
    <thead>
      <tr><th></th><th>{{t $.Lang "report.ipAddress"}}</th><th>{{t $.Lang "report.hostname"}}</th><th>{{t $.Lang "report.type"}}</th><th>{{t $.Lang "report.mac"}}</th><th>{{t $.Lang "report.vendor"}}</th><th>{{t $.Lang "report.group"}}</th><th>{{t $.Lang "report.location"}}</th><th>{{t $.Lang "report.lastSeen"}}</th></tr>
    </thead>
    <tbody>
    {{range .Devices}}
      <tr>
        <td><span class="dot {{if .IsOnline}}on{{else}}off{{end}}" title="{{if .IsOnline}}{{t $.Lang "common.status.online"}}{{else}}{{t $.Lang "common.status.offline"}}{{end}}"></span></td>
        <td class="mono">{{.IP}}</td>
        <td>{{if .DisplayHostname}}{{.DisplayHostname}}{{else}}<span class="muted">-</span>{{end}}</td>
        <td>{{if .Type}}{{typeName $.Lang .Type}}{{else}}<span class="muted">-</span>{{end}}</td>
        <td class="mono">{{if .MAC}}{{.MAC}}{{else}}<span class="muted">-</span>{{end}}</td>
        <td>{{if .Vendor}}{{.Vendor}}{{else}}<span class="muted">-</span>{{end}}</td>
        <td>{{if .Group}}{{.Group}}{{else}}<span class="muted">-</span>{{end}}</td>
        <td>{{if .Location}}{{.Location}}{{else}}<span class="muted">-</span>{{end}}</td>
        <td class="muted">{{.LastSeen.Format "2006-01-02 15:04"}}</td>
      </tr>
    {{end}}
    </tbody>
  </table>
  <footer>{{t $.Lang "report.generatedBy"}} &middot; 291 Group</footer>
</div>
</body>
</html>
`
