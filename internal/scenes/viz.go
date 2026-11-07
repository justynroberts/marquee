package scenes

// Audio visualisers, ported line by line from thermalright-display/viz.py. Arithmetic follows
// numpy's: colour tables and outputs are truncated to uint8 (astype), np.round is half-to-even,
// and LEDs dimmer than 22 show as unlit (scenes.to_image).

import (
	"math"
	"math/rand/v2"

	"github.com/justynroberts/marquee/internal/audio"
	"github.com/justynroberts/marquee/internal/led"
)

// AudioSource supplies features to the visualisers. Defaults to the shared microphone engine;
// tests replace it.
var AudioSource = func() audio.Features { return audio.Shared().Analyse() }

func init() {
	Register("mirror", "audio", "Symmetric bars from the centre, rainbow by frequency; beats flash the background", func() Scene { return &mirror{} })
	Register("waterfall", "audio", "Scrolling spectrogram: time runs right to left, bass at the bottom", func() Scene { return &waterfall{} })
	Register("scope", "audio", "Triggered oscilloscope with phosphor afterglow", func() Scene { return &scope{gain: 4} })
	Register("pulse", "audio", "Every beat fires a ring and sparks; bass glows the core; treble twinkles", func() Scene { return newPulse() })
	Register("meter", "audio", "Segmented LED graphic equaliser with peak hold", func() Scene { return newMeter() })
	Register("spectrum", "audio", "Classic bars with falling peak dots", func() Scene { return newSpectrum() })
}

func pow(x, p float64) float64 { return math.Pow(math.Max(x, 0), p) }

// trunc8 is numpy's clip(0, 255).astype(uint8).
func trunc8(v float64) uint8 {
	if !(v > 0) {
		return 0
	}
	if v >= 255 {
		return 255
	}
	return uint8(v)
}

func (c fcol) trunc() fcol {
	return fcol{float64(trunc8(c.r)), float64(trunc8(c.g)), float64(trunc8(c.b))}
}

// vhsv is scenes.hsv for one value, truncated to uint8 as every caller does.
func vhsv(h, s, v float64) fcol {
	h = math.Mod(h, 1)
	if h < 0 {
		h++
	}
	h *= 6
	i := int(math.Floor(h)) % 6
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
	return fcol{r * 255, g * 255, b * 255}.trunc()
}

// palette is scenes.palette: n entries interpolated between stops, truncated to uint8.
func palette(n int, stops ...any) []fcol {
	type stop struct {
		at float64
		c  [3]float64
	}
	var st []stop
	for i := 0; i+1 < len(stops); i += 2 {
		c := stops[i+1].([3]float64)
		st = append(st, stop{stops[i].(float64), c})
	}
	out := make([]fcol, n)
	for k := range out {
		x := 0.0
		if n > 1 {
			x = float64(k) / float64(n-1)
		}
		var c [3]float64
		switch {
		case x <= st[0].at:
			c = st[0].c
		case x >= st[len(st)-1].at:
			c = st[len(st)-1].c
		default:
			for j := 1; j < len(st); j++ {
				if x <= st[j].at {
					a, b := st[j-1], st[j]
					u := (x - a.at) / (b.at - a.at)
					for ch := range 3 {
						c[ch] = a.c[ch] + (b.c[ch]-a.c[ch])*u
					}
					break
				}
			}
		}
		out[k] = fcol{c[0], c[1], c[2]}.trunc()
	}
	return out
}

// hueColours is viz.hue_colours.
func hueColours(n int, offset, s float64) []fcol {
	out := make([]fcol, n)
	for i := range out {
		out[i] = vhsv(float64(i)/float64(n)*0.85+offset, s, 1)
	}
	return out
}

// roundEven is numpy's round (half to even).
func roundEven(v float64) int { return int(math.RoundToEven(v)) }

// emit converts a float frame like .clip(0,255).astype(uint8) followed by to_image(floor=22).
func emit(g *led.Grid, f *field) {
	for y := range rows {
		for x := range cols {
			c := led.RGB{R: trunc8(f[y][x].r), G: trunc8(f[y][x].g), B: trunc8(f[y][x].b)}
			if c.Luma() >= 22 {
				g.Lit[y][x] = true
				g.Col[y][x] = c
			}
		}
	}
}

// emitRGB is to_image(floor=22) for a uint8 frame.
func emitRGB(g *led.Grid, img *[rows][cols]led.RGB) {
	for y := range rows {
		for x := range cols {
			if c := img[y][x]; c.Luma() >= 22 {
				g.Lit[y][x] = true
				g.Col[y][x] = c
			}
		}
	}
}

