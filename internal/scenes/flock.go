package scenes

// flock: a murmuration. Birds steer by the three boid rules (keep apart, match heading, stay
// together) and turn back from the edges; every so often a hawk crosses and the flock breaks
// round it.

import (
	"math"

	"github.com/justynroberts/marquee/internal/led"
)

func init() {
	Register("flock", "scene", "A murmuration: a flock of birds wheeling across the panel, scattering when a hawk passes", func() Scene { return newFlock() })
}

type bird struct{ x, y, vx, vy float64 }

type flock struct {
	birds []bird
	hawk  bird
	hawkT float64 // seconds until the next hawk; negative while one is crossing
	trail field
}

const (
	flockN     = 160
	flockSee   = 13.0 // cells a bird looks around
	flockApart = 2.6
	flockMin   = 18.0 // cells a second
	flockMax   = 34.0
)

func newFlock() *flock {
	f := &flock{hawkT: uniform(12, 20)}
	cx, cy := uniform(60, cols-60), uniform(20, rows-20)
	for range flockN {
		a := uniform(0, 2*math.Pi)
		f.birds = append(f.birds, bird{cx + uniform(-25, 25), cy + uniform(-12, 12), 24 * math.Cos(a), 24 * math.Sin(a)})
	}
	return f
}

func (f *flock) Draw(g *led.Grid, t, dt float64) {
	dt = math.Min(dt, 0.1)
	f.moveHawk(dt)
	for i := range f.birds {
		b := &f.birds[i]
		var ax, ay, cx, cy, vx, vy, sx, sy float64
		n := 0.0
		for j := range f.birds {
			if i == j {
				continue
			}
			o := &f.birds[j]
			dx, dy := o.x-b.x, (o.y-b.y)*1.6 // the panel is wide: judge distance a little taller
			d := math.Hypot(dx, dy)
			if d > flockSee {
				continue
			}
			n++
			cx, cy = cx+o.x, cy+o.y
			vx, vy = vx+o.vx, vy+o.vy
			if d < flockApart && d > 0 {
				sx, sy = sx-dx/d*(flockApart-d), sy-dy/d*(flockApart-d)
			}
		}
		if n > 0 {
			ax += (cx/n - b.x) * 1.5 // cohesion
			ay += (cy/n - b.y) * 1.5
			ax += (vx/n - b.vx) * 1.6 // alignment
			ay += (vy/n - b.vy) * 1.6
		}
		ax, ay = ax+sx*14, ay+sy*14 // separation
		// turn back from the edges
		const m = 14.0
		if b.x < m {
			ax += (m - b.x) * 6
		} else if b.x > cols-m {
			ax -= (b.x - (cols - m)) * 6
		}
		if b.y < m/2 {
			ay += (m/2 - b.y) * 8
		} else if b.y > rows-m/2 {
			ay -= (b.y - (rows - m/2)) * 8
		}
		// flee the hawk
		if f.hawkT < 0 {
			dx, dy := b.x-f.hawk.x, b.y-f.hawk.y
			if d := math.Hypot(dx, dy); d < 24 && d > 0 {
				ax, ay = ax+dx/d*(24-d)*22, ay+dy/d*(24-d)*22
			}
		}
		b.vx += ax * dt
		b.vy += ay * dt
		sp := math.Hypot(b.vx, b.vy)
		if lim := math.Max(flockMin, math.Min(flockMax, sp)); sp > 0 && lim != sp {
			b.vx, b.vy = b.vx/sp*lim, b.vy/sp*lim
		}
		b.x += b.vx * dt
		b.y += b.vy * dt
	}

	// draw: fading trails, then the birds at full brightness
	f.trail.scaleAll(math.Pow(0.08, dt))
	dusk := fcol{150, 158, 190}
	for _, b := range f.birds {
		x, y := int(b.x), int(b.y)
		if x >= 0 && x < cols && y >= 0 && y < rows {
			f.trail[y][x] = f.trail[y][x].maxc(dusk)
		}
	}
	if f.hawkT < 0 {
		hx, hy := int(f.hawk.x), int(f.hawk.y)
		for _, d := range [][2]int{{0, 0}, {-1, 0}, {1, 0}, {-2, -1}, {2, -1}} {
			if x, y := hx+d[0], hy+d[1]; x >= 0 && x < cols && y >= 0 && y < rows {
				f.trail[y][x] = fcol{255, 91, 56}
			}
		}
	}
	f.trail.toGrid(g, 18)
	for _, b := range f.birds {
		g.Set(int(b.x), int(b.y), led.RGB{R: 236, G: 234, B: 226})
	}
}

// moveHawk counts down to the next hawk, then flies it straight across the panel.
func (f *flock) moveHawk(dt float64) {
	if f.hawkT > 0 {
		f.hawkT -= dt
		if f.hawkT <= 0 {
			f.hawkT = -1
			left := uniform(0, 1) < 0.5
			f.hawk = bird{x: -4, y: uniform(10, rows-10), vx: 70, vy: uniform(-8, 8)}
			if !left {
				f.hawk.x, f.hawk.vx = cols+4, -70
			}
		}
		return
	}
	f.hawk.x += f.hawk.vx * dt
	f.hawk.y += f.hawk.vy * dt
	if f.hawk.x < -8 || f.hawk.x > cols+8 {
		f.hawkT = uniform(14, 26)
	}
}
