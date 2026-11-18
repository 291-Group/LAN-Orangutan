package i18n

import (
	"regexp"
	"sort"
	"strings"
	"testing"
	"unicode"
)

var placeholder = regexp.MustCompile(`\{[A-Za-z]+\}`)

// pluralBase returns the key without its plural suffix, and whether it had one.
func pluralBase(key string) (string, bool) {
	for _, f := range []string{"zero", "one", "two", "few", "many", "other"} {
		if strings.HasSuffix(key, "."+f) {
			return strings.TrimSuffix(key, "."+f), true
		}
	}
	return key, false
}

// Every language must translate every English key (plural keys in the forms
// that language uses), carry no keys English does not have, and keep the same
// placeholders, since a dropped {count} silently loses the number.
func TestCataloguesComplete(t *testing.T) {
	en := Keys(Default)
	if len(en) == 0 {
		t.Fatal("English catalogue is empty")
	}
	enPlural := map[string]bool{}
	for k := range en {
		if base, ok := pluralBase(k); ok {
			enPlural[base] = true
		}
	}

	for _, l := range Languages {
		if l.ID == Default {
			continue
		}
		cat := Keys(l.ID)
		var problems []string
		for k, v := range en {
			if base, ok := pluralBase(k); ok && enPlural[base] {
				continue // checked per form below
			}
			tv, ok := cat[k]
			if !ok {
				problems = append(problems, "missing "+k)
				continue
			}
			if !samePlaceholders(v, tv) {
				problems = append(problems, "placeholders differ in "+k)
			}
		}
		for base := range enPlural {
			ref := en[base+".other"]
			for _, form := range PluralForms(l.ID) {
				tv, ok := cat[base+"."+form]
				if !ok {
					problems = append(problems, "missing "+base+"."+form)
					continue
				}
				// The zero, one and two forms name an exact number, so a
				// language may say it in words ("two days ago" is a bare dual
				// in Arabic) and leave {count} out.
				if !samePlaceholders(ref, tv) && !(exactForm(form) && samePlaceholders(withoutCount(ref), withoutCount(tv))) {
					problems = append(problems, "placeholders differ in "+base+"."+form)
				}
			}
		}
		for k := range cat {
			if base, ok := pluralBase(k); ok && enPlural[base] {
				continue
			}
			if _, ok := en[k]; !ok {
				problems = append(problems, "not in English: "+k)
			}
		}
		sort.Strings(problems)
		for _, p := range problems {
			t.Errorf("%s: %s", l.ID, p)
		}
	}
}

func exactForm(form string) bool {
	return form == "zero" || form == "one" || form == "two"
}

func withoutCount(s string) string {
	return strings.ReplaceAll(s, "{count}", "")
}

func samePlaceholders(a, b string) bool {
	pa, pb := placeholder.FindAllString(a, -1), placeholder.FindAllString(b, -1)
	sort.Strings(pa)
	sort.Strings(pb)
	return strings.Join(pa, ",") == strings.Join(pb, ",")
}

// Latin words that legitimately stay in Latin script inside a non-Latin
// translation: product, protocol and platform names, and things typed exactly.
// Ported from LAN Sheriff's scripts/check-i18n.mjs.
var allowedLatin = map[string]bool{}

func init() {
	for _, w := range strings.Fields(`
		LAN Orangutan Group GitHub IP IPv4 IPv6 MAC DNS DHCP mDNS ARP TCP UDP HTTP HTTPS
		CSV JSON HTML API URL CIDR VPN TV IoT OS macOS Linux Windows Docker
		Tailscale tailnet Nmap nmap arp scan sudo root Go Cobra IEEE bcrypt
		Pi Raspberry NetBIOS Bearer Authorization Ctrl Esc Enter T R ini config
		lano MB GB KB ms s px Wi Fi WiFi Ethernet eth wlan en id
		JavaScript Telnet FTP SSH NAS VLAN VLANs Bluetooth
		Telegram Discord Slack ntfy Home Assistant Webhook Webhooks
		http https network ip id seconds from to action new unexpected missing moved
	`) {
		allowedLatin[w] = true
	}
}

var (
	latinWord = regexp.MustCompile(`[A-Za-z][A-Za-z0-9]+`)
	codeSpan  = regexp.MustCompile(`(?s)<(code|kbd)[^>]*>.*?</(code|kbd)>`)
	tag       = regexp.MustCompile(`<[^>]+>`)
)

