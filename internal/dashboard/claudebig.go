package dashboard

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"math/rand/v2"
	"strings"
	"time"

	"github.com/justynroberts/marquee/internal/feeds"
)

// Four large Claude Code screens: a 24-hour clock made of the Claude spark, sessions orbiting
// the spark like planets, a firework for every reply, and three big gauges.

// projectColour gives each project a steady colour.
func projectColour(name string) color.RGBA {
	cs := []color.RGBA{claudeOrange, cyan, violet, green, amber, {255, 120, 170, 255}, {120, 230, 220, 255}}
	h := 0
	for _, r := range name {
		h = h*31 + int(r)
	}
	return cs[(h%len(cs)+len(cs))%len(cs)]
}

// ------------------------------------------------------------------------------------ clock

// ClaudeClock is the Claude spark drawn as a 24-hour clock: one ray an hour, as long as that
// hour's output, midnight at the top. The ray for now grows as tokens arrive and a hand sweeps
// round with the time.
type ClaudeClock struct {
	live  *ccLive
	cache frameCache
	hours [24]int64
	top   int64
}

func NewClaudeClock() *ClaudeClock { return &ClaudeClock{live: newCCLive()} }

const (
	ckX, ckY = 960.0, 231.0
	ckIn     = 34.0
	ckOut    = 206.0
)

func ckAngle(h float64) float64 { return -math.Pi/2 + h/24*2*math.Pi }

func (c *ClaudeClock) Frame(t, dt float64, now time.Time) *image.RGBA {
	u, got, ok, err := c.live.get()
	if !ok {
		return waitMessage(&c.cache, "CLAUDE CODE", err)
	}
	img := c.cache.get(fmt.Sprint(got.UnixNano(), now.Format("15:04")), func(img *image.RGBA) { c.base(img, u, now) })
	// the hour now: its ray breathes, brighter the faster tokens come
	h := now.Hour()
	k := 0.5 + 0.5*math.Sin(t*3)
	heat := math.Min(1, c.live.rate/3000)
	l := ckIn + 10 + float64(c.hours[h])/float64(c.top)*(ckOut-ckIn-10)
	l += 6 * k * (0.3 + heat)
	ckRay(img, ckAngle(float64(h)+0.5), l, 1.25, mix(claudeOrange, color.RGBA{255, 236, 210, 255}, 0.3+0.5*k*heat))
	// the hand
	hf := minutesOfDay(now) / 60
	a := ckAngle(hf)
	wideLine(img, int(ckX+math.Cos(a)*ckIn*0.6), int(ckY+math.Sin(a)*ckIn*0.6), int(ckX+math.Cos(a)*(ckOut+14)), int(ckY+math.Sin(a)*(ckOut+14)), mix(bg, cyan, 0.8), 2)
	disc(img, ckX+math.Cos(a)*(ckOut+14), ckY+math.Sin(a)*(ckOut+14), 5, cyan)
	disc(img, ckX, ckY, ckIn*(0.8+0.12*k), claudeOrange)
	return img
}

// ckRay is one tapered ray of the spark from the centre out to length l.
func ckRay(img *image.RGBA, a, l, wide float64, c color.RGBA) {
	dx, dy := math.Cos(a), math.Sin(a)
	seg := func(f0, f1, w float64) {
		wideLine(img, int(ckX+dx*l*f0), int(ckY+dy*l*f0), int(ckX+dx*l*f1), int(ckY+dy*l*f1), c, max(2, int(w*wide)))
	}
	seg(0.15, 0.5, 26)
	seg(0.45, 0.8, 18)
	seg(0.75, 1, 10)
	disc(img, ckX+dx*l, ckY+dy*l, 5*wide, c)
}

