package scenes

// Camera scenes: the webcam as a dot matrix, a pop-art triptych, motion trails and a
// time-displacement slit scan. They draw from CameraSource, which tests replace.

import (
	"math"

	"github.com/justynroberts/marquee/internal/camera"
	"github.com/justynroberts/marquee/internal/led"
)

// CameraSource supplies frames to the camera scenes. Defaults to the shared camera.
var CameraSource = camera.Latest

func init() {
	Register("camera", "camera", "The webcam as a dot matrix, auto-levelled, mirrored like a selfie view", func() Scene { return &camView{} })
	Register("popart", "camera", "Three posterised webcam panels in bold palettes that change every few seconds", func() Scene { return &popart{} })
	Register("motion", "camera", "Only what moves lights up, leaving trails that drift through the colours", func() Scene { return &motion{} })
	Register("slitscan", "camera", "Time displacement: live in the centre, seconds in the past towards the edges", func() Scene { return newSlitscan() })
}

func luma(r, g, b uint8) float64 { return 0.299*float64(r) + 0.587*float64(g) + 0.114*float64(b) }

// cover samples the frame into a w x h block, scaled to cover it and centre-cropped.
func cover(f camera.Frame, w, h int, dst []led.RGB) {
	s := math.Max(float64(w)/float64(f.W), float64(h)/float64(f.H))
	ox := (float64(f.W) - float64(w)/s) / 2
	oy := (float64(f.H) - float64(h)/s) / 2
	for y := range h {
		sy := int(oy + (float64(y)+0.5)/s)
		for x := range w {
			r, g, b := f.At(int(ox+(float64(x)+0.5)/s), sy)
			dst[y*w+x] = led.RGB{R: r, G: g, B: b}
		}
	}
}

// levels returns the 3rd and 97th percentile luma of px, at least 24 apart.
func levels(px []led.RGB) (lo, hi float64) {
	var hist [256]int
	for _, c := range px {
		hist[int(luma(c.R, c.G, c.B))]++
	}
	lo, hi = percentileHist(&hist, len(px), 0.03), percentileHist(&hist, len(px), 0.97)
	if hi-lo < 24 {
		mid := (lo + hi) / 2
		lo, hi = mid-12, mid+12
	}
	return lo, hi
}

func percentileHist(h *[256]int, n int, p float64) float64 {
	want, acc := int(p*float64(n)), 0
	for v, c := range h {
		acc += c
		if acc > want {
			return float64(v)
		}
	}
	return 255
}

// cameraProblem draws a message instead of the picture when there is none, and reports
// whether it did.
func cameraProblem(g *led.Grid, f camera.Frame, t float64) bool {
	switch {
	case f.Denied:
		message(g, []string{"CAMERA ACCESS NEEDED", "System Settings > Privacy & Security", "> Camera: allow Marquee"}, amber)
	case f.Err == camera.ErrPending:
		message(g, []string{"CAMERA", "Allow camera access in the dialog"}, amber)
	case f.Err != nil:
		e := f.Err.Error()
		if r := []rune(e); len(r) > 60 {
			e = string(r[:60])
		}
		message(g, []string{"CAMERA", e}, alert)
	case f.W == 0:
		if math.Mod(t, 1) < 0.6 {
			message(g, []string{"CAMERA", "starting..."}, ink3)
		}
	default:
		return false
	}
	return true
}

// ---------------------------------------------------------------------------
// camera: the picture itself, full colour with auto levels.

type camView struct {
	px     []led.RGB
	lo, hi float64
}

func (c *camView) Draw(g *led.Grid, t, dt float64) {
	f := CameraSource()
	if cameraProblem(g, f, t) {
		return
	}
	if c.px == nil {
		c.px = make([]led.RGB, rows*cols)
		c.lo, c.hi = -1, -1
	}
	cover(f, cols, rows, c.px)
	lo, hi := levels(c.px)
	if c.lo < 0 {
		c.lo, c.hi = lo, hi
	}
	k := math.Min(1, dt*3) // ease the levels so exposure changes do not flicker
	c.lo += (lo - c.lo) * k
	c.hi += (hi - c.hi) * k
	gain := 255 / (c.hi - c.lo)
	for y := range rows {
		for x := range cols {
			p := c.px[y*cols+x]
			col := led.RGB{
				R: u8((float64(p.R) - c.lo) * gain),
				G: u8((float64(p.G) - c.lo) * gain),
				B: u8((float64(p.B) - c.lo) * gain),
			}
			if col.Luma() >= 22 {
				g.Set(x, y, col)
			}
		}
	}
}

// ---------------------------------------------------------------------------
// popart: three panels, each posterised to four tones in its own palette.

type popart struct{ px []led.RGB }

