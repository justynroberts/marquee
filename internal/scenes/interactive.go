package scenes

// Interactive camera scenes: snow that settles on you, bubbles you pop, and painting in the air
// with a pinch. The silhouette comes from Vision person segmentation and the fingertips from
// Vision hand pose (camera.Mask, camera.Hands), computed only while these scenes play.

import (
	"fmt"
	"math"
	"math/rand/v2"

	"github.com/justynroberts/marquee/internal/camera"
	"github.com/justynroberts/marquee/internal/led"
)

func init() {
	Register("snowfall", "camera", "Snow falls and settles on your head and shoulders; move and it slides off", func() Scene { return &snowfall{} })
	Register("bubbles", "camera", "Bubbles rise across the panel; touch them with any part of you to pop them", func() Scene { return &bubbles{} })
	Register("airpaint", "camera", "Pinch thumb and index finger to draw light in the air; two hands, two colours", func() Scene { return &airpaint{} })
}

// view maps camera frame pixels onto the grid: gx = fx*sx + ox, gy = fy*sy + oy.
type view struct{ sx, sy, ox, oy float64 }

// fitView shows the whole frame at full panel height, stretched horizontally by stretch
// (capped at the panel width) and centred.
func fitView(f camera.Frame, stretch float64) view {
	sy := float64(rows) / float64(f.H)
	sx := math.Min(sy*stretch, float64(cols)/float64(f.W))
	return view{sx: sx, sy: sy, ox: (float64(cols) - sx*float64(f.W)) / 2}
}

func (v view) frame(gx, gy int) (int, int) {
	return int((float64(gx) + 0.5 - v.ox) / v.sx), int((float64(gy) + 0.5 - v.oy) / v.sy)
}

func (v view) grid(p camera.Point, f camera.Frame) (float64, float64) {
	return p.X*float64(f.W)*v.sx + v.ox, p.Y*float64(f.H)*v.sy + v.oy
}

// silhouette fills body with the person mask in grid cells, and pix with the camera colours
// (black outside the frame), auto-levelled on the person so they show bright even in a dim
// room. It reports whether the frame carried a mask.
func silhouette(f camera.Frame, v view, body *[rows][cols]bool, pix *[rows][cols]led.RGB) bool {
	defer levelBody(body, pix)
	for y := range rows {
		for x := range cols {
			fx, fy := v.frame(x, y)
			if fx < 0 || fx >= f.W || fy < 0 || fy >= f.H {
				body[y][x], pix[y][x] = false, led.RGB{}
				continue
			}
			body[y][x] = f.MaskAt(fx, fy)
			r, g, b := f.At(fx, fy)
			pix[y][x] = led.RGB{R: r, G: g, B: b}
		}
	}
	return f.Mask != nil
}

// levelBody stretches pix so the person's 3rd-97th percentile brightness fills the range
// (the whole picture when nobody is masked).
func levelBody(body *[rows][cols]bool, pix *[rows][cols]led.RGB) {
	var sample []led.RGB
	for y := range rows {
		for x := range cols {
			if body[y][x] {
				sample = append(sample, pix[y][x])
			}
		}
	}
	if len(sample) < 50 {
		sample = sample[:0]
		for y := range rows {
			sample = append(sample, pix[y][:]...)
		}
	}
	lo, hi := levels(sample)
	gain := 255 / (hi - lo)
	for y := range rows {
		for x := range cols {
			p := &pix[y][x]
			*p = led.RGB{R: u8((float64(p.R) - lo) * gain), G: u8((float64(p.G) - lo) * gain), B: u8((float64(p.B) - lo) * gain)}
		}
	}
}

// ---------------------------------------------------------------------------
// snowfall: falling-sand snow that piles on the silhouette and the floor.

const snowMelt = 0.004 // per settled flake per frame

type snowfall struct {
	snow [rows][cols]bool
	body [rows][cols]bool
	pix  [rows][cols]led.RGB
	seq  uint64
	n    int
}

func (s *snowfall) solid(x, y int) bool {
	if y >= rows {
		return true
	}
	if x < 0 || x >= cols || y < 0 {
		return false
	}
	return s.snow[y][x] || s.body[y][x]
}

