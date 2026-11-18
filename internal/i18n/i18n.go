// Package i18n holds the interface translations and picks the language for a
// request.
//
// Each language is a directory under locales/ holding one JSON file per part
// of the interface (common.json, dashboard.json, ...). A file is a flat map of
// key to text, and its keys are addressed as "<file>.<key>", so "Settings" in
// common.json is "common.settings". English is the default, the fallback for
// any key a translation lacks, and the reference the tests check every other
// language against.
//
// Text can carry named placeholders, {count} or {name}, so a translator can
// move them to wherever they belong in their language. Text that varies with a
// number has one key per plural form ("x.one", "x.other", and "x.few",
// "x.many", "x.two", "x.zero" for the languages that use them), chosen by
// PluralForm.
package i18n

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

//go:embed locales
var localesFS embed.FS

// Language describes one supported language.
type Language struct {
	ID      string // BCP 47 code, used for the html lang attribute and the cookie
	Native  string // the language's name in its own script, as the picker shows it
	English string // its English name, shown under the native one
	Dir     string // "ltr" or "rtl"
}

// Default is the language used when the visitor has not chosen one. English is
// deliberate rather than guessed from the browser, the same choice LAN Sheriff
// makes: a saved preference always wins, and nobody is surprised.
const Default = "en"

// CookieName holds the visitor's chosen language.
const CookieName = "orangutan_lang"

// Languages lists every supported language in picker order: English first as
// the default, then alphabetical by English name.
var Languages = []Language{
	{"en", "English", "English", "ltr"},
	{"ar", "العربية", "Arabic", "rtl"},
	{"bn", "বাংলা", "Bengali", "ltr"},
	{"zh", "中文", "Chinese", "ltr"},
	{"fr", "Français", "French", "ltr"},
	{"de", "Deutsch", "German", "ltr"},
	{"he", "עברית", "Hebrew", "rtl"},
	{"hi", "हिन्दी", "Hindi", "ltr"},
	{"it", "Italiano", "Italian", "ltr"},
	{"ja", "日本語", "Japanese", "ltr"},
	{"ko", "한국어", "Korean", "ltr"},
	{"pl", "Polski", "Polish", "ltr"},
	{"pt", "Português", "Portuguese", "ltr"},
	{"ru", "Русский", "Russian", "ltr"},
	{"es", "Español", "Spanish", "ltr"},
	{"uk", "Українська", "Ukrainian", "ltr"},
}

// catalogs maps language to key to text, loaded once at startup.
var catalogs = mustLoad()

func mustLoad() map[string]map[string]string {
	c, err := load(localesFS)
	if err != nil {
		panic(err)
	}
	return c
}

func load(fsys fs.FS) (map[string]map[string]string, error) {
	out := map[string]map[string]string{}
	for _, l := range Languages {
		cat := map[string]string{}
		files, err := fs.Glob(fsys, path.Join("locales", l.ID, "*.json"))
		if err != nil {
			return nil, err
		}
		for _, f := range files {
			data, err := fs.ReadFile(fsys, f)
			if err != nil {
				return nil, err
			}
			var m map[string]string
			if err := json.Unmarshal(data, &m); err != nil {
				return nil, fmt.Errorf("%s: %w", f, err)
			}
			ns := strings.TrimSuffix(path.Base(f), ".json")
			for k, v := range m {
				cat[ns+"."+k] = v
			}
		}
		out[l.ID] = cat
	}
	return out, nil
}

// Supported reports whether id is one of Languages.
func Supported(id string) bool {
	for _, l := range Languages {
		if l.ID == id {
			return true
		}
	}
	return false
}

// Get returns the Language for id, or English.
func Get(id string) Language {
	for _, l := range Languages {
		if l.ID == id {
			return l
		}
	}
	return Languages[0]
}

// FromRequest returns the language the visitor chose, from their cookie, or
// Default.
func FromRequest(r *http.Request) string {
	if r == nil {
		return Default
	}
	if c, err := r.Cookie(CookieName); err == nil && Supported(c.Value) {
		return c.Value
	}
	return Default
}

