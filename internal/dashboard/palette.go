package dashboard

import (
	"image"
	"image/color"
	"strings"
)

// The panel's LEDs swallow dark greys, so the full-resolution screens added after the system
// dashboard use a brighter palette with colour for meaning. (The system dashboard keeps ink2,
// ink3 and rule, which must match the Python original pixel for pixel.)
var (
	txt    = color.RGBA{250, 246, 236, 255}
	txt2   = color.RGBA{238, 234, 222, 255}
	txt3   = color.RGBA{198, 193, 180, 255}
	line   = color.RGBA{70, 71, 63, 255}
	cyan   = color.RGBA{86, 204, 255, 255}  // headings, names, labels
	amber  = color.RGBA{255, 186, 64, 255}  // the key number on a screen, warm readings
	green  = color.RGBA{112, 228, 124, 255} // good, rising, low
	violet = color.RGBA{186, 146, 255, 255} // secondary series
)

// head is a section heading: small capitals in cyan.
func head(dst *image.RGBA, x, y int, s string, width int) {
	text(dst, float64(x), y, fit(strings.ToUpper(s), 22, float64(width)), 22, cyan)
}

// vrule is vline in the brighter palette.
func vrule(dst *image.RGBA, x int) { wideLine(dst, x, pad, x, H-pad, line, 2) }

// sourceColour gives each news source its own colour: Hacker News amber, the BBC cyan, others
// by name.
func sourceColour(name string) color.RGBA {
	n := strings.ToUpper(name)
	switch {
	case n == "HN" || strings.HasPrefix(n, "HACKER"):
		return amber
	case strings.HasPrefix(n, "BBC"):
		return cyan
	}
	h := 0
	for _, r := range n {
		h = h*31 + int(r)
	}
	return []color.RGBA{green, violet, cyan, amber}[(h%4+4)%4]
}

// ramp picks a colour from green through amber to hot as k runs 0..1.
func ramp(k float64) color.RGBA {
	switch {
	case k <= 0:
		return green
	case k < 0.5:
		return mix(green, amber, k*2)
	case k < 1:
		return mix(amber, hot, (k-0.5)*2)
	}
	return hot
}