// A translation into a non-Latin script must not contain stray Latin words
// (beyond the names above), and a Latin-script translation must not contain
// stray characters from another script. Both are the mechanical damage bulk
// edits leave behind, the kind LAN Sheriff caught as "טלמetriה". Neither
// replaces a native speaker's review.
func TestNoStrayScript(t *testing.T) {
	nonLatin := map[string]bool{"ar": true, "bn": true, "zh": true, "he": true, "hi": true, "ja": true, "ko": true, "ru": true, "uk": true}
	for _, l := range Languages {
		for k, v := range Keys(l.ID) {
			// Placeholders, markup and anything inside <code> or <kbd> are code,
			// not text.
			text := codeSpan.ReplaceAllString(v, " ")
			text = tag.ReplaceAllString(text, " ")
			text = placeholder.ReplaceAllString(text, " ")
			if nonLatin[l.ID] {
				for _, w := range latinWord.FindAllString(text, -1) {
					if !allowedLatin[w] {
						t.Errorf("%s %s: stray Latin %q in %q", l.ID, k, w, v)
					}
				}
				continue
			}
			for _, r := range text {
				if unicode.IsLetter(r) && !unicode.In(r, unicode.Latin) {
					t.Errorf("%s %s: stray non-Latin %q in %q", l.ID, k, r, v)
					break
				}
			}
		}
	}
}

func TestPluralForms(t *testing.T) {
	cases := []struct {
		lang string
		n    int
		want string
	}{
		{"en", 1, "one"}, {"en", 0, "other"}, {"en", 2, "other"},
		{"fr", 0, "one"}, {"fr", 2, "other"},
		{"ru", 1, "one"}, {"ru", 3, "few"}, {"ru", 5, "many"}, {"ru", 11, "many"}, {"ru", 21, "one"}, {"ru", 22, "few"},
		{"ar", 0, "zero"}, {"ar", 2, "two"}, {"ar", 5, "few"}, {"ar", 11, "many"}, {"ar", 100, "other"},
		{"uk", 1, "one"}, {"uk", 3, "few"}, {"uk", 5, "many"}, {"uk", 21, "one"},
		{"pl", 1, "one"}, {"pl", 3, "few"}, {"pl", 5, "many"}, {"pl", 12, "many"}, {"pl", 21, "many"}, {"pl", 22, "few"},
		{"it", 1, "one"}, {"it", 0, "other"},
		{"ja", 1, "other"}, {"ko", 1, "other"}, {"he", 2, "two"},
	}
	for _, c := range cases {
		if got := PluralForm(c.lang, c.n); got != c.want {
			t.Errorf("PluralForm(%s, %d) = %q, want %q", c.lang, c.n, got, c.want)
		}
		found := false
		for _, f := range PluralForms(c.lang) {
			if f == c.want {
				found = true
			}
		}
		if !found {
			t.Errorf("PluralForms(%s) does not list %q", c.lang, c.want)
		}
	}
}

func TestFallbackAndFill(t *testing.T) {
	if got := T("xx", "common.byOrg", "org", "291 Group"); got != "LAN Orangutan by 291 Group" {
		t.Errorf("unknown language should fall back to English, got %q", got)
	}
	if got := N("en", "common.time.dayAgo", 3); got != "3 days ago" {
		t.Errorf("N = %q", got)
	}
	if got := T("en", "no.such.key"); got != "no.such.key" {
		t.Errorf("missing key = %q", got)
	}
}

// Script merges every js*.json file into one flat map, so a key used in two of
// them would silently shadow the other.
func TestScriptKeysUnique(t *testing.T) {
	owner := map[string]string{}
	for k := range Keys(Default) {
		ns, key, ok := strings.Cut(k, ".")
		if !ok || !strings.HasPrefix(ns, "js") {
			continue
		}
		if prev, dup := owner[key]; dup && prev != ns {
			t.Errorf("script key %q is in both %s.json and %s.json", key, prev, ns)
		}
		owner[key] = ns
	}
}

// The server writes relative times with common.time.* and app.js rewrites the
// same text every 30 seconds with app.time.*. If the two differ, the wording
// visibly changes under the reader, so they must match in every language.
func TestRelativeTimesMatchBetweenServerAndScript(t *testing.T) {
	for _, l := range Languages {
		cat := Keys(l.ID)
		for k, v := range cat {
			if !strings.HasPrefix(k, "js-app.app.time.") {
				continue
			}
			server := "common.time." + strings.TrimPrefix(k, "js-app.app.time.")
			if sv, ok := cat[server]; ok && sv != v {
				t.Errorf("%s: %s is %q but %s is %q", l.ID, k, v, server, sv)
			}
		}
	}
}