// audioProblem draws viz.problem_screen and reports whether it did.
func audioProblem(g *led.Grid, f audio.Features) bool {
	switch {
	case f.Err != "":
		e := f.Err
		if r := []rune(e); len(r) > 60 {
			e = string(r[:60])
		}
		message(g, []string{"AUDIO", e}, led.RGB{R: 255, G: 91, B: 56})
	case f.Silent:
		message(g, []string{"MICROPHONE ACCESS NEEDED", "System Settings > Privacy & Security", "> Microphone: allow Marquee"}, led.RGB{R: 255, G: 214, B: 60})
	default:
		return false
	}
	return true
}

func bandsTo(f audio.Features, n int) []float64 { return interp(f.Bands[:], n) }

// ---------------------------------------------------------------------------
// Spectrum: classic bars with falling peak dots, green -> yellow -> red by height.

type spectrum struct {
	peak []float64
	grad []fcol
}

const spBar, spGap = 3, 1

func newSpectrum() *spectrum {
	return &spectrum{
		peak: make([]float64, cols/(spBar+spGap)),
		grad: palette(rows, 0.0, [3]float64{70, 220, 110}, 0.6, [3]float64{255, 214, 60}, 1.0, [3]float64{255, 91, 56}),
	}
}

func (s *spectrum) Draw(g *led.Grid, t, dt float64) {
	f := AudioSource()
	if audioProblem(g, f) {
		return
	}
	n := len(s.peak)
	levels := bandsTo(f, n)
	var out [rows][cols]led.RGB
	for i := 0; i < n; i++ {
		s.peak[i] = math.Max(levels[i], s.peak[i]-dt*0.5)
		h := int(levels[i] * (rows - 2))
		pk := int(s.peak[i] * (rows - 2))
		x0 := i * (spBar + spGap)
		for k := 0; k < h; k++ { // row rows-1-k gets grad[k]
			c := s.grad[k].rgb()
			for x := x0; x < x0+spBar; x++ {
				out[rows-1-k][x] = c
			}
		}
		if py := rows - 1 - pk; py >= 0 && py < rows {
			for x := x0; x < x0+spBar; x++ {
				out[py][x] = led.RGB{R: 236, G: 232, B: 220}
			}
		}
	}
	emitRGB(g, &out)
}

// ---------------------------------------------------------------------------
// Mirror: bass in the centre spreading to treble at both edges, mirrored about the horizontal
// centre line, rainbow by frequency; beats flash the background.

type mirror struct {
	flash, hue float64
	out        field
}

func (m *mirror) Draw(g *led.Grid, t, dt float64) {
	f := AudioSource()
	if audioProblem(g, f) {
		return
	}
	if f.Beat {
		m.flash = 1
		m.hue = math.Mod(m.hue+0.13, 1)
	}
	m.flash *= math.Exp(-dt * 6)
	half := cols / 2
	nBars := (half + 1) / 2
	bands := bandsTo(f, nBars)
	right := make([]float64, half)
	for i := 0; i < nBars; i++ {
		right[2*i] = bands[i] // 1-LED gaps between bars
	}
	colour := hueColours(half, m.hue, 0.9)
	var heights [cols]float64
	var cc [cols]fcol
	for x := 0; x < cols; x++ {
		switch {
		case x < half:
			heights[x], cc[x] = right[half-1-x], colour[half-1-x]
		case x < 2*half:
			heights[x], cc[x] = right[x-half], colour[x-half]
		}
	}
	mid := float64(rows) / 2
	var bg fcol
	if m.flash > 0.05 {
		bg = vhsv(m.hue, 0.8, 1).scale(0.18 * m.flash)
	}
	m0, m1 := int(mid)-1, int(mid)+1
	for y := range rows {
		dist := math.Abs(float64(y)-mid+0.5) / mid
		fade := clamp01(1.15 - dist*0.6)
		for x := range cols {
			lit := dist < heights[x]*1.02
			var c fcol
			if lit {
				c = cc[x].scale(fade)
			}
			if y >= m0 && y < m1 {
				c = c.maxc(cc[x].scale(0.35))
			}
			if !lit && m.flash > 0.05 {
				c = c.add(bg)
			}
			m.out[y][x] = c
		}
	}
	emit(g, &m.out)
}

// ---------------------------------------------------------------------------
// Waterfall: scrolling spectrogram, time right to left, low frequencies at the bottom.

type waterfall struct {
	img [rows][cols]led.RGB
}

var waterfallLUT = palette(256, 0.0, [3]float64{0, 0, 0}, 0.2, [3]float64{40, 10, 90}, 0.45, [3]float64{160, 20, 120},
	0.7, [3]float64{255, 91, 56}, 0.88, [3]float64{255, 214, 60}, 1.0, [3]float64{255, 255, 230})

const wfSpeed = 2