func (c *ClaudeClock) base(img *image.RGBA, u *feeds.ClaudeUsage, now time.Time) {
	c.top = 1
	for i, v := range u.Hours {
		c.hours[i] = v.Out
		c.top = max(c.top, v.Out)
	}
	// the dial
	for h := range 24 {
		a := ckAngle(float64(h))
		r0, r1 := ckOut+6, ckOut+14
		if h%6 == 0 {
			r1 = ckOut + 22
		}
		wideLine(img, int(ckX+math.Cos(a)*r0), int(ckY+math.Sin(a)*r0), int(ckX+math.Cos(a)*r1), int(ckY+math.Sin(a)*r1), txt3, 2)
	}
	for _, h := range []int{6, 12, 18} {
		a := ckAngle(float64(h))
		l := fmt.Sprintf("%02d", h)
		x := ckX + math.Cos(a)*(ckOut+40) - measure(l, 18)/2
		text(img, x, int(ckY+math.Sin(a)*(ckOut+36))-10, l, 18, txt3)
	}
	// the past hours
	for h, v := range c.hours {
		if h == now.Hour() {
			continue
		}
		a := ckAngle(float64(h) + 0.5)
		if v == 0 || h > now.Hour() {
			ckRay(img, a, ckIn+12, 0.4, line)
			continue
		}
		f := float64(v) / float64(c.top)
		ckRay(img, a, ckIn+10+f*(ckOut-ckIn-10), 0.55+0.6*f, mix(bg, claudeOrange, 0.45+0.55*f))
	}

	// left: today, large
	text(img, pad, pad-4, "CLAUDE CODE  TODAY", 22, claudeOrange)
	s := tok(u.Today.Out)
	text(img, pad, 76, s, 120, amber)
	text(img, pad, 214, "OUTPUT TOKENS", 22, txt2)
	r, rc := c.live.rateLine()
	text(img, pad, 260, r, 24, rc)
	text(img, pad, 304, fmt.Sprintf("%d REPLIES", u.Today.Calls), 24, txt)
	text(img, pad, 348, fmt.Sprintf("%d SESSIONS RUNNING", len(u.Sessions)), 24, txt)

	// right: the peak, the busiest project
	x := float64(ckX + ckOut + 120)
	ph, pv := 0, int64(0)
	for h, v := range c.hours {
		if v > pv {
			ph, pv = h, v
		}
	}
	text(img, x, pad-4, "PEAK HOUR", 22, cyan)
	if pv > 0 {
		text(img, x, 70, fmt.Sprintf("%02d:00", ph), 72, txt)
		text(img, x, 156, tok(pv)+" TOKENS", 24, amber)
	}
	if len(u.Projects) > 0 {
		text(img, x, 230, "MOSTLY", 22, cyan)
		p := u.Projects[0]
		text(img, x, 266, fit(strings.ToUpper(p.Name), 48, float64(W-pad)-x), 48, projectColour(p.Name))
		text(img, x, 330, tok(p.Out)+" OUTPUT", 24, txt2)
	}
	text(img, x, 400, "EACH RAY IS AN HOUR", 16, txt3)
}

// ------------------------------------------------------------------------------------ orbit

// ClaudeOrbit is the spark as a sun with every running session a planet round it: the bigger
// the planet the more it has written today, its ring how full its conversation is. Working
// planets race round and stream sparks into the sun; idle ones drift.
type ClaudeOrbit struct {
	live    *ccLive
	cache   frameCache
	planets []orPlanet
	rot     float64
	spin    float64
	spark   []ccSpark
	rng     *rand.Rand
}

type orPlanet struct {
	s         feeds.ClaudeSession
	rx, ry    float64
	ang, size float64
	acc       float64
	x, y      float64
}

func NewClaudeOrbit() *ClaudeOrbit {
	return &ClaudeOrbit{live: newCCLive(), rng: rand.New(rand.NewPCG(4, 4))}
}

const orX, orY = 960.0, 236.0

