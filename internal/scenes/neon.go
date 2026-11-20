package scenes

// Two more camera scenes: neon (you traced in glowing outlines) and clones (delayed copies of
// you in colour, trailing every move).

import (
	"math"

	"github.com/justynroberts/marquee/internal/camera"
	"github.com/justynroberts/marquee/internal/led"
)

func init() {
	Register("neon", "camera", "You traced in glowing neon outlines that shift colour and leave light trails", func() Scene { return &neon{} })
	Register("clones", "camera", "Delayed copies of you in colour, trailing every move", func() Scene { return &clones{} })
}

// ---------------------------------------------------------------------------
// neon: edges of the picture (Sobel on brightness), the person's outline brightest, in a hue
// that drifts across the panel and through time, with fading trails.

type neon struct {
	lum   [rows][cols]float64
	body  [rows][cols]bool
	trail field
	seq   uint64
}

func (s *neon) Draw(g *led.Grid, t, dt float64) {
	f := CameraSource(camera.Mask)
	if cameraProblem(g, f, t) {
		return
	}
	s.trail.scaleAll(math.Pow(0.04, dt))
	if f.Seq != s.seq {
		s.seq = f.Seq
		v := fitView(f, 1.6)
		for y := range rows {
			for x := range cols {
				fx, fy := v.frame(x, y)
				if fx < 0 || fx >= f.W || fy < 0 || fy >= f.H {
					s.lum[y][x], s.body[y][x] = 0, false
					continue
				}
				r, gg, b := f.At(fx, fy)
				s.lum[y][x] = luma(r, gg, b)
				s.body[y][x] = f.MaskAt(fx, fy)
			}
		}
	}
	at := func(x, y int) float64 {
		x, y = min(max(x, 0), cols-1), min(max(y, 0), rows-1)
		return s.lum[y][x]
	}
	for y := range rows {
		for x := range cols {
			gx := at(x+1, y-1) + 2*at(x+1, y) + at(x+1, y+1) - at(x-1, y-1) - 2*at(x-1, y) - at(x-1, y+1)
			gy := at(x-1, y+1) + 2*at(x, y+1) + at(x+1, y+1) - at(x-1, y-1) - 2*at(x, y-1) - at(x+1, y-1)
			e := math.Hypot(gx, gy) / 255
			edge := s.body[y][x] != s.body[min(y+1, rows-1)][x] || s.body[y][x] != s.body[y][min(x+1, cols-1)]
			k := 0.0
			switch {
			case edge && f.Mask != nil:
				k = 1 // the person's outline
			case e > 0.35:
				k = math.Min(1, (e-0.35)*1.6)
				if f.Mask != nil && !s.body[y][x] {
					k *= 0.35 // the room, dimmer
				}
			}
			if k <= 0 {
				continue
			}
			h := math.Mod(float64(x)/cols*0.6+float64(y)/rows*0.2+t*0.08, 1)
			c := fc(hsv(h, 0.85, 1)).scale(k)
			s.trail[y][x] = s.trail[y][x].maxc(c)
		}
	}
	s.trail.toGrid(g, 20)
}

// ---------------------------------------------------------------------------
// clones: the person mask is kept for the last couple of seconds; copies from further back are
// drawn first, each in its own colour, and you in camera colour on top.

type clones struct {
	hist [clonesKeep][rows][cols]bool
	n    int // masks recorded
	body [rows][cols]bool
	pix  [rows][cols]led.RGB
	seq  uint64
	lag  float64 // seconds since the last mask was recorded
}

const (
	clonesKeep  = 40   // masks
	clonesEvery = 0.06 // seconds between them: 2.4 s of history
)

var cloneHues = []led.RGB{{R: 255, G: 60, B: 190}, {R: 150, G: 80, B: 255}, {R: 60, G: 160, B: 255}, {R: 40, G: 230, B: 200}, {R: 140, G: 240, B: 70}}

func (s *clones) Draw(g *led.Grid, t, dt float64) {
	f := CameraSource(camera.Mask)
	if cameraProblem(g, f, t) {
		return
	}
	if f.Seq != s.seq {
		s.seq = f.Seq
		silhouette(f, fitView(f, 1.6), &s.body, &s.pix)
	}
	s.lag += dt
	if s.lag >= clonesEvery {
		s.lag = 0
		s.hist[s.n%clonesKeep] = s.body
		s.n++
	}
	// oldest copy first, so nearer ones cover it
	for i := len(cloneHues); i >= 1; i-- {
		back := i * clonesKeep / (len(cloneHues) + 1)
		if back >= s.n {
			continue
		}
		m := &s.hist[(s.n-1-back+clonesKeep)%clonesKeep]
		c := cloneHues[i-1]
		k := 1 - float64(i-1)*0.12
		col := led.RGB{R: u8(float64(c.R) * k), G: u8(float64(c.G) * k), B: u8(float64(c.B) * k)}
		for y := range rows {
			for x := range cols {
				if m[y][x] {
					g.Set(x, y, col)
				}
			}
		}
	}
	for y := range rows {
		for x := range cols {
			if s.body[y][x] {
				p := s.pix[y][x]
				g.Set(x, y, led.RGB{R: u8(float64(p.R)*0.85 + 30), G: u8(float64(p.G)*0.85 + 30), B: u8(float64(p.B)*0.85 + 30)})
			}
		}
	}
}
