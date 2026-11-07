package scenes

import (
	"math"
	"math/rand/v2"

	"github.com/justynroberts/marquee/internal/emoji"
	"github.com/justynroberts/marquee/internal/led"
)

func init() {
	Register("bounce", "scene", "An emoji bouncing around the panel, changing on every wall hit", func() Scene { return newBounce() })
	Register("fireworks", "scene", "Rockets that burst into coloured sparks", func() Scene { return &fireworks{} })
	Register("aquarium", "scene", "Emoji fish, rising bubbles and swaying weed", func() Scene { return newAquarium() })
}

// sprite is an emoji (or fallback block) at LED resolution.
type sprite struct {
	w, h int
	lit  []bool
	col  []fcol
}

func emojiSprite(cluster string, h int, flip bool) sprite {
	s, ok := emoji.Render(cluster, h)
	if !ok {
		sp := sprite{w: h, h: h, lit: make([]bool, h*h), col: make([]fcol, h*h)}
		for i := range sp.lit {
			sp.lit[i], sp.col[i] = true, fcol{220, 220, 220}
		}
		return sp
	}
	sp := sprite{w: s.W, h: s.H, lit: make([]bool, s.W*s.H), col: make([]fcol, s.W*s.H)}
	for y := 0; y < s.H; y++ {
		for x := 0; x < s.W; x++ {
			sx := x
			if flip {
				sx = s.W - 1 - x
			}
			i := y*s.W + sx
			o := y*s.W + x
			sp.lit[o] = s.Lit[i]
			c := s.Col[i]
			sp.col[o] = fcol{float64(c.R), float64(c.G), float64(c.B)}
		}
	}
	return sp
}

// ---------------------------------------------------------------------------

type bounce struct {
	x, y, vx, vy float64
	i            int
	trail        field
	cache        map[int]sprite
}

var bounceCast = []string{"🚀", "⭐️", "🍕", "👾", "🌈", "🔥", "🎉", "☕️", "🐙", "💎"}

func newBounce() *bounce {
	return &bounce{x: 40, y: 10, vx: 38, vy: 17, cache: map[int]sprite{}}
}

func (b *bounce) sprite() sprite {
	k := b.i % len(bounceCast)
	if s, ok := b.cache[k]; ok {
		return s
	}
	s := emojiSprite(bounceCast[k], 30, false)
	b.cache[k] = s
	return s
}

func (b *bounce) Draw(g *led.Grid, t, dt float64) {
	s := b.sprite()
	b.x += b.vx * dt
	b.y += b.vy * dt
	hit := false
	if b.x < 0 || b.x+float64(s.w) > cols {
		b.vx = -b.vx
		b.x = math.Min(math.Max(b.x, 0), float64(cols-s.w))
		hit = true
	}
	if b.y < 0 || b.y+float64(s.h) > rows {
		b.vy = -b.vy
		b.y = math.Min(math.Max(b.y, 0), float64(rows-s.h))
		hit = true
	}
	if hit {
		b.i++
	}
	b.trail.scaleAll(0.82)
	xi, yi := int(b.x), int(b.y)
	for y := 0; y < s.h; y++ {
		for x := 0; x < s.w; x++ {
			if s.lit[y*s.w+x] && yi+y < rows && xi+x < cols {
				b.trail[yi+y][xi+x] = s.col[y*s.w+x].scale(0.55)
			}
		}
	}
	b.trail.toGrid(g, 22)
	for y := 0; y < s.h; y++ {
		for x := 0; x < s.w; x++ {
			if c := s.col[y*s.w+x].rgb(); s.lit[y*s.w+x] && c.Luma() >= 22 { // to_image floor
				g.Set(xi+x, yi+y, c)
			}
		}
	}
}

// ---------------------------------------------------------------------------

type rocket struct {
	x, y, vx, vy, fuse float64
	ci                 int
}

type spark struct {
	x, y, vx, vy, life float64
	c                  fcol
}

type fireworks struct {
	rockets    []rocket
	sparks     []spark
	canvas     field
	nextLaunch float64
}

var fwColours = []fcol{{255, 91, 56}, {255, 214, 60}, {70, 220, 110}, {60, 210, 230}, {170, 110, 255}, {255, 105, 200}, {236, 232, 220}}