func (o *ClaudeOrbit) Frame(t, dt float64, now time.Time) *image.RGBA {
	u, got, ok, err := o.live.get()
	if !ok {
		return waitMessage(&o.cache, "CLAUDE CODE", err)
	}
	img := o.cache.get(fmt.Sprint(got.UnixNano(), now.Format("15:04")), func(img *image.RGBA) { o.base(img, u) })
	// all planets turn together, evenly spaced, so they never pile up
	o.spin += dt * (0.08 + math.Min(0.25, o.live.rate/20000))
	for i := range o.planets {
		p := &o.planets[i]
		a := o.spin + p.ang
		p.x, p.y = orX+math.Cos(a)*p.rx, orY+math.Sin(a)*p.ry
	}
	// behind the sun first, then the sun, then in front
	draw := func(front bool) {
		for i := range o.planets {
			if p := &o.planets[i]; (p.y >= orY) == front {
				o.drawPlanet(img, p, t)
			}
		}
	}
	draw(false)
	o.rot += dt * (0.1 + math.Min(2, o.live.rate/2500))
	k := 0.5 + 0.5*math.Sin(t*1.7)
	disc(img, orX, orY, 92+6*k, mix(bg, claudeOrange, 0.08))
	disc(img, orX, orY, 74, mix(bg, claudeOrange, 0.16))
	ccLogo(img, orX, orY, 66, o.rot, claudeOrange)
	draw(true)

	for i := range o.planets {
		p := &o.planets[i]
		if !p.s.Busy {
			continue
		}
		p.acc += dt * math.Max(4, math.Min(30, o.live.sessRate[p.s.Name]/250))
		for ; p.acc >= 1 && len(o.spark) < 400; p.acc-- {
			cx := (p.x+orX)/2 + (o.rng.Float64()-0.5)*160
			cy := (p.y+orY)/2 + (o.rng.Float64()-0.5)*120
			o.spark = append(o.spark, ccSpark{x0: p.x, y0: p.y, cx: cx, cy: cy, x1: orX, y1: orY, t0: t, life: 0.9 + o.rng.Float64(), c: projectColour(p.s.Project)})
		}
	}
	keep := o.spark[:0]
	for _, s := range o.spark {
		v := (t - s.t0) / s.life
		if v >= 1 || v < 0 {
			continue
		}
		keep = append(keep, s)
		a, b, c := (1-v)*(1-v), 2*v*(1-v), v*v
		disc(img, a*s.x0+b*s.cx+c*s.x1, a*s.y0+b*s.cy+c*s.y1, 2.6, mix(s.c, claudeOrange, v))
	}
	o.spark = keep
	return img
}

func (o *ClaudeOrbit) drawPlanet(img *image.RGBA, p *orPlanet, t float64) {
	c := projectColour(p.s.Project)
	if !p.s.Busy {
		c = mix(bg, c, 0.55)
	} else {
		disc(img, p.x, p.y, p.size+10+4*math.Sin(t*3), mix(bg, c, 0.18))
	}
	disc(img, p.x, p.y, p.size, c)
	disc(img, p.x-p.size*0.3, p.y-p.size*0.3, p.size*0.35, mix(c, txt, 0.4))
	f := ctxFrac(p.s.Context)
	ring(img, int(p.x), int(p.y), p.size+8, line)
	ccArc(img, p.x, p.y, p.size+8, f, ramp(f*1.2-0.2), 4)
	name := fit(strings.ToUpper(p.s.Name), 20, 260)
	text(img, p.x-measure(name, 20)/2, int(p.y+p.size+14), name, 20, txt)
	info := tok(p.s.Today.Out)
	if p.s.Busy {
		info += "  WORKING"
	}
	text(img, p.x-measure(info, 16)/2, int(p.y+p.size+38), info, 16, txt2)
}

func (o *ClaudeOrbit) base(img *image.RGBA, u *feeds.ClaudeUsage) {
	// orbits and a sprinkle of stars
	r := rand.New(rand.NewPCG(1, 2))
	for range 120 {
		x, y := r.IntN(W), r.IntN(H)
		rect(img, x, y, x, y, mix(bg, txt, 0.2+0.4*r.Float64()))
	}
	o.planets = o.planets[:0]
	ss := u.Sessions
	if len(ss) > 6 {
		ss = ss[:6]
	}
	for i, s := range ss {
		rx := 330 + float64(i)*(560/math.Max(1, float64(len(ss)-1)))
		if len(ss) == 1 {
			rx = 520
		}
		ry := math.Min(150, rx*0.24)
		pts := make([][2]float64, 0, 129)
		for k := 0; k <= 128; k++ {
			a := float64(k) / 128 * 2 * math.Pi
			pts = append(pts, [2]float64{orX + math.Cos(a)*rx, orY + math.Sin(a)*ry})
		}
		polyline(img, pts, mix(bg, line, 0.9), 1)
		ang := float64(i) * 2 * math.Pi / float64(len(ss))
		size := 14 + math.Min(34, math.Sqrt(float64(s.Today.Out))/18)
		o.planets = append(o.planets, orPlanet{s: s, rx: rx, ry: ry, ang: ang, size: size,
			x: orX + math.Cos(o.spin+ang)*rx, y: orY + math.Sin(o.spin+ang)*ry})
	}
	text(img, pad, pad-4, "CLAUDE CODE", 26, claudeOrange)
	text(img, pad, pad+34, tok(u.Today.Out)+" TODAY", 22, amber)
	r2, rc := o.live.rateLine()
	text(img, pad, pad+66, r2, 18, rc)
	if len(ss) == 0 {
		msg := "NO SESSIONS IN ORBIT"
		text(img, orX-measure(msg, 22)/2, int(orY)+110, msg, 22, txt3)
	}
	text(img, float64(W-pad)-measure("PLANET SIZE  OUTPUT TODAY", 16), H-pad-14, "PLANET SIZE  OUTPUT TODAY", 16, txt3)
	text(img, float64(W-pad)-measure("RING  CONTEXT USED", 16), H-pad-38, "RING  CONTEXT USED", 16, txt3)
}