var popPalettes = [][4]led.RGB{
	{{R: 20, G: 10, B: 90}, {R: 255, G: 40, B: 140}, {R: 255, G: 200, B: 0}, {R: 255, G: 250, B: 220}},
	{{R: 0, G: 70, B: 60}, {R: 255, G: 91, B: 56}, {R: 120, G: 230, B: 255}, {R: 255, G: 255, B: 255}},
	{{R: 90, G: 0, B: 40}, {R: 60, G: 200, B: 90}, {R: 255, G: 130, B: 0}, {R: 255, G: 240, B: 120}},
	{{R: 10, G: 30, B: 110}, {R: 255, G: 214, B: 60}, {R: 255, G: 70, B: 70}, {R: 240, G: 240, B: 255}},
	{{R: 60, G: 0, B: 90}, {R: 0, G: 200, B: 200}, {R: 255, G: 120, B: 200}, {R: 255, G: 255, B: 180}},
}

func (p *popart) Draw(g *led.Grid, t, dt float64) {
	f := CameraSource()
	if cameraProblem(g, f, t) {
		return
	}
	const gap = 3
	pw := (cols - 2*gap) / 3
	if p.px == nil {
		p.px = make([]led.RGB, pw*rows)
	}
	cover(f, pw, rows, p.px)
	// Tone thresholds at the quartiles, so every panel uses all four colours.
	var hist [256]int
	for _, c := range p.px {
		hist[int(luma(c.R, c.G, c.B))]++
	}
	q1 := percentileHist(&hist, len(p.px), 0.25)
	q2 := percentileHist(&hist, len(p.px), 0.5)
	q3 := percentileHist(&hist, len(p.px), 0.75)
	shift := int(t / 4)
	x0 := (cols - 3*pw - 2*gap) / 2
	for panel := range 3 {
		pal := popPalettes[(shift+panel*2)%len(popPalettes)]
		left := x0 + panel*(pw+gap)
		for y := range rows {
			for x := range pw {
				c := p.px[y*pw+x]
				l := luma(c.R, c.G, c.B)
				tone := 0
				switch {
				case l > q3:
					tone = 3
				case l > q2:
					tone = 2
				case l > q1:
					tone = 1
				}
				g.Set(left+x, y, pal[tone])
			}
		}
	}
}

// ---------------------------------------------------------------------------
// motion: frame differencing with decaying, hue-shifting trails.

// motionHues is hsv(h, 0.85, 1) round the wheel; the trail brightness scales it.
var motionHues = func() (lut [256]led.RGB) {
	for i := range lut {
		lut[i] = hsv(float64(i)/256, 0.85, 1)
	}
	return lut
}()

type motion struct {
	prev  []float64
	heat  []float64
	hue   []float64
	px    []led.RGB
	seq   uint64
	noise float64
}

func (m *motion) Draw(g *led.Grid, t, dt float64) {
	f := CameraSource()
	if cameraProblem(g, f, t) {
		return
	}
	n := rows * cols
	if m.px == nil {
		m.px = make([]led.RGB, n)
		m.prev = make([]float64, n)
		m.heat = make([]float64, n)
		m.hue = make([]float64, n)
		m.noise = 8
	}
	decay := math.Exp(-dt * 2.2)
	for i := range m.heat {
		m.heat[i] *= decay
	}
	if f.Seq != m.seq {
		first := m.seq == 0
		m.seq = f.Seq
		cover(f, cols, rows, m.px)
		var sum float64
		for i, c := range m.px {
			l := luma(c.R, c.G, c.B)
			d := math.Abs(l - m.prev[i])
			m.prev[i] = l
			if first {
				continue
			}
			sum += d
			if v := (d - m.noise*2.5) / 40; v > m.heat[i] {
				m.heat[i] = math.Min(1, v)
				m.hue[i] = t * 0.08
			}
		}
		if !first { // track sensor noise so a still room stays dark
			m.noise += (sum/float64(n) - m.noise) * 0.05
			m.noise = math.Max(2, m.noise)
		}
	}
	for y := range rows {
		for x := range cols {
			i := y*cols + x
			if h := m.heat[i]; h > 0.05 {
				hue := m.hue[i] + (1-h)*0.35
				c := motionHues[int((hue-math.Floor(hue))*float64(len(motionHues)))%len(motionHues)]
				g.Set(x, y, c.Scale(math.Min(1, h*1.3)))
			}
		}
	}
}

// ---------------------------------------------------------------------------
// slitscan: each column is delayed by its distance from the centre.

type slitscan struct {
	ring [][]led.RGB
	head int
	n    int
	seq  uint64
}

func newSlitscan() *slitscan {
	s := &slitscan{ring: make([][]led.RGB, cols/2+1)}
	for i := range s.ring {
		s.ring[i] = make([]led.RGB, rows*cols)
	}
	return s
}

func (s *slitscan) Draw(g *led.Grid, t, dt float64) {
	f := CameraSource()
	if cameraProblem(g, f, t) {
		return
	}
	if f.Seq != s.seq || s.n == 0 {
		s.seq = f.Seq
		s.head = (s.head + 1) % len(s.ring)
		cover(f, cols, rows, s.ring[s.head])
		s.n = min(s.n+1, len(s.ring))
	}
	mid := cols / 2
	for x := range cols {
		d := min(abs(x-mid), s.n-1)
		fr := s.ring[(s.head-d+len(s.ring))%len(s.ring)]
		for y := range rows {
			if c := fr[y*cols+x]; c.Luma() >= 22 {
				g.Set(x, y, c)
			}
		}
	}
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
