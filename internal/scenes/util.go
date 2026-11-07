package scenes

import (
	"math"
	"math/rand/v2"

	"github.com/justynroberts/marquee/internal/led"
)

const (
	rows = led.Rows
	cols = led.Cols
)

// fcol is a float colour used while accumulating a frame.
type fcol struct{ r, g, b float64 }

func (c fcol) scale(k float64) fcol { return fcol{c.r * k, c.g * k, c.b * k} }
func (c fcol) add(d fcol) fcol      { return fcol{c.r + d.r, c.g + d.g, c.b + d.b} }
func (c fcol) maxc(d fcol) fcol {
	return fcol{math.Max(c.r, d.r), math.Max(c.g, d.g), math.Max(c.b, d.b)}
}

func fc(c led.RGB) fcol { return fcol{float64(c.R), float64(c.G), float64(c.B)} }

func (c fcol) rgb() led.RGB { return led.RGB{R: u8(c.r), G: u8(c.g), B: u8(c.b)} }

// u8 converts like numpy's clip(0, 255).astype(uint8): truncation, not rounding.
func u8(v float64) uint8 {
	if v <= 0 {
		return 0
	}
	if v >= 255 {
		return 255
	}
	return uint8(v)
}

// hsv matches scenes.py's hsv(): channels computed then (x*255).clip(0,255).astype(uint8).
func hsv(h, s, v float64) led.RGB {
	h = math.Mod(h, 1)
	if h < 0 {
		h++
	}
	h *= 6
	i := int(math.Floor(h)) % 6
	f := h - math.Floor(h)
	p, q, tt := v*(1-s), v*(1-s*f), v*(1-s*(1-f))
	var r, g, b float64
	switch i {
	case 0:
		r, g, b = v, tt, p
	case 1:
		r, g, b = q, v, p
	case 2:
		r, g, b = p, v, tt
	case 3:
		r, g, b = p, q, v
	case 4:
		r, g, b = tt, p, v
	default:
		r, g, b = v, p, q
	}
	return led.RGB{R: u8(r * 255), G: u8(g * 255), B: u8(b * 255)}
}

// stop is one colour stop of a palette.
type stop struct {
	at      float64
	r, g, b float64
}

// palette256 matches scenes.py's palette(): np.interp over linspace(0, 1, 256), truncated.
func palette256(stops ...stop) [256]led.RGB {
	var out [256]led.RGB
	for i := range out {
		x := float64(i) / 255
		var c [3]float64
		switch {
		case x <= stops[0].at:
			c = [3]float64{stops[0].r, stops[0].g, stops[0].b}
		case x >= stops[len(stops)-1].at:
			l := stops[len(stops)-1]
			c = [3]float64{l.r, l.g, l.b}
		default:
			for j := 1; j < len(stops); j++ {
				if x <= stops[j].at {
					a, b := stops[j-1], stops[j]
					k := (x - a.at) / (b.at - a.at)
					c = [3]float64{a.r + (b.r-a.r)*k, a.g + (b.g-a.g)*k, a.b + (b.b-a.b)*k}
					break
				}
			}
		}
		out[i] = led.RGB{R: u8(c[0]), G: u8(c[1]), B: u8(c[2])}
	}
	return out
}

// field is a full frame of float colours.
type field [rows][cols]fcol

func (f *field) clear() {
	for y := range rows {
		for x := range cols {
			f[y][x] = fcol{}
		}
	}
}

func (f *field) scaleAll(k float64) {
	for y := range rows {
		for x := range cols {
			f[y][x] = f[y][x].scale(k)
		}
	}
}

// toGrid lights every LED whose brightest channel reaches floor (very dark LEDs show unlit),
// like the Python to_image(floor=22).
func (f *field) toGrid(g *led.Grid, floor uint8) {
	for y := range rows {
		for x := range cols {
			c := f[y][x].rgb()
			if c.Luma() >= floor {
				g.Lit[y][x] = true
				g.Col[y][x] = c
			}
		}
	}
}

func clamp01(v float64) float64 { return math.Max(0, math.Min(1, v)) }

func uniform(lo, hi float64) float64 { return lo + rand.Float64()*(hi-lo) }

// interp resamples src (len m) to n points, like numpy.interp(linspace(0, m-1, n), arange(m), src).
func interp(src []float64, n int) []float64 {
	out := make([]float64, n)
	m := len(src)
	if m == 0 {
		return out
	}
	for i := range out {
		pos := 0.0
		if n > 1 {
			pos = float64(i) * float64(m-1) / float64(n-1)
		}
		lo := int(math.Floor(pos))
		hi := min(lo+1, m-1)
		out[i] = src[lo] + (src[hi]-src[lo])*(pos-float64(lo))
	}
	return out
}

// stampMask draws m at (x, y), clipping to maxW columns.
func stampMask(g *led.Grid, m *led.Mask, x, y, maxW int, c led.RGB) {
	for my := 0; my < m.H; my++ {
		for mx := 0; mx < min(m.W, maxW); mx++ {
			if m.At(mx, my) {
				g.Set(x+mx, y+my, c)
			}
		}
	}
}

// label draws small text whose ink starts at row top, clipped to maxW.
func label(g *led.Grid, s, font string, size, x, top, maxW int, c led.RGB) int {
	m := led.TextAt(s, font, size, top)
	stampMask(g, m, x, 0, maxW, c)
	return min(m.W, maxW)
}

// message draws lines of text for problem screens.
func message(g *led.Grid, lines []string, c led.RGB) {
	y := 10
	for _, l := range lines {
		label(g, l, "mono", 11, 4, y, cols-8, c)
		y += 16
	}
}

// Shared palette pieces.
var (
	ink   = led.Ink
	ink2  = led.Ink2
	ink3  = led.Ink3
	alert = led.Alert
	green = led.RGB{R: 70, G: 220, B: 110}
	amber = led.RGB{R: 255, G: 214, B: 60}
	track = led.RGB{R: 45, G: 45, B: 40}
	rule  = led.RGB{R: 44, G: 44, B: 38}
)

func grad(stops ...any) led.Gradient {
	var g led.Gradient
	for i := 0; i+1 < len(stops); i += 2 {
		g = append(g, struct {
			At float64
			C  led.RGB
		}{stops[i].(float64), stops[i+1].(led.RGB)})
	}
	return g
}
