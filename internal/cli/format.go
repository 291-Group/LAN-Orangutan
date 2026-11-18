package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
	"unicode"

	"github.com/291-Group/LAN-Orangutan/internal/types"
)

// timeLayout is the one layout every listing uses for a moment in time, in the
// local zone, so the same scan reads the same in every command.
const timeLayout = "2006-01-02 15:04"

// csvTimeLayout keeps the seconds, because a CSV file is for other programs
// and has to stay interchangeable with the one the browser downloads from
// /api/devices?format=csv.
const csvTimeLayout = "2006-01-02 15:04:05"

func formatTime(t time.Time) string {
	return t.Local().Format(timeLayout)
}

func yesNo(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

// deviceStatus is the word the web interface and the API use for whether a
// device is here now: "online" when it answered within the last hour, the
// same rule as the API's "online" field.
func deviceStatus(d *types.Device) string {
	if d.IsOnline() {
		return "online"
	}
	return "offline"
}

// printJSON writes v to stdout the way the API would, indented and with a
// trailing newline, so the output can be piped straight into jq.
func printJSON(v any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

// outputFormat validates a --format value against the formats a command
// supports, case-insensitively, and returns it in canonical form.
func outputFormat(value string, allowed ...string) (string, error) {
	v := strings.ToLower(strings.TrimSpace(value))
	for _, a := range allowed {
		if v == a {
			return a, nil
		}
	}
	return "", fmt.Errorf("unknown format %q (%s)", value, strings.Join(allowed, ", "))
}

// truncate shortens s to at most width columns on screen, adding "..." when
// something was cut. It counts runes rather than bytes, and wide characters
// (CJK, emoji) as two columns, so a cut never lands inside a character and a
// column of names stays aligned.
// count reads "1 device" or "3 devices", so a message never says "1 devices".
// plural is given in full, since not every word takes an s.
func count(n int, singular, plural string) string {
	if n == 1 {
		return "1 " + singular
	}
	return fmt.Sprintf("%d %s", n, plural)
}

// printable replaces control characters with "?", so a label that came in
// through the API or an old data file cannot ring the terminal bell or move
// the cursor when the table is printed. Tabs become spaces.
func printable(s string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r == '\t':
			return ' '
		case r < 0x20, r == 0x7f:
			return '?'
		}
		return r
	}, s)
}

func truncate(s string, width int) string {
	if displayWidth(s) <= width {
		return s
	}
	const ellipsis = "..."
	if width <= len(ellipsis) {
		return cutToWidth(s, width)
	}
	return cutToWidth(s, width-len(ellipsis)) + ellipsis
}

// cutToWidth returns the longest prefix of s that fits in width columns.
func cutToWidth(s string, width int) string {
	used := 0
	for i, r := range s {
		w := runeWidth(r)
		if used+w > width {
			return s[:i]
		}
		used += w
	}
	return s
}

// displayWidth is how many terminal columns s occupies.
func displayWidth(s string) int {
	n := 0
	for _, r := range s {
		n += runeWidth(r)
	}
	return n
}

// runeWidth is a small stand-in for a full East Asian Width table: combining
// marks and joiners take no space, CJK and emoji take two columns, and
// everything else one. That covers the names devices actually have.
func runeWidth(r rune) int {
	switch {
	case r == 0, unicode.Is(unicode.Mn, r), unicode.Is(unicode.Me, r), unicode.Is(unicode.Cf, r):
		return 0
	case r >= 0x1100 && r <= 0x115F, // Hangul Jamo
		r >= 0x2E80 && r <= 0xA4CF && r != 0x303F, // CJK radicals through Yi
		r >= 0xAC00 && r <= 0xD7A3,                // Hangul syllables
		r >= 0xF900 && r <= 0xFAFF,                // CJK compatibility ideographs
		r >= 0xFE30 && r <= 0xFE4F,                // CJK compatibility forms
		r >= 0xFF00 && r <= 0xFF60,                // fullwidth forms
		r >= 0xFFE0 && r <= 0xFFE6,
		r >= 0x1F300 && r <= 0x1FAFF, // emoji and pictographs
		r >= 0x20000 && r <= 0x3FFFD: // CJK extensions
		return 2
	}
	return 1
}

// printTable writes a header, an underline and the rows as aligned columns.
// text/tabwriter counts runes, so a CJK hostname pushes the rest of its row
// out of line; this pads by display width instead.
func printTable(w io.Writer, header []string, rows [][]string) error {
	widths := make([]int, len(header))
	for i, h := range header {
		widths[i] = displayWidth(h)
	}
	for _, row := range rows {
		for i, cell := range row {
			row[i] = printable(cell)
			cell = row[i]
			if i < len(widths) && displayWidth(cell) > widths[i] {
				widths[i] = displayWidth(cell)
			}
		}
	}
	underline := make([]string, len(header))
	for i, h := range header {
		underline[i] = strings.Repeat("-", displayWidth(h))
	}
	for _, row := range append([][]string{header, underline}, rows...) {
		var b strings.Builder
		for i, cell := range row {
			b.WriteString(cell)
			if i < len(row)-1 {
				b.WriteString(strings.Repeat(" ", widths[i]-displayWidth(cell)+2))
			}
		}
		if _, err := fmt.Fprintln(w, strings.TrimRight(b.String(), " ")); err != nil {
			return err
		}
	}
	return nil
}
