package scenes

import (
	"hash/fnv"
	"math"
	"math/rand/v2"

	"github.com/justynroberts/marquee/internal/led"
)

func init() {
	Register("fire", "scene", "Flames rising from the bottom of the panel", func() Scene { return newFire() })
	Register("life", "scene", "Conway's Game of Life, cells coloured by age, reseeds when it settles", func() Scene { return newLife() })
	Register("warp", "scene", "Flying through a starfield", func() Scene { return newWarp() })
	Register("plasma", "scene", "A flowing colour field", func() Scene { return &plasma{} })
	Register("rain", "scene", "Falling streams of light with bright heads", func() Scene { return newRain() })
}

// ---------------------------------------------------------------------------
// fire: a heat buffer with a hot bottom row; heat rises, averages and cools.

type fire struct {
	heat [rows + 2][cols]float64
	lut  [256]led.RGB
}

func newFire() *fire {
	return &fire{lut: palette256(stop{0, 0, 0, 0}, stop{0.25, 120, 10, 0}, stop{0.5, 255, 70, 0}, stop{0.75, 255, 190, 30}, stop{1, 255, 255, 220})}
}

func (f *fire) Draw(g *led.Grid, t, dt float64) {
	h := &f.heat
	var up [rows + 2][cols]float64
	for step := 0; step < 3; step++ {
		for x := 0; x < cols; x++ {
			gust := 0.5 + 0.5*math.Sin(float64(x)*0.05+t*1.3)
			v := 0.25
			if rand.Float64() > 0.45 {
				v = 1
			}
			h[rows+1][x] = v * (0.75 + 0.25*gust)
			h[rows][x] = h[rows+1][x]
		}
		for y := 0; y < rows; y++ {
			y1 := (y + 1) % (rows + 2)
			y2 := (y + 2) % (rows + 2)
			for x := 0; x < cols; x++ {
				xl := (x - 1 + cols) % cols
				xr := (x + 1) % cols
				up[y][x] = (h[y1][x] + h[y1][xl] + h[y1][xr] + h[y2][x]) / 4
			}
		}
		for y := 0; y < rows; y++ {
			for x := 0; x < cols; x++ {
				cool := 0.006 + 0.007*rand.Float64()
				h[y][x] = clamp01(up[y][x] - cool)
			}
		}
	}
	for y := 0; y < rows; y++ {
		for x := 0; x < cols; x++ {
			c := f.lut[int(h[y][x]*255)]
			if c.Luma() >= 22 {
				g.Set(x, y, c)
			}
		}
	}
}

// ---------------------------------------------------------------------------
// life

type life struct {
	grid    [rows][cols]bool
	age     [rows][cols]float64
	history []uint64
	hue     float64
}

func newLife() *life { l := &life{}; l.seed(); return l }

func (l *life) seed() {
	for y := range rows {
		for x := range cols {
			l.grid[y][x] = rand.Float64() < 0.28
			l.age[y][x] = 0
		}
	}
	l.history = nil
	l.hue = rand.Float64()
}

func (l *life) Draw(g *led.Grid, t, dt float64) {
	var next [rows][cols]bool
	alive := 0
	hsh := fnv.New64a()
	row := make([]byte, cols)
	for y := range rows {
		for x := range cols {
			n := 0
			for dy := -1; dy <= 1; dy++ {
				for dx := -1; dx <= 1; dx++ {
					if dx == 0 && dy == 0 {
						continue
					}
					if l.grid[(y+dy+rows)%rows][(x+dx+cols)%cols] {
						n++
					}
				}
			}
			was := l.grid[y][x]
			now := n == 3 || (was && n == 2)
			next[y][x] = now
			switch {
			case now && was:
				l.age[y][x]++
			default:
				l.age[y][x] = 0
			}
			if now {
				alive++
				row[x] = 1
			} else {
				row[x] = 0
			}
		}
		hsh.Write(row)
	}
	l.grid = next
	sig := hsh.Sum64()
	l.history = append(l.history, sig)
	if len(l.history) > 12 {
		l.history = l.history[len(l.history)-12:]
	}
	count := 0
	for _, s := range l.history {
		if s == sig {
			count++
		}
	}
	// As in scenes.py: reseed (new grid, zero ages, new hue) before colouring, then colour the
	// generation just computed using the possibly reset ages and hue.
	if alive < 40 || count > 2 {
		l.seed()
	}
	for y := range rows {
		for x := range cols {
			if next[y][x] {
				a := l.age[y][x]
				g.Set(x, y, hsv(l.hue+math.Min(a, 30)*0.018, 0.75, 1-math.Min(a, 40)*0.012))
			}
		}
	}
}