func (w *waterfall) Draw(g *led.Grid, t, dt float64) {
	f := AudioSource()
	if audioProblem(g, f) {
		return
	}
	col := bandsTo(f, rows)
	for y := range rows {
		copy(w.img[y][:], w.img[y][wfSpeed:])
		v := col[rows-1-y] // reversed: bass at the bottom
		c := waterfallLUT[trunc8(pow(v, 1.2)*255)].rgb()
		for x := cols - wfSpeed; x < cols; x++ {
			w.img[y][x] = c
		}
	}
	if f.Beat { // beat tick along the top
		for y := 0; y < 2; y++ {
			for x := cols - wfSpeed; x < cols; x++ {
				w.img[y][x] = led.RGB{R: 236, G: 232, B: 220}
			}
		}
	}
	emitRGB(g, &w.img)
}

// ---------------------------------------------------------------------------
// Scope: oscilloscope with phosphor afterglow, triggered on a rising zero crossing.

type scope struct {
	glow [rows][cols]float64
	gain float64
	out  field
}

func (s *scope) Draw(g *led.Grid, t, dt float64) {
	f := AudioSource()
	if audioProblem(g, f) {
		return
	}
	w := f.Wave
	span := cols * 3
	if len(w) < span {
		return
	}
	// trigger: last rising zero crossing in the older part, then COLS*3 samples
	start := len(w) - span
	search := w[:len(w)-span]
	for i := len(search) - 2; i >= 0; i-- {
		if search[i] < 0 && search[i+1] >= 0 {
			start = i
			break
		}
	}
	var seg [cols]float64
	peak := 0.0
	for x := range cols {
		a := w[start+x*3 : start+x*3+3]
		seg[x] = (float64(a[0]) + float64(a[1]) + float64(a[2])) / 3
		peak = math.Max(peak, math.Abs(seg[x]))
	}
	peak += 1e-6
	target := 0.85 / peak
	s.gain += (math.Min(target, 60) - s.gain) * 0.15
	var y [cols]float64
	for x := range cols {
		y[x] = math.Min(math.Max(float64(rows)/2-seg[x]*s.gain*(float64(rows)/2-3), 1), rows-2)
	}
	var frame [rows][cols]bool
	for x := range cols {
		y0 := y[x]
		y1 := y0
		if x+1 < cols {
			y1 = y[x+1]
		}
		lo, hi := int(math.Min(y0, y1)), int(math.Max(y0, y1))+1
		for yy := max(lo, 0); yy < min(hi, rows); yy++ {
			frame[yy][x] = true
		}
	}
	for yy := range rows {
		for x := range cols {
			s.glow[yy][x] *= 0.62
			if frame[yy][x] {
				s.glow[yy][x] = math.Max(s.glow[yy][x], 1)
			}
		}
	}
	green := fcol{70, 255, 120}
	gridC := fcol{25, 45, 30}
	for yy := range rows {
		for x := range cols {
			// graticule
			var c fcol
			if (yy == rows/2 && x%4 == 0) || (yy%4 == 0 && x%(cols/10) == 0) {
				c = gridC
			}
			// 3x3 halo with wrap-around (np.roll)
			h := 0.0
			for dy := -1; dy <= 1; dy++ {
				for dx := -1; dx <= 1; dx++ {
					h += s.glow[(yy+dy+rows)%rows][(x+dx+cols)%cols]
				}
			}
			level := math.Max(s.glow[yy][x], h/9*0.35)
			c = c.maxc(green.scale(level))
			if frame[yy][x] {
				c = fcol{200, 255, 210}
			}
			s.out[yy][x] = c
		}
	}
	emit(g, &s.out)
}

// ---------------------------------------------------------------------------
// Pulse: every beat fires a ring from the centre (and sparks); bass glows the core; treble
// twinkles.

type ring struct{ r, hue, strength float64 }

type vspark struct{ x, y, vx, vy, life, hue float64 }

type pulse struct {
	rings  []ring
	sparks []vspark
	hue    float64
	out    field
}

// pulseDist is the elliptical distance from the centre, so rings fill the wide strip.
var pulseDist = func() (d [rows][cols]float64) {
	cx, cy := float64(cols)/2, float64(rows)/2
	for y := range rows {
		for x := range cols {
			dx, dy := (float64(x)-cx)/4, float64(y)-cy
			d[y][x] = math.Sqrt(dx*dx + dy*dy)
		}
	}
	return
}()

func newPulse() *pulse { return &pulse{hue: rand.Float64()} }