// T returns the text for key in lang with its placeholders filled from args,
// which are name/value pairs: T("fr", "dashboard.found", "count", 3). A key the
// language lacks falls back to English, and one English lacks comes back as
// the key itself, which the tests make sure never ships.
func T(lang, key string, args ...any) string {
	s, ok := catalogs[lang][key]
	if !ok {
		if s, ok = catalogs[Default][key]; !ok {
			return key
		}
	}
	return fill(s, args)
}

// N returns the plural form of key that suits n in lang, with {count} filled
// in as n and any other placeholders from args.
func N(lang, key string, n int, args ...any) string {
	form := key + "." + PluralForm(lang, n)
	if _, ok := catalogs[lang][form]; !ok {
		if _, ok := catalogs[Default][form]; !ok {
			form = key + ".other"
		}
	}
	return T(lang, form, append([]any{"count", n}, args...)...)
}

func fill(s string, args []any) string {
	if len(args) < 2 || !strings.Contains(s, "{") {
		return s
	}
	pairs := make([]string, 0, len(args))
	for i := 0; i+1 < len(args); i += 2 {
		pairs = append(pairs, "{"+fmt.Sprint(args[i])+"}", fmt.Sprint(args[i+1]))
	}
	return strings.NewReplacer(pairs...).Replace(s)
}

// PluralForm returns the CLDR plural category for n in lang: "zero", "one",
// "two", "few", "many" or "other". Only whole numbers are needed here, which
// keeps the rules short.
func PluralForm(lang string, n int) string {
	if n < 0 {
		n = -n
	}
	switch lang {
	case "ja", "zh", "ko":
		return "other"
	case "fr", "hi", "bn":
		// 0 and 1 are both singular.
		if n <= 1 {
			return "one"
		}
		return "other"
	case "ru", "uk":
		switch {
		case n%10 == 1 && n%100 != 11:
			return "one"
		case n%10 >= 2 && n%10 <= 4 && (n%100 < 12 || n%100 > 14):
			return "few"
		default:
			return "many"
		}
	case "pl":
		// Like Russian, except that only 1 itself is singular: 21 is "many".
		switch {
		case n == 1:
			return "one"
		case n%10 >= 2 && n%10 <= 4 && (n%100 < 12 || n%100 > 14):
			return "few"
		default:
			return "many"
		}
	case "ar":
		switch {
		case n == 0:
			return "zero"
		case n == 1:
			return "one"
		case n == 2:
			return "two"
		case n%100 >= 3 && n%100 <= 10:
			return "few"
		case n%100 >= 11:
			return "many"
		default:
			return "other"
		}
	case "he":
		switch n {
		case 1:
			return "one"
		case 2:
			return "two"
		}
		return "other"
	default: // en, de, es, it, pt
		if n == 1 {
			return "one"
		}
		return "other"
	}
}

// PluralForms lists the categories lang uses, which the tests require every
// plural key to provide.
func PluralForms(lang string) []string {
	switch lang {
	case "ja", "zh", "ko":
		return []string{"other"}
	case "ru", "uk", "pl":
		return []string{"one", "few", "many"}
	case "ar":
		return []string{"zero", "one", "two", "few", "many", "other"}
	case "he":
		return []string{"one", "two", "other"}
	default:
		return []string{"one", "other"}
	}
}

// Script returns the strings the page scripts need, from every file whose name
// starts with "js" (js-app.json, js-settings.json), keyed without the file
// name, with English filling any gap. Keys must be unique across those files;
// the tests check.
func Script(lang string) map[string]string {
	out := map[string]string{}
	for _, l := range []string{Default, lang} {
		for k, v := range catalogs[l] {
			ns, key, ok := strings.Cut(k, ".")
			if ok && strings.HasPrefix(ns, "js") {
				out[key] = v
			}
		}
	}
	return out
}

// Keys returns every key in lang's catalogue, for the tests.
func Keys(lang string) map[string]string {
	return catalogs[lang]
}
