package cli

import (
	"fmt"
	"regexp"
	"runtime"
	"runtime/debug"
	"strings"

	"github.com/spf13/cobra"
)

// Version information, set via ldflags by the Makefile and by the release
// workflow. When it is not set, resolveVersion fills these in from the build
// information Go embeds automatically, so a plain `go build` still reports
// something meaningful rather than "dev".
var (
	Version   = ""
	Commit    = ""
	BuildDate = ""
)

func init() {
	resolveVersion()

	// The release workflow strips the tag's leading "v" while a local
	// `make build` keeps it, so the same commit could report "3.1.3" or
	// "v3.1.3". Normalise so every build of a release shows the same string.
	Version = strings.TrimPrefix(Version, "v")
}

// resolveVersion fills in any version details that were not supplied at build
// time.
//
// Go records the commit, its timestamp, and whether the tree was dirty for any
// binary built inside a repository, so a source build can report its exact
// provenance without needing the Makefile.
func resolveVersion() {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		applyFallbacks()
		return
	}

	var revision, vcsTime string
	var modified bool
	for _, setting := range info.Settings {
		switch setting.Key {
		case "vcs.revision":
			revision = setting.Value
		case "vcs.time":
			vcsTime = setting.Value
		case "vcs.modified":
			modified = setting.Value == "true"
		}
	}

	if Commit == "" && revision != "" {
		Commit = shortCommit(revision)
	}
	if BuildDate == "" && vcsTime != "" {
		BuildDate = vcsTime
	}

	if Version == "" {
		// `go install ...@v3.1.3` records the real tag, which is what we want
		// to show. A build from a working copy instead records either
		// "(devel)" or a generated pseudo-version like
		// v0.0.0-20260715193857-0c49f4c9dda2, which is accurate but unreadable.
		// In that case show the short commit, which says the same thing.
		if v := info.Main.Version; isReleaseVersion(v) {
			Version = v
		} else if revision != "" {
			Version = shortCommit(revision)
			if modified {
				Version += "-dirty"
			}
		}
	}

	applyFallbacks()
}

// applyFallbacks covers the case where nothing could be determined at all, such
// as a binary built from a source tarball with no repository present.
func applyFallbacks() {
	if Version == "" {
		Version = "unknown"
	}
	if Commit == "" {
		Commit = "unknown"
	}
	if BuildDate == "" {
		BuildDate = "unknown"
	}
}

// pseudoVersion matches the version string Go generates for a build that is not
// at a tagged release, for example v0.0.0-20260715193857-0c49f4c9dda2.
var pseudoVersion = regexp.MustCompile(`-\d{14}-[0-9a-f]{12}`)

// isReleaseVersion reports whether v is a real tagged version worth showing to
// a user, rather than a generated placeholder.
func isReleaseVersion(v string) bool {
	if v == "" || v == "(devel)" {
		return false
	}
	return !pseudoVersion.MatchString(v)
}

// shortCommit trims a full git hash to the usual short form.
func shortCommit(revision string) string {
	if len(revision) > 7 {
		return revision[:7]
	}
	return revision
}

// DisplayVersion returns the version string shown in the web interface.
func DisplayVersion() string {
	return Version
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version information",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("LAN Orangutan %s\n", Version)
		fmt.Printf("  Commit: %s\n", Commit)
		fmt.Printf("  Built: %s\n", BuildDate)
		fmt.Printf("  Go: %s\n", runtime.Version())
		fmt.Printf("  OS/Arch: %s/%s\n", runtime.GOOS, runtime.GOARCH)
	},
}
