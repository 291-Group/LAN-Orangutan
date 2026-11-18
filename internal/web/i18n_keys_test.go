package web

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/291-Group/LAN-Orangutan/internal/i18n"
)

// Every key the templates and Go code ask for must exist in the English
// catalogue. A missing one would show on the page as the raw key, in every
// language, and nothing else would catch it.
func TestTranslationKeysExist(t *testing.T) {
	en := i18n.Keys(i18n.Default)
	// {{t $.Lang "ns.key"}}, {{th ...}}, {{tn $.Lang "ns.key" N}}, i18n.T(lang, "ns.key")
	call := regexp.MustCompile(`(?:\{\{-?\s*(?:t|th|tn)\s+\$?\.?[A-Za-z.]*Lang|i18n\.[TN]\([a-zA-Z.]+,)\s*"([a-zA-Z0-9-]+\.[A-Za-z0-9. ]+)"`)
	var files []string
	tmpl, _ := filepath.Glob("templates/*.html")
	gosrc, _ := filepath.Glob("*.go")
	files = append(files, tmpl...)
	for _, f := range gosrc {
		if !strings.HasSuffix(f, "_test.go") {
			files = append(files, f)
		}
	}
	files = append(files, "../report/report.go", "../api/api.go")

	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		isTN := regexp.MustCompile(`\{\{-?\s*tn\s`)
		for _, m := range call.FindAllStringSubmatchIndex(string(data), -1) {
			key := string(data[m[2]:m[3]])
			if strings.HasSuffix(key, ".") {
				continue // a prefix the code completes, like "common.type." + name
			}
			if isTN.Match(data[m[0]:m[1]]) || string(data[m[0]:m[0]+6]) == "i18n.N" {
				if _, ok := en[key+".other"]; !ok {
					t.Errorf("%s: plural key %q has no .other form in English", f, key)
				}
				continue
			}
			if _, ok := en[key]; !ok {
				t.Errorf("%s: key %q is not in the English catalogue", f, key)
			}
		}
	}
	for _, c := range codeContributors {
		if _, ok := en[c.For]; !ok {
			t.Errorf("credits: %q is not in the English catalogue", c.For)
		}
	}
	for _, b := range builtWith {
		if _, ok := en[b.For]; !ok {
			t.Errorf("credits built with: %q is not in the English catalogue", b.For)
		}
	}
	for _, k := range riskKeys {
		if _, ok := en[k]; !ok {
			t.Errorf("risk key %q is not in the English catalogue", k)
		}
	}
}