// ------------------------------------------------------------------------------------ burst

// ClaudeBurst sets off a firework for every reply Claude writes: the more tokens, the bigger
// the burst, in its project's colour, with the project and count written where it bursts.
// When nothing new arrives it replays the day's replies, dimmer.
type ClaudeBurst struct {
	live    *ccLive
	cache   frameCache
	seen    time.Time
	queue   []bsQueued
	rockets []bsRocket
	sparks  []bsSpark
	labels  []bsLabel
	replay  float64
	rng     *rand.Rand
	count   int
}

type bsQueued struct {
	at   float64
	call feeds.ClaudeCall
	old  bool
}

type bsRocket struct {
	x, y, vy, top float64
	call          feeds.ClaudeCall
	c             color.RGBA
	old           bool
}

type bsSpark struct {
	x, y, vx, vy, life, age float64
	c                       color.RGBA
}

type bsLabel struct {
	x, y, age float64
	s         string
	c         color.RGBA
}

func NewClaudeBurst() *ClaudeBurst {
	return &ClaudeBurst{live: newCCLive(), rng: rand.New(rand.NewPCG(5, 8))}
}

func (b *ClaudeBurst) Frame(t, dt float64, now time.Time) *image.RGBA {
	u, got, ok, err := b.live.get()
	if !ok {
		return waitMessage(&b.cache, "CLAUDE CODE", err)
	}
	img := b.cache.get(fmt.Sprint(got.UnixNano(), now.Format("15:04")), func(img *image.RGBA) { b.base(img, u) })

	// new replies since last time, spread over the next few seconds
	if b.seen.IsZero() {
		if n := len(u.Recent); n > 0 {
			b.seen = u.Recent[n-1].At
		} else {
			b.seen = now
		}
	}
	var fresh []feeds.ClaudeCall
	for _, c := range u.Recent {
		if c.At.After(b.seen) {
			fresh = append(fresh, c)
		}
	}
	if len(fresh) > 0 {
		b.seen = fresh[len(fresh)-1].At
		gap := math.Min(0.6, 7/float64(len(fresh)))
		for i, c := range fresh {
			b.queue = append(b.queue, bsQueued{at: t + float64(i)*gap, call: c})
		}
		b.replay = t + 3
	}
	// quiet: replay the day's replies now and then
	if len(b.queue) == 0 && t > b.replay && len(u.Recent) > 0 {
		c := u.Recent[b.rng.IntN(len(u.Recent))]
		b.queue = append(b.queue, bsQueued{at: t, call: c, old: true})
		b.replay = t + 0.4 + b.rng.Float64()*0.7
	}
	keepQ := b.queue[:0]
	for _, q := range b.queue {
		if q.at > t {
			keepQ = append(keepQ, q)
			continue
		}
		c := projectColour(q.call.Project)
		if q.old {
			c = mix(bg, c, 0.85)
		}
		size := math.Min(1, 0.25+math.Sqrt(float64(q.call.Out))/45)
		b.rockets = append(b.rockets, bsRocket{x: 300 + b.rng.Float64()*(W-400), y: H, vy: -420 - 160*b.rng.Float64(),
			top: 130 + (1-size)*110 + b.rng.Float64()*70, call: q.call, c: c, old: q.old})
	}
	b.queue = keepQ

	keepR := b.rockets[:0]
	for _, r := range b.rockets {
		r.y += r.vy * dt
		if r.y > r.top {
			disc(img, r.x, r.y, 3, mix(r.c, txt, 0.5))
			for k := 1; k < 6; k++ {
				disc(img, r.x+(b.rng.Float64()-0.5)*3, r.y+float64(k)*7, 1.6, mix(bg, amber, 0.8-float64(k)*0.13))
			}
			keepR = append(keepR, r)
			continue
		}
		size := math.Min(1, 0.25+math.Sqrt(float64(r.call.Out))/45)
		n := 30 + int(size*170)
		speed := 120 + size*300
		for i := 0; i < n && len(b.sparks) < 2500; i++ {
			a := float64(i)/float64(n)*2*math.Pi + b.rng.Float64()*0.2
			v := speed * (0.6 + 0.4*b.rng.Float64())
			c := r.c
			if i%5 == 0 {
				c = mix(r.c, txt, 0.6)
			}
			b.sparks = append(b.sparks, bsSpark{x: r.x, y: r.y, vx: math.Cos(a) * v, vy: math.Sin(a) * v, life: 1.4 + size*1.2, c: c})
		}
		if !r.old {
			b.count++
		}
		lbl := strings.ToUpper(r.call.Project) + "  " + tok(r.call.Out)
		b.labels = append(b.labels, bsLabel{x: r.x, y: r.y, s: lbl, c: r.c})
	}
	b.rockets = keepR

	keepS := b.sparks[:0]
	for _, s := range b.sparks {
		s.age += dt
		if s.age > s.life {
			continue
		}
		s.vy += 120 * dt
		s.vx *= 1 - 1.2*dt
		s.vy *= 1 - 1.2*dt
		s.x += s.vx * dt
		s.y += s.vy * dt
		f := 1 - s.age/s.life
		if s.x >= 0 && s.x < W && s.y >= 0 && s.y < H {
			disc(img, s.x, s.y, 1.8+2.2*f, mix(bg, s.c, math.Sqrt(f)))
		}
		keepS = append(keepS, s)
	}
	b.sparks = keepS

	keepL := b.labels[:0]
	for _, l := range b.labels {
		l.age += dt
		if l.age > 2.4 {
			continue
		}
		f := math.Sqrt(1 - l.age/2.4)
		text(img, l.x-measure(l.s, 20)/2, int(l.y-30-l.age*12), l.s, 20, mix(bg, l.c, f))
		keepL = append(keepL, l)
	}
	b.labels = keepL

	// the scoreboard, drawn last so bursts pass behind it
	ccLogo(img, pad+30, pad+22, 26, t*0.3, claudeOrange)
	text(img, pad+70, pad+4, fmt.Sprintf("%d REPLIES TODAY", u.Today.Calls), 24, claudeOrange)
	if b.count > 0 {
		text(img, pad+70, pad+38, fmt.Sprintf("%d BURST SINCE YOU LOOKED", b.count), 16, txt2)
	} else {
		text(img, pad+70, pad+38, "EACH BURST IS ONE REPLY  ·  DIM ONES ARE REPLAYS", 16, txt3)
	}
	return img
}