func (s *snowfall) Draw(g *led.Grid, t, dt float64) {
	f := CameraSource(camera.Mask)
	if cameraProblem(g, f, t) {
		return
	}
	if f.Seq != s.seq {
		s.seq = f.Seq
		silhouette(f, fitView(f, 1.6), &s.body, &s.pix)
		// Snow the body has moved into is pushed up to the first free cell above it, so a
		// rising head lifts its snow; with no room it is shaken off.
		for y := range rows {
			for x := range cols {
				if !s.snow[y][x] || !s.body[y][x] {
					continue
				}
				s.snow[y][x] = false
				for yy := y - 1; yy >= max(0, y-8); yy-- {
					if !s.solid(x, yy) {
						s.snow[yy][x] = true
						break
					}
				}
			}
		}
	}
	s.n++
	// Fall, bottom row up, alternating sweep direction so piles slump evenly.
	for y := rows - 1; y >= 0; y-- {
		for i := range cols {
			x := i
			if s.n%2 == 1 {
				x = cols - 1 - i
			}
			if !s.snow[y][x] {
				continue
			}
			if !s.solid(x, y+1) {
				s.snow[y][x] = false
				if y+1 < rows {
					nx := x
					if rand.IntN(6) == 0 { // a little drift
						nx = min(max(x+rand.IntN(3)-1, 0), cols-1)
						if s.solid(nx, y+1) {
							nx = x
						}
					}
					s.snow[y+1][nx] = true
				}
				continue
			}
			d := 1
			if rand.IntN(2) == 0 {
				d = -1
			}
			moved := false
			for _, dx := range []int{d, -d} {
				if nx := x + dx; nx >= 0 && nx < cols && !s.solid(nx, y) && !s.solid(nx, y+1) {
					s.snow[y][x], s.snow[y+1][nx] = false, true
					moved = true
					break
				}
			}
			// Settled snow melts (about 20 s a flake), so drifts level off instead of
			// burying the panel.
			if !moved && rand.Float64() < snowMelt {
				s.snow[y][x] = false
			}
		}
	}
	// New flakes, and the floor slowly melts so it never fills up.
	for x := range cols {
		if rand.Float64() < 0.035 && !s.solid(x, 0) {
			s.snow[0][x] = true
		}
		if s.snow[rows-1][x] && rand.Float64() < 0.03 {
			s.snow[rows-1][x] = false
		}
	}
	for y := range rows {
		for x := range cols {
			switch {
			case s.snow[y][x]:
				g.Set(x, y, led.RGB{R: 235, G: 240, B: 255})
			case s.body[y][x]:
				p := s.pix[y][x]
				// near full colour with a cool cast, so pure-white snow still reads on top
				c := led.RGB{R: u8(float64(p.R)*0.78 + 8), G: u8(float64(p.G)*0.84 + 12), B: u8(float64(p.B)*0.9 + 28)}
				if c.Luma() >= 22 {
					g.Set(x, y, c)
				}
			}
		}
	}
}

// ---------------------------------------------------------------------------
// bubbles: rising bubbles that pop on contact with the silhouette.

type popBubble struct{ x, y, r, vy, phase, hue float64 }

type popSpark struct{ x, y, vx, vy, life float64 }

type bubbles struct {
	body   [rows][cols]bool
	pix    [rows][cols]led.RGB
	list   []popBubble
	sparks []popSpark
	seq    uint64
	pops   int
	spawn  float64
	popped float64 // time of the last pop, for the counter's flash
}

func (b *bubbles) touching(bu popBubble) bool {
	hits := 0
	for i := range 12 {
		a := float64(i) / 12 * 2 * math.Pi
		x, y := int(bu.x+bu.r*math.Cos(a)), int(bu.y+bu.r*math.Sin(a))
		if x >= 0 && x < cols && y >= 0 && y < rows && b.body[y][x] {
			hits++
		}
	}
	cx, cy := int(bu.x), int(bu.y)
	if cx >= 0 && cx < cols && cy >= 0 && cy < rows && b.body[cy][cx] {
		hits += 2
	}
	return hits >= 2
}

