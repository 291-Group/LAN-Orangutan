package web

import (
	"bytes"
	"net/http"
)

// Contributor is someone credited for code on the Thanks page. Handle is their
// GitHub handle; an organisation without one gives Name and URL instead. For
// says what they built, as a catalogue key so it shows in the visitor's
// language.
type Contributor struct {
	Handle string
	Name   string
	URL    string
	For    string
}

// codeContributors wrote code that shipped. 291 Group first, as the authors of
// the app, then outside contributors, newest first.
var codeContributors = []Contributor{
	{Name: "291 Group", URL: "https://291group.com", For: "credits.for291Group"},
	{Handle: "katsugtgz", For: "credits.forCiActions"},
	{Handle: "dajiaohuang", For: "credits.forIPv6Merge"},
	{Handle: "AdamMagued", For: "credits.forDeviceTypes"},
}

// communityContributors opened an issue or a discussion, reported a bug, or
// tested a fix until it was right. Alphabetical, ignoring case.
//
// This list and the Thanks section of README.md must name the same people;
// TestCreditsMatchReadme fails when they drift apart.
var communityContributors = []string{
	"Bluesguy1984", "chraso", "dao55", "dax29", "dbm26", "dcchillin46",
	"devra1", "Extra-Citron-7630", "forke24x7", "GrafDogi", "JanusHoi",
	"Louai199", "luciancelmare", "maurognx", "msanaullahsahar", "technoplayer",
	"tolga-balci", "wmgordon3", "y90ziv",
}

// Dependency is one piece of the open source LAN Orangutan stands on. For is
// a catalogue key; the names are not translated.
type Dependency struct {
	Name, URL, For string
}

// builtWith lists them in the order the Thanks page shows them.
var builtWith = []Dependency{
	{"Go", "https://go.dev", "credits.builtGo"},
	{"Nmap", "https://nmap.org", "credits.builtNmap"},
	{"arp-scan", "https://github.com/royhills/arp-scan", "credits.builtArpScan"},
	{"Cobra", "https://github.com/spf13/cobra", "credits.builtCobra"},
	{"Go crypto", "https://pkg.go.dev/golang.org/x/crypto", "credits.builtCrypto"},
	{"IEEE registry", "https://standards-oui.ieee.org", "credits.builtIeee"},
}

// handleCredits renders the Thanks page. Everything on it is compiled in: the
// page loads nothing from GitHub or anywhere else, in keeping with the promise
// that the app makes no connection the user did not ask for.
func (h *Handler) handleCredits(w http.ResponseWriter, r *http.Request) {
	data := struct {
		PageData
		Code      []Contributor
		Community []string
		BuiltWith []Dependency
	}{
		PageData: PageData{
			Title:       "Thanks - LAN Orangutan",
			Theme:       h.cfg.UI.Theme,
			Version:     h.version,
			AuthEnabled: h.auth.Enabled(),
		},
		Code:      codeContributors,
		Community: communityContributors,
		BuiltWith: builtWith,
	}

	withLang(r, &data.PageData)

	var buf bytes.Buffer
	if err := h.templates.ExecuteTemplate(&buf, "credits.html", data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	buf.WriteTo(w)
}