func (b *ClaudeBurst) base(img *image.RGBA, u *feeds.ClaudeUsage) {
	// a dark skyline of today's hours along the bottom
	step := float64(W) / 24
	var top int64 = 1
	for _, h := range u.Hours {
		top = max(top, h.Out)
	}
	for i, h := range u.Hours {
		bh := 6 + int(float64(h.Out)/float64(top)*50)
		rect(img, int(float64(i)*step), H-bh, int(float64(i+1)*step)-3, H, mix(bg, violet, 0.22))
	}
}

// ------------------------------------------------------------------------------------ meter

// ClaudeMeter is three big gauges: output tokens a minute, how full the busiest conversation
// is, and the cache hit rate. Needles swing to new readings and tremble while Claude works.
type ClaudeMeter struct {
	live   *ccLive
	cache  frameCache
	needle [3]float64
	target [3]float64
	busy   bool
	labels [3]string
}

func NewClaudeMeter() *ClaudeMeter { return &ClaudeMeter{live: newCCLive()} }

const (
	mtY   = 268.0
	mtR   = 176.0
	mtA0  = math.Pi * 0.8 // from lower left
	mtA1  = math.Pi * 2.2 // to lower right
	mtMax = 20000.0       // tokens a minute at full scale
)

var mtX = [3]float64{350, 960, 1570}