func (b *bubbles) Draw(g *led.Grid, t, dt float64) {
	f := CameraSource(camera.Mask)
	if cameraProblem(g, f, t) {
		return
	}
	if f.Seq != b.seq {
		b.seq = f.Seq
		silhouette(f, view{sx: float64(cols) / float64(f.W), sy: float64(rows) / float64(f.H)}, &b.body, &b.pix)
	}
	b.spawn += dt
	for b.spawn > 0.35 && len(b.list) < 40 {
		b.spawn -= 0.35
		r := 2.5 + rand.Float64()*4.5
		b.list = append(b.list, popBubble{x: r + rand.Float64()*(cols-2*r), y: rows + r, r: r, vy: 5 + rand.Float64()*7, phase: rand.Float64() * 6, hue: rand.Float64()})
	}
	keep := b.list[:0]
	for _, bu := range b.list {
		bu.y -= bu.vy * dt
		bu.x += math.Sin(t*1.7+bu.phase) * 3 * dt
		if bu.y < -bu.r {
			continue
		}
		if bu.y < rows-bu.r && b.touching(bu) {
			b.pops++
			b.popped = t
			for i := range 14 {
				a := float64(i)/14*2*math.Pi + rand.Float64()*0.3
				sp := 12 + rand.Float64()*14
				b.sparks = append(b.sparks, popSpark{bu.x + bu.r*math.Cos(a), bu.y + bu.r*math.Sin(a), sp * math.Cos(a), sp * math.Sin(a), 0.5})
			}
			continue
		}
		keep = append(keep, bu)
	}
	b.list = keep
	for y := range rows {
		for x := range cols {
			if b.body[y][x] {
				// you, at about two thirds brightness so the bubbles stay on top
				p := b.pix[y][x]
				c := led.RGB{R: u8(float64(p.R)*0.6 + 10), G: u8(float64(p.G)*0.65 + 24), B: u8(float64(p.B)*0.65 + 28)}
				g.Set(x, y, c)
			}
		}
	}
	for _, bu := range b.list {
		rim := hsv(0.45+bu.hue*0.3, 0.55, 1)
		steps := int(bu.r*2*math.Pi) + 4
		for i := range steps {
			a := float64(i) / float64(steps) * 2 * math.Pi
			g.Set(int(math.Round(bu.x+bu.r*math.Cos(a))), int(math.Round(bu.y+bu.r*math.Sin(a))), rim)
		}
		g.Set(int(bu.x-bu.r*0.4), int(bu.y-bu.r*0.45), led.RGB{R: 255, G: 255, B: 255}) // glint
	}
	ks := b.sparks[:0]
	for _, s := range b.sparks {
		s.life -= dt
		if s.life <= 0 {
			continue
		}
		s.x += s.vx * dt
		s.y += s.vy * dt
		s.vy += 30 * dt
		if c := (led.RGB{R: 255, G: 240, B: 200}).Scale(s.life * 2); c.Luma() >= 22 {
			g.Set(int(s.x), int(s.y), c)
		}
		ks = append(ks, s)
	}
	b.sparks = ks
	c := ink2
	if t-b.popped < 0.3 {
		c = amber
	}
	label(g, fmt.Sprintf("POPS %d", b.pops), "mono", 11, 4, 2, 80, c)
}

// ---------------------------------------------------------------------------
// airpaint: pinch to draw glowing strokes that slowly fade.

type pen struct {
	x, y     float64 // smoothed fingertip, grid cells
	seen     bool
	down     bool
	lastX    float64
	lastY    float64
	hasLast  bool
	lostTime float64
}

type airpaint struct {
	ink   [rows][cols]float64
	hue   [rows][cols]float64
	pens  [2]pen
	seq   uint64
	seen  float64 // last time any hand was seen
	drawn bool
}

