// Package brand holds the LAN Orangutan logo for output that has to carry it
// itself rather than load it from /static, such as the self-contained HTML
// report. It depends on nothing, so any package can use it.
package brand

import _ "embed"

//go:embed orangutan.svg
var logoSVG []byte

// LogoSVG returns the logo as SVG.
func LogoSVG() []byte { return logoSVG }
