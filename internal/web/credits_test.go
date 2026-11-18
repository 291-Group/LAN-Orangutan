package web

import (
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// The Thanks page and the README's Thanks section must credit the same people,
// so nobody is listed in one and missing from the other.
func TestCreditsMatchReadme(t *testing.T) {
	data, err := os.ReadFile("../../README.md")
	if err != nil {
		t.Fatalf("read README: %v", err)
	}
	readme := string(data)
	start := strings.Index(readme, "## Thanks")
	if start < 0 {
		t.Fatal("README has no Thanks section")
	}
	section := readme[start+len("## Thanks"):]
	if end := strings.Index(section, "\n## "); end >= 0 {
		section = section[:end]
	}

	inReadme := map[string]bool{}
	for _, m := range regexp.MustCompile(`\[@([A-Za-z0-9-]+)\]`).FindAllStringSubmatch(section, -1) {
		inReadme[m[1]] = true
	}
	onPage := map[string]bool{}
	for _, c := range codeContributors {
		if c.Handle != "" {
			onPage[c.Handle] = true
		}
	}
	for _, h := range communityContributors {
		onPage[h] = true
	}

	var missingFromReadme, missingFromPage []string
	for h := range onPage {
		if !inReadme[h] {
			missingFromReadme = append(missingFromReadme, h)
		}
	}
	for h := range inReadme {
		if !onPage[h] {
			missingFromPage = append(missingFromPage, h)
		}
	}
	sort.Strings(missingFromReadme)
	sort.Strings(missingFromPage)
	if len(missingFromReadme) > 0 {
		t.Errorf("on the Thanks page but not in the README: %v", missingFromReadme)
	}
	if len(missingFromPage) > 0 {
		t.Errorf("in the README but not on the Thanks page (internal/web/credits.go): %v", missingFromPage)
	}
}

func TestCommunityContributorsSorted(t *testing.T) {
	if !sort.SliceIsSorted(communityContributors, func(i, j int) bool {
		return strings.ToLower(communityContributors[i]) < strings.ToLower(communityContributors[j])
	}) {
		t.Error("communityContributors should stay alphabetical, ignoring case")
	}
}