func (p *pulse) Draw(g *led.Grid, t, dt float64) {
	f := AudioSource()
	if audioProblem(g, f) {
		return
	}
	cx, cy := float64(cols)/2, float64(rows)/2
	if f.Beat {
		p.hue = math.Mod(p.hue+0.11, 1)
		p.rings = append(p.rings, ring{1, p.hue, 0.85 + f.Bass*0.4})
		for range 24 {
			a := uniform(0, 2*math.Pi)
			spd := uniform(40, 120)
			p.sparks = append(p.sparks, vspark{cx, cy, math.Cos(a) * spd, math.Sin(a) * spd * 0.35, 1, p.hue})
		}
	}
	base := vhsv(p.hue, 0.75, 1)
	coreR := 5 + 22*f.Bass
	coreK := 0.45 + 0.55*f.Bass
	for y := range rows {
		for x := range cols {
			core := pow(clamp01(1-pulseDist[y][x]/coreR), 1.2)
			p.out[y][x] = base.scale(core * coreK)
		}
	}
	alive := p.rings[:0]
	for _, r := range p.rings {
		r.r += dt * (26 + 14*r.strength)
		fade := math.Max(0, 1-r.r/38)
		if fade <= 0 {
			continue
		}
		alive = append(alive, r)
		col := vhsv(r.hue, 0.85, 1)
		k := (0.35 + 0.65*fade) * math.Min(1, r.strength)
		for y := range rows {
			for x := range cols {
				d := math.Abs(pulseDist[y][x] - r.r)
				if d < 2.4 {
					p.out[y][x] = p.out[y][x].maxc(col.scale(pow(1-d/2.4, 0.8) * k))
				}
			}
		}
	}
	p.rings = alive
	live := p.sparks[:0]
	for _, s := range p.sparks {
		s.x += s.vx * dt
		s.y += s.vy * dt
		s.life -= dt * 0.9
		if !(s.life > 0 && s.x >= 0 && s.x < cols && s.y >= 0 && s.y < rows) {
			continue
		}
		live = append(live, s)
		yi, xi := int(s.y), int(s.x)
		p.out[yi][xi] = p.out[yi][xi].maxc(vhsv(s.hue, 0.5, s.life))
	}
	p.sparks = live
	// treble twinkle
	if n := int(f.Treble * 60); n > 0 {
		for range n {
			y, x := rand.IntN(rows), rand.IntN(cols)
			p.out[y][x] = p.out[y][x].maxc(fcol{200, 200, 200})
		}
	}
	emit(g, &p.out)
}

// ---------------------------------------------------------------------------
// Meter: segmented LED graphic equaliser, green/amber/red zones, peak hold.

type meter struct {
	n, segs int
	peak    []float64
	hold    []float32 // float32 like the numpy array, so the hold expires on the same frame
}

const (
	mtColW, mtColGap = 7, 2
	mtSegH, mtSegGap = 4, 1
)

func newMeter() *meter {
	n := cols / (mtColW + mtColGap)
	return &meter{n: n, segs: rows / (mtSegH + mtSegGap), peak: make([]float64, n), hold: make([]float32, n)}
}

func (m *meter) Draw(g *led.Grid, t, dt float64) {
	f := AudioSource()
	if audioProblem(g, f) {
		return
	}
	levels := bandsTo(f, m.n)
	xPad := (cols - m.n*(mtColW+mtColGap)) / 2
	var out [rows][cols]led.RGB
	for i := 0; i < m.n; i++ {
		lit := roundEven(levels[i] * float64(m.segs))
		if levels[i] >= m.peak[i] {
			m.peak[i], m.hold[i] = levels[i], 0.5
		} else {
			m.hold[i] -= float32(dt)
		}
		if m.hold[i] < 0 {
			m.peak[i] = math.Max(levels[i], m.peak[i]-dt*0.35)
		}
		pk := roundEven(m.peak[i] * float64(m.segs))
		x0 := xPad + i*(mtColW+mtColGap)
		for s := 0; s < m.segs; s++ {
			y1 := rows - s*(mtSegH+mtSegGap)
			y0 := y1 - mtSegH
			frac := float64(s) / float64(max(1, m.segs-1))
			on, off := led.RGB{R: 70, G: 220, B: 110}, led.RGB{R: 16, G: 32, B: 20}
			switch {
			case frac > 0.82:
				on, off = led.RGB{R: 255, G: 70, B: 56}, led.RGB{R: 34, G: 20, B: 18}
			case frac > 0.6:
				on, off = led.RGB{R: 255, G: 200, B: 50}, led.RGB{R: 34, G: 30, B: 16}
			}
			c := off
			if s < lit || s == pk-1 {
				c = on
			}
			for y := max(0, y0); y < y1; y++ {
				for x := x0; x < x0+mtColW; x++ {
					out[y][x] = c
				}
			}
		}
	}
	emitRGB(g, &out)
}