func (m *ClaudeMeter) Frame(t, dt float64, now time.Time) *image.RGBA {
	u, got, ok, err := m.live.get()
	if !ok {
		return waitMessage(&m.cache, "CLAUDE CODE", err)
	}
	img := m.cache.get(fmt.Sprint(got.UnixNano(), now.Format("15:04")), func(img *image.RGBA) { m.base(img, u) })
	for i := range m.needle {
		m.needle[i] += (m.target[i] - m.needle[i]) * math.Min(1, dt*2.5)
		v := m.needle[i]
		if m.busy && i < 2 {
			v += 0.008 * math.Sin(t*23+float64(i)*2)
		}
		v = math.Max(0, math.Min(1, v))
		a := mtA0 + v*(mtA1-mtA0)
		x, y := mtX[i], mtY
		c := ramp(v*1.3 - 0.3)
		if i == 2 {
			c = ramp(1 - v)
		}
		ccArc2(img, x, y, mtR-14, mtA0, a, mix(bg, c, 0.85), 14)
		wideLine(img, int(x), int(y), int(x+math.Cos(a)*(mtR-30)), int(y+math.Sin(a)*(mtR-30)), txt, 4)
		disc(img, x, y, 14, txt)
		disc(img, x, y, 6, bg)
		s := m.labels[i]
		text(img, x-measure(s, 52)/2, int(y)+52, s, 52, c)
	}
	return img
}

// ccArc2 strokes an arc between two angles.
func ccArc2(img *image.RGBA, cx, cy, r, a0, a1 float64, c color.RGBA, w int) {
	n := max(2, int(math.Abs(a1-a0)*24))
	pts := make([][2]float64, 0, n+1)
	for i := 0; i <= n; i++ {
		a := a0 + (a1-a0)*float64(i)/float64(n)
		pts = append(pts, [2]float64{cx + r*math.Cos(a), cy + r*math.Sin(a)})
	}
	polyline(img, pts, c, w)
}

func (m *ClaudeMeter) base(img *image.RGBA, u *feeds.ClaudeUsage) {
	var ctx int64
	who := "NO SESSION"
	m.busy = false
	for _, s := range u.Sessions {
		if s.Busy {
			m.busy = true
		}
		if s.Context > ctx {
			ctx, who = s.Context, strings.ToUpper(s.Name)
		}
	}
	span := 200_000.0
	if ctx > 200_000 {
		span = 1_000_000
	}
	m.target = [3]float64{math.Sqrt(math.Min(1, m.live.rate/mtMax)), float64(ctx) / span, u.Today.CacheHit()}
	m.labels = [3]string{tok(int64(m.live.rate)), tok(ctx), fmt.Sprintf("%.0f%%", u.Today.CacheHit()*100)}
	titles := [3]string{"TOKENS A MINUTE", "CONTEXT  " + who, "CACHE HITS"}
	scales := [3][2]string{{"0", "20K"}, {"0", tok(int64(span))}, {"0", "100%"}}
	for i, x := range mtX {
		ccArc2(img, x, mtY, mtR-14, mtA0, mtA1, mix(bg, line, 1), 14)
		for k := 0; k <= 10; k++ {
			a := mtA0 + float64(k)/10*(mtA1-mtA0)
			r0 := mtR + 2
			if k%5 == 0 {
				r0 = mtR - 6
			}
			wideLine(img, int(x+math.Cos(a)*r0), int(mtY+math.Sin(a)*r0), int(x+math.Cos(a)*(mtR+12)), int(mtY+math.Sin(a)*(mtR+12)), txt3, 2)
		}
		t := fit(titles[i], 22, 2*mtR+60)
		text(img, x-measure(t, 22)/2, 30, t, 22, cyan)
		a0, a1 := mtA0, mtA1
		text(img, x+math.Cos(a0)*(mtR+18)-10, int(mtY+math.Sin(a0)*(mtR+18)), scales[i][0], 16, txt3)
		text(img, x+math.Cos(a1)*(mtR+18)-measure(scales[i][1], 16)+10, int(mtY+math.Sin(a1)*(mtR+18)), scales[i][1], 16, txt3)
	}
	for _, x := range mtX {
		ccLogo(img, x, mtY-96, 24, 0.2, claudeOrange)
	}
}