func (f *fireworks) Draw(g *led.Grid, t, dt float64) {
	if t >= f.nextLaunch {
		f.rockets = append(f.rockets, rocket{uniform(30, cols-30), rows - 1, uniform(-6, 6), uniform(-62, -48), uniform(0.55, 0.9), rand.IntN(len(fwColours))})
		f.nextLaunch = t + uniform(0.25, 0.9)
	}
	f.canvas.scaleAll(0.72)
	alive := f.rockets[:0]
	for _, r := range f.rockets {
		r.x += r.vx * dt
		r.y += r.vy * dt
		r.vy += 40 * dt
		r.fuse -= dt
		if r.fuse <= 0 || r.vy > -4 {
			n := 40 + rand.IntN(51)
			c := fwColours[r.ci]
			two := rand.Float64() < 0.3
			c2 := fwColours[rand.IntN(len(fwColours))]
			for i := 0; i < n; i++ {
				ang := uniform(0, 2*math.Pi)
				spd := uniform(8, 34)
				col := c
				if two && i%2 == 1 {
					col = c2
				}
				f.sparks = append(f.sparks, spark{r.x, r.y, math.Cos(ang) * spd * 1.6, math.Sin(ang) * spd, uniform(0.8, 1.6), col})
			}
			continue
		}
		alive = append(alive, r)
		xi, yi := int(r.x), int(r.y)
		if xi >= 0 && xi < cols && yi >= 0 && yi < rows {
			f.canvas[yi][xi] = fcol{255, 230, 180}
		}
	}
	f.rockets = alive
	live := f.sparks[:0]
	for _, s := range f.sparks {
		s.x += s.vx * dt
		s.y += s.vy * dt
		s.vy += 22 * dt
		s.vx *= 0.985
		s.life -= dt
		if s.life <= 0 || s.y >= rows || s.x < 0 || s.x >= cols {
			continue
		}
		live = append(live, s)
		yi := min(max(int(s.y), 0), rows-1)
		k := clamp01(s.life)
		if s.life < 0.4 {
			k *= uniform(0.3, 1)
		}
		f.canvas[yi][int(s.x)] = f.canvas[yi][int(s.x)].maxc(s.c.scale(k))
	}
	f.sparks = live
	f.canvas.toGrid(g, 22)
}

// ---------------------------------------------------------------------------

type fish struct {
	x, y, v, phase float64
	s              sprite
}

type bubble struct{ x, y, v float64 }

type aquarium struct {
	fish    []*fish
	bubbles []bubble
	out     field
}

var fishCast = []string{"🐠", "🐟", "🐡", "🐙", "🦈", "🐬", "🦑", "🐳"}

func newAquarium() *aquarium {
	a := &aquarium{}
	for i := 0; i < 7; i++ {
		x := uniform(0, cols)
		a.fish = append(a.fish, a.spawn(&x))
	}
	for i := 0; i < 30; i++ {
		a.bubbles = append(a.bubbles, bubble{uniform(0, cols), uniform(0, rows), uniform(5, 14)})
	}
	return a
}

func (a *aquarium) spawn(x *float64) *fish {
	sizes := []int{14, 18, 22, 28}
	size := sizes[rand.IntN(len(sizes))]
	v := uniform(8, 26)
	if rand.IntN(2) == 0 {
		v = -v
	}
	// emoji fish face left; flip the ones swimming right
	s := emojiSprite(fishCast[rand.IntN(len(fishCast))], size, v > 0)
	f := &fish{y: uniform(2, float64(rows-size-6)), v: v, phase: uniform(0, 6.28), s: s}
	switch {
	case x != nil:
		f.x = *x
	case v > 0:
		f.x = -float64(s.w)
	default:
		f.x = cols
	}
	return f
}

func (a *aquarium) Draw(g *led.Grid, t, dt float64) {
	for y := range rows {
		depth := float64(y) / rows
		base := fcol{4, 18, 40}.scale(0.6 + 0.6*depth)
		for x := range cols {
			rays := 0.5 + 0.5*math.Sin(float64(x)*0.05+t*0.4+float64(y)*0.08)
			a.out[y][x] = base.add(fcol{10, 30, 40}.scale(rays * (1 - depth) * (1 - depth)))
		}
	}
	for wx := 12; wx < cols; wx += 37 {
		for y := rows - 22; y < rows; y++ {
			sway := int(math.Round(2 * math.Sin(t*1.5+float64(wx)+float64(y)*0.2) * float64(rows-y) / 22))
			x := wx + sway
			if x >= 0 && x < cols {
				a.out[y][x] = fcol{40, 160, 70}
			}
		}
	}
	for i := range a.bubbles {
		b := &a.bubbles[i]
		b.y -= b.v * dt
		b.x += math.Sin(t*3+b.v) * 0.3
		if b.y < 0 {
			b.y, b.x = rows-1, uniform(0, cols)
		}
		x := ((int(b.x) % cols) + cols) % cols
		a.out[int(b.y)][x] = fcol{170, 220, 240}
	}
	for i, f := range a.fish {
		f.x += f.v * dt
		if (f.v > 0 && f.x > cols) || (f.v < 0 && f.x < -float64(f.s.w)) {
			a.fish[i] = a.spawn(nil)
			continue
		}
		x0 := int(f.x)
		y0 := int(f.y + 2*math.Sin(t*1.3+f.phase))
		for y := 0; y < f.s.h; y++ {
			for x := 0; x < f.s.w; x++ {
				px, py := x0+x, y0+y
				if px < 0 || px >= cols || py < 0 || py >= rows || !f.s.lit[y*f.s.w+x] {
					continue
				}
				a.out[py][px] = f.s.col[y*f.s.w+x]
			}
		}
	}
	a.out.toGrid(g, 22)
}