// ---------------------------------------------------------------------------
// warp: points projected from 3D, brighter and longer as they near.

type warp struct {
	x, y, z []float64
	out     field
}

func newWarp() *warp {
	n := 420
	w := &warp{x: make([]float64, n), y: make([]float64, n), z: make([]float64, n)}
	for i := range n {
		w.x[i], w.y[i], w.z[i] = uniform(-1, 1), uniform(-1, 1), uniform(0.05, 1)
	}
	return w
}

func (w *warp) Draw(g *led.Grid, t, dt float64) {
	speed := 0.55 + 0.35*math.Sin(t*0.4)
	w.out.clear()
	cx, cy := float64(cols)/2, float64(rows)/2
	blue := fcol{120, 190, 255}
	white := fcol{240, 236, 225}
	for trail := 0; trail < 7; trail++ {
		for i := range w.z {
			z := w.z[i] + float64(trail)*0.02*speed
			sx := int(cx + w.x[i]/z*cx*0.5)
			sy := int(cy + w.y[i]/z*cy*1.6)
			if sx < 0 || sx >= cols || sy < 0 || sy >= rows {
				continue
			}
			v := clamp01((1.35 - z) * 1.6 * (1 - float64(trail)*0.13))
			tint := white
			if i%7 == 0 {
				tint = blue
			}
			w.out[sy][sx] = w.out[sy][sx].maxc(tint.scale(v))
		}
	}
	for i := range w.z {
		w.z[i] -= dt * speed
		if w.z[i] <= 0.02 {
			w.z[i], w.x[i], w.y[i] = 1, uniform(-1, 1), uniform(-1, 1)
		}
	}
	w.out.toGrid(g, 22)
}

// ---------------------------------------------------------------------------
// plasma

type plasma struct{}

func (p *plasma) Draw(g *led.Grid, t, dt float64) {
	cx := 3 - math.Sin(t*0.3)*2
	for y := range rows {
		fy := float64(y) / rows * 1.8
		for x := range cols {
			fx := float64(x) / cols * 6.0
			dx, dy := fx-cx, fy-0.9
			v := math.Sin(fx+t) + math.Sin((fy+t*0.7)*2) + math.Sin((fx+fy+t*0.5)*1.3) +
				math.Sin(math.Sqrt(dx*dx+dy*dy)*3-t*1.5)
			c := hsv(v*0.12+t*0.05, 0.85, 0.35+0.65*(0.5+0.5*math.Sin(v*2)))
			if c.Luma() >= 22 {
				g.Set(x, y, c)
			}
		}
	}
}

// ---------------------------------------------------------------------------
// rain

type rain struct {
	head, speed, length []float64
	accent              []bool
}

func newRain() *rain {
	r := &rain{head: make([]float64, cols), speed: make([]float64, cols), length: make([]float64, cols), accent: make([]bool, cols)}
	for x := range cols {
		r.head[x] = uniform(-rows, rows)
		r.speed[x] = uniform(14, 40)
		r.length[x] = uniform(8, 30)
		r.accent[x] = rand.Float64() < 0.12
	}
	return r
}

func (r *rain) Draw(g *led.Grid, t, dt float64) {
	inkc := fcol{150, 220, 200}
	acc := fcol{255, 91, 56}
	white := fcol{255, 255, 245}
	for x := range cols {
		r.head[x] += r.speed[x] * dt
		col := inkc
		if r.accent[x] {
			col = acc
		}
		for y := range rows {
			d := r.head[x] - float64(y)
			tail := 0.0
			if d >= 0 {
				tail = clamp01(1 - d/r.length[x])
			}
			glow := clamp01(1 - math.Abs(d)/1.2)
			c := col.scale(math.Pow(tail, 1.6)).add(white.scale(glow * 0.9)).rgb()
			if c.Luma() >= 22 {
				g.Set(x, y, c)
			}
		}
		if r.head[x]-r.length[x] > rows {
			r.head[x] = uniform(-30, 0)
			r.speed[x] = uniform(14, 40)
		}
	}
}
