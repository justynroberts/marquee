// Package led is the drawing surface every Marquee program uses: a grid of round LEDs
// (275 x 66 cells of 7 x 7 pixels on a 1920 x 462 panel), helpers to colour it, render text
// into it, and expand it to the panel image.
package led

import (
	"image"
	"image/color"
	"math"
)

const (
	Cell = 7    // pixels per LED
	W    = 1920 // panel pixels
	H    = 462
	Rows = H / Cell              // 66
	Cols = (W + Cell - 1) / Cell // 275 (the last column is cropped)
)

// RGB is one LED colour.
type RGB struct{ R, G, B uint8 }

var (
	Black  = RGB{}
	Ground = RGB{12, 13, 11} // gaps between LEDs
	Dim    = RGB{30, 30, 26} // an unlit LED
	Ink    = RGB{236, 232, 220}
	Ink2   = RGB{161, 157, 144}
	Ink3   = RGB{96, 93, 85}
	Alert  = RGB{255, 91, 56}
	Phos   = RGB{70, 255, 120}
)

// Palette are the named colours tags and the API accept.
var Palette = map[string]RGB{
	"white": Ink, "grey": {140, 138, 130}, "gray": {140, 138, 130},
	"red": {255, 64, 56}, "orange": {255, 140, 40}, "yellow": {255, 214, 60},
	"green": {70, 220, 110}, "cyan": {60, 210, 230}, "blue": {80, 140, 255},
	"purple": {170, 110, 255}, "pink": {255, 105, 200}, "alert": Alert, "black": Black,
}

// Scale multiplies a colour by k (clamped).
func (c RGB) Scale(k float64) RGB {
	return RGB{clamp(float64(c.R) * k), clamp(float64(c.G) * k), clamp(float64(c.B) * k)}
}

// Lerp blends from c to d by t in 0..1.
func (c RGB) Lerp(d RGB, t float64) RGB {
	return RGB{clamp(float64(c.R) + (float64(d.R)-float64(c.R))*t), clamp(float64(c.G) + (float64(d.G)-float64(c.G))*t), clamp(float64(c.B) + (float64(d.B)-float64(c.B))*t)}
}

// Max is the per-channel maximum.
func (c RGB) Max(d RGB) RGB {
	return RGB{max(c.R, d.R), max(c.G, d.G), max(c.B, d.B)}
}

// Luma is the brightest channel, 0..255.
func (c RGB) Luma() uint8 { return max(c.R, c.G, c.B) }

func clamp(v float64) uint8 {
	if v <= 0 {
		return 0
	}
	if v >= 255 {
		return 255
	}
	return uint8(v + 0.5)
}

// HSV converts hue (any real, wraps), saturation and value (0..1) to RGB.
func HSV(h, s, v float64) RGB {
	h = math.Mod(h, 1)
	if h < 0 {
		h++
	}
	h *= 6
	i := int(h) % 6
	f := h - math.Floor(h)
	p, q, t := v*(1-s), v*(1-s*f), v*(1-s*(1-f))
	var r, g, b float64
	switch i {
	case 0:
		r, g, b = v, t, p
	case 1:
		r, g, b = q, v, p
	case 2:
		r, g, b = p, v, t
	case 3:
		r, g, b = p, q, v
	case 4:
		r, g, b = t, p, v
	default:
		r, g, b = v, p, q
	}
	return RGB{clamp(r * 255), clamp(g * 255), clamp(b * 255)}
}

// Gradient is a colour ramp sampled at 0..1.
type Gradient []struct {
	At float64
	C  RGB
}

// At samples the gradient.
func (g Gradient) At(t float64) RGB {
	if len(g) == 0 {
		return Black
	}
	if t <= g[0].At {
		return g[0].C
	}
	for i := 1; i < len(g); i++ {
		if t <= g[i].At {
			a, b := g[i-1], g[i]
			return a.C.Lerp(b.C, (t-a.At)/(b.At-a.At))
		}
	}
	return g[len(g)-1].C
}

// Grid is a frame at LED resolution. Lit LEDs show their colour; unlit ones show Back.
type Grid struct {
	Lit  [Rows][Cols]bool
	Col  [Rows][Cols]RGB
	Back [Rows][Cols]RGB // colour of an unlit LED (default Dim)
}

// NewGrid returns an empty grid with every LED unlit and dim.
func NewGrid() *Grid {
	g := &Grid{}
	g.ClearBack(Dim)
	return g
}

// ClearBack sets every unlit LED's colour.
func (g *Grid) ClearBack(c RGB) {
	for y := range Rows {
		for x := range Cols {
			g.Back[y][x] = c
		}
	}
}

// Set lights one LED (bounds-checked).
func (g *Grid) Set(x, y int, c RGB) {
	if x >= 0 && x < Cols && y >= 0 && y < Rows {
		g.Lit[y][x] = true
		g.Col[y][x] = c
	}
}

// SetMax lights an LED with the brighter of its current and the new colour.
func (g *Grid) SetMax(x, y int, c RGB) {
	if x >= 0 && x < Cols && y >= 0 && y < Rows {
		if g.Lit[y][x] {
			c = c.Max(g.Col[y][x])
		}
		g.Lit[y][x] = true
		g.Col[y][x] = c
	}
}