func (a *airpaint) stroke(x0, y0, x1, y1, hue float64) {
	n := int(math.Max(math.Abs(x1-x0), math.Abs(y1-y0))) + 1
	for i := 0; i <= n; i++ {
		k := float64(i) / float64(n)
		cx, cy := x0+(x1-x0)*k, y0+(y1-y0)*k
		for dy := -1; dy <= 1; dy++ {
			for dx := -1; dx <= 1; dx++ {
				x, y := int(cx)+dx, int(cy)+dy
				if x < 0 || x >= cols || y < 0 || y >= rows {
					continue
				}
				v := 1.0
				if dx != 0 || dy != 0 {
					v = 0.45
				}
				if v > a.ink[y][x] {
					a.ink[y][x], a.hue[y][x] = v, hue
				}
			}
		}
	}
	a.drawn = true
}

func (a *airpaint) Draw(g *led.Grid, t, dt float64) {
	f := CameraSource(camera.Hands)
	if cameraProblem(g, f, t) {
		return
	}
	fade := math.Exp(-dt / 7)
	for y := range rows {
		for x := range cols {
			a.ink[y][x] *= fade
		}
	}
	if f.Seq != a.seq {
		a.seq = f.Seq
		v := view{sx: float64(cols) / float64(f.W), sy: float64(rows) / float64(f.H)}
		// Match hands to pens by nearest previous position, so strokes do not swap.
		var used [2]bool
		for _, h := range f.Hands {
			if h.Index.Conf < 0.3 {
				continue
			}
			x, y := v.grid(h.Index, f)
			best, bd := -1, math.Inf(1)
			for i := range a.pens {
				if used[i] {
					continue
				}
				d := 0.0
				if a.pens[i].seen {
					d = math.Hypot(a.pens[i].x-x, a.pens[i].y-y)
				} else {
					d = 1e6 + float64(i)
				}
				if d < bd {
					best, bd = i, d
				}
			}
			if best < 0 {
				continue
			}
			used[best] = true
			p := &a.pens[best]
			if p.seen {
				p.x += (x - p.x) * 0.6
				p.y += (y - p.y) * 0.6
			} else {
				p.x, p.y = x, y
			}
			p.seen, p.lostTime = true, t
			// Pinch: thumb tip near index tip, relative to the hand's size (wrist to index tip).
			if h.Thumb.Conf > 0.3 {
				pinch := math.Hypot((h.Index.X-h.Thumb.X)*float64(f.W), (h.Index.Y-h.Thumb.Y)*float64(f.H))
				size := math.Hypot((h.Index.X-h.Wrist.X)*float64(f.W), (h.Index.Y-h.Wrist.Y)*float64(f.H))
				if h.Wrist.Conf < 0.3 || size < 1 {
					size = float64(f.W) * 0.15
				}
				switch r := pinch / size; {
				case r < 0.28:
					p.down = true
				case r > 0.45:
					p.down = false
				}
			}
			a.seen = t
		}
		for i := range a.pens {
			p := &a.pens[i]
			if !used[i] && t-p.lostTime > 0.4 {
				p.seen, p.down, p.hasLast = false, false, false
			}
			if !p.seen {
				continue
			}
			if p.down {
				hue := t*0.05 + float64(i)*0.5
				if p.hasLast {
					a.stroke(p.lastX, p.lastY, p.x, p.y, hue)
				} else {
					a.stroke(p.x, p.y, p.x, p.y, hue)
				}
				p.lastX, p.lastY, p.hasLast = p.x, p.y, true
			} else {
				p.hasLast = false
			}
		}
	}
	for y := range rows {
		for x := range cols {
			if v := a.ink[y][x]; v > 0.06 {
				g.Set(x, y, motionHues[int((a.hue[y][x]-math.Floor(a.hue[y][x]))*256)%256].Scale(math.Min(1, v*1.2)))
			}
		}
	}
	if t-a.seen > 1.5 && !a.drawn {
		message(g, []string{"AIR PAINT", "raise a hand, pinch thumb and finger to draw"}, ink3)
	} else if t-a.seen > 1.5 {
		label(g, "raise a hand, pinch to draw", "mono", 11, 4, 2, 200, ink3)
	}
}