// Fill lights a rectangle of LEDs [x0,x1) x [y0,y1).
func (g *Grid) Fill(x0, y0, x1, y1 int, c RGB) {
	for y := max(0, y0); y < min(Rows, y1); y++ {
		for x := max(0, x0); x < min(Cols, x1); x++ {
			g.Lit[y][x] = true
			g.Col[y][x] = c
		}
	}
}

// FromColours lights every LED whose colour is brighter than floor (for field-style scenes).
func (g *Grid) FromColours(cols *[Rows][Cols]RGB, floor uint8) {
	for y := range Rows {
		for x := range Cols {
			c := cols[y][x]
			g.Lit[y][x] = c.Luma() >= floor
			g.Col[y][x] = c
		}
	}
}

// Stamp lights the LEDs of a mask at (x, y) in colour c.
func (g *Grid) Stamp(m *Mask, x, y int, c RGB) {
	for my := 0; my < m.H; my++ {
		for mx := 0; mx < m.W; mx++ {
			if m.At(mx, my) {
				g.Set(x+mx, y+my, c)
			}
		}
	}
}

// Mask is a 1-bit LED bitmap (text, sprites).
type Mask struct {
	W, H int
	Bits []bool
}

// NewMask returns an empty mask.
func NewMask(w, h int) *Mask { return &Mask{W: w, H: h, Bits: make([]bool, w*h)} }

// At reports whether (x, y) is set.
func (m *Mask) At(x, y int) bool {
	if x < 0 || y < 0 || x >= m.W || y >= m.H {
		return false
	}
	return m.Bits[y*m.W+x]
}

// SetBit sets (x, y).
func (m *Mask) SetBit(x, y int, v bool) {
	if x >= 0 && y >= 0 && x < m.W && y < m.H {
		m.Bits[y*m.W+x] = v
	}
}

// InkRows returns the first and last rows that contain set bits, or -1, -1.
func (m *Mask) InkRows() (int, int) {
	top, bot := -1, -1
	for y := 0; y < m.H; y++ {
		for x := 0; x < m.W; x++ {
			if m.Bits[y*m.W+x] {
				if top < 0 {
					top = y
				}
				bot = y
				break
			}
		}
	}
	return top, bot
}

// dot is the 7x7 round LED shape.
var dot [Cell][Cell]bool

func init() {
	c := float64(Cell-1) / 2
	r := float64(Cell) * 0.42
	for y := range Cell {
		for x := range Cell {
			dx, dy := float64(x)-c, float64(y)-c
			dot[y][x] = dx*dx+dy*dy <= r*r
		}
	}
}

// Image expands the grid to the 1920 x 462 panel image as round LEDs on the ground colour.
func (g *Grid) Image(ground RGB) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, W, H))
	pix := img.Pix
	// fill ground
	for i := 0; i < len(pix); i += 4 {
		pix[i], pix[i+1], pix[i+2], pix[i+3] = ground.R, ground.G, ground.B, 255
	}
	for gy := range Rows {
		for gx := range Cols {
			c := g.Back[gy][gx]
			if g.Lit[gy][gx] {
				c = g.Col[gy][gx]
			}
			x0, y0 := gx*Cell, gy*Cell
			for dy := range Cell {
				py := y0 + dy
				if py >= H {
					break
				}
				row := py * img.Stride
				for dx := range Cell {
					px := x0 + dx
					if px >= W {
						break
					}
					if dot[dy][dx] {
						o := row + px*4
						pix[o], pix[o+1], pix[o+2] = c.R, c.G, c.B
					}
				}
			}
		}
	}
	return img
}

// ParseColour accepts a palette name, "#rrggbb", "rrggbb", "#rgb".
func ParseColour(s string) (RGB, bool) {
	if c, ok := Palette[lower(s)]; ok {
		return c, true
	}
	h := s
	if len(h) > 0 && h[0] == '#' {
		h = h[1:]
	}
	if len(h) == 3 {
		h = string([]byte{h[0], h[0], h[1], h[1], h[2], h[2]})
	}
	if len(h) != 6 {
		return RGB{}, false
	}
	var v [3]uint8
	for i := range 3 {
		hi, ok1 := hexv(h[2*i])
		lo, ok2 := hexv(h[2*i+1])
		if !ok1 || !ok2 {
			return RGB{}, false
		}
		v[i] = hi<<4 | lo
	}
	return RGB{v[0], v[1], v[2]}, true
}

func hexv(c byte) (uint8, bool) {
	switch {
	case c >= '0' && c <= '9':
		return c - '0', true
	case c >= 'a' && c <= 'f':
		return c - 'a' + 10, true
	case c >= 'A' && c <= 'F':
		return c - 'A' + 10, true
	}
	return 0, false
}

func lower(s string) string {
	b := []byte(s)
	for i, c := range b {
		if c >= 'A' && c <= 'Z' {
			b[i] = c + 32
		}
	}
	return string(b)
}

// ToColor converts for image APIs.
func (c RGB) ToColor() color.RGBA { return color.RGBA{c.R, c.G, c.B, 255} }
