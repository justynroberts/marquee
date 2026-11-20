package dashboard

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"math/rand/v2"
	"sort"
	"strings"
	"time"

	"github.com/justynroberts/marquee/internal/feeds"
)

// Three playful views of the same Claude Code usage as the claude screen: the hive (each
// running session an orb, sparks of output flying into the Claude spark), the city (the week as
// a night skyline, one tower per hour) and the flow (today's output as a river by project).

var claudeOrange = color.RGBA{236, 128, 92, 255}

// ccLive polls the shared scanner and works out how fast tokens are arriving.
type ccLive struct {
	poll     poller[*feeds.ClaudeUsage]
	scan     func(time.Time) (*feeds.ClaudeUsage, error)
	got      time.Time
	prevOut  int64
	prevSess map[string]int64
	rate     float64            // output tokens a minute, all sessions
	sessRate map[string]float64 // by session name
}

func newCCLive() *ccLive {
	ccOnce.Do(func() { ccScanner = feeds.NewClaudeScanner(feeds.ClaudeHome()) })
	return &ccLive{poll: poller[*feeds.ClaudeUsage]{every: 8 * time.Second}, scan: ccScanner.Scan, sessRate: map[string]float64{}}
}

func (l *ccLive) get() (*feeds.ClaudeUsage, time.Time, bool, error) {
	u, got, ok, err := l.poll.get("", func(string) (*feeds.ClaudeUsage, error) { return l.scan(time.Now()) })
	if ok && got != l.got {
		if !l.got.IsZero() {
			mins := got.Sub(l.got).Minutes()
			if mins > 0 {
				ease := func(old, now float64) float64 { return old*0.4 + math.Max(0, now)*0.6 }
				l.rate = ease(l.rate, float64(u.Today.Out-l.prevOut)/mins)
				for _, s := range u.Sessions {
					if p, ok := l.prevSess[s.Name]; ok {
						l.sessRate[s.Name] = ease(l.sessRate[s.Name], float64(s.Today.Out-p)/mins)
					}
				}
			}
		}
		l.got, l.prevOut, l.prevSess = got, u.Today.Out, map[string]int64{}
		for _, s := range u.Sessions {
			l.prevSess[s.Name] = s.Today.Out
		}
	}
	return u, got, ok, err
}

func (l *ccLive) rateLine() (string, color.RGBA) {
	if l.rate >= 50 {
		return tok(int64(l.rate)) + " TOKENS A MINUTE", green
	}
	return "QUIET JUST NOW", txt3
}

// ccLogo draws the Claude spark: tapered rays of uneven length round a small heart.
var ccRays = []float64{1, 0.82, 0.95, 0.78, 1, 0.86, 0.97, 0.8, 0.92, 0.84, 1, 0.79}

func ccLogo(img *image.RGBA, cx, cy, r, rot float64, c color.RGBA) {
	n := len(ccRays)
	for i, k := range ccRays {
		a := rot + float64(i)*2*math.Pi/float64(n)
		dx, dy := math.Cos(a), math.Sin(a)
		seg := func(f0, f1, w float64) {
			wideLine(img, int(cx+dx*r*k*f0), int(cy+dy*r*k*f0), int(cx+dx*r*k*f1), int(cy+dy*r*k*f1), c, max(1, int(w)))
		}
		seg(0, 0.45, r*0.2)
		seg(0.4, 0.75, r*0.14)
		seg(0.7, 1, r*0.08)
	}
	disc(img, cx, cy, r*0.22, c)
}

// activityLine says what a session is doing: the tool it is running (from the hooks) when
// there is one, else working or idle and for how long.
func activityLine(s feeds.ClaudeSession, now map[string]feeds.ClaudeNow) (string, color.RGBA) {
	if a, ok := now[s.ID]; ok && time.Since(a.Since) < 10*time.Minute {
		switch a.State {
		case "tool":
			return strings.ToUpper(a.Tool) + "  " + a.Detail, toolColour(a.Tool)
		case "thinking":
			return "THINKING  " + agoShort(time.Since(a.Since)), violet
		}
	}
	st, sc := "IDLE", txt3
	if s.Busy {
		st, sc = "WORKING", claudeOrange
	}
	if !s.Since.IsZero() {
		st += " " + agoShort(time.Since(s.Since))
	}
	return st, sc
}

// toolColour groups Claude Code's tools: changing files amber, running commands green, reading
// cyan, agents violet.
func toolColour(tool string) color.RGBA {
	switch tool {
	case "Edit", "Write", "MultiEdit", "NotebookEdit":
		return amber
	case "Bash", "PowerShell":
		return green
	case "Read", "Grep", "Glob", "LS", "WebFetch", "WebSearch":
		return cyan
	case "Task", "Agent", "Workflow", "SendMessage":
		return violet
	}
	return claudeOrange
}

// ccArc strokes part of a circle, clockwise from the top.
func ccArc(img *image.RGBA, cx, cy, r, frac float64, c color.RGBA, w int) {
	if frac <= 0 {
		return
	}
	n := max(2, int(64*math.Min(frac, 1)))
	pts := make([][2]float64, 0, n+1)
	for i := 0; i <= n; i++ {
		a := -math.Pi/2 + 2*math.Pi*math.Min(frac, 1)*float64(i)/float64(n)
		pts = append(pts, [2]float64{cx + r*math.Cos(a), cy + r*math.Sin(a)})
	}
	polyline(img, pts, c, w)
}

func commas(n int64) string {
	s := fmt.Sprint(n)
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	return s
}

func ctxFrac(n int64) float64 {
	if n > 200_000 {
		return float64(n) / 1_000_000
	}
	return float64(n) / 200_000
}

// ------------------------------------------------------------------------------------ hive

// ClaudeHive shows each running session as an orb ringed by how full its conversation is.
// Working orbs spin the spark and throw sparks across to the big Claude spark, which spins
// faster the more tokens are flowing; idle orbs doze.
type ClaudeHive struct {
	live  *ccLive
	cache frameCache
	orbs  []ccOrb
	disp  float64
	rot   float64
	spark []ccSpark
	rng   *rand.Rand
	now   map[string]feeds.ClaudeNow
}

type ccOrb struct {
	x, y float64
	s    feeds.ClaudeSession
	acc  float64
}

type ccSpark struct {
	x0, y0, cx, cy, x1, y1 float64
	t0, life               float64
	c                      color.RGBA
}

func NewClaudeHive() *ClaudeHive {
	return &ClaudeHive{live: newCCLive(), rng: rand.New(rand.NewPCG(7, 11))}
}

const (
	hvLogoX, hvLogoY = pad + 50, 98
	hvX0, hvX1       = 600, W - pad
	hvOrbY           = 178
	hvOrbR           = 64
)

func (h *ClaudeHive) Frame(t, dt float64, now time.Time) *image.RGBA {
	u, got, ok, err := h.live.get()
	if !ok {
		return waitMessage(&h.cache, "CLAUDE CODE", err)
	}
	img := h.cache.get(fmt.Sprint(got.UnixNano(), now.Format("15:04")), func(img *image.RGBA) { h.base(img, u) })

	target := float64(u.Today.Out)
	if h.disp == 0 || h.disp > target {
		h.disp = target
	}
	h.disp += (target - h.disp) * math.Min(1, dt*1.5)
	s := commas(int64(h.disp + 0.5))
	px := 80
	for px > 40 && measure(s, px) > 500 {
		px -= 4
	}
	text(img, pad, 170, s, px, amber)

	h.rot += dt * (0.12 + math.Min(2.5, h.live.rate/2500))
	glow := 0.5 + 0.5*math.Sin(t*2)
	disc(img, hvLogoX, hvLogoY, 58+4*glow, mix(bg, claudeOrange, 0.10))
	ccLogo(img, hvLogoX, hvLogoY, 48, h.rot, claudeOrange)

	h.now = feeds.Activity.Now()
	for i := range h.orbs {
		o := &h.orbs[i]
		h.drawOrb(img, o, t)
		if o.s.Busy {
			o.acc += dt * math.Max(3, math.Min(25, h.live.sessRate[o.s.Name]/300))
			for ; o.acc >= 1; o.acc-- {
				h.emit(o, t)
			}
		}
	}
	keep := h.spark[:0]
	for _, p := range h.spark {
		u := (t - p.t0) / p.life
		if u >= 1 || u < 0 {
			continue
		}
		keep = append(keep, p)
		a, b, c := (1-u)*(1-u), 2*u*(1-u), u*u
		x, y := a*p.x0+b*p.cx+c*p.x1, a*p.y0+b*p.cy+c*p.y1
		disc(img, x, y, 2.6, mix(p.c, claudeOrange, u))
	}
	h.spark = keep
	return img
}

func (h *ClaudeHive) emit(o *ccOrb, t float64) {
	if len(h.spark) > 400 {
		return
	}
	a := h.rng.Float64() * 2 * math.Pi
	x0, y0 := o.x+math.Cos(a)*hvOrbR*0.7, o.y+math.Sin(a)*hvOrbR*0.7
	cs := []color.RGBA{amber, txt, claudeOrange, cyan}
	h.spark = append(h.spark, ccSpark{
		x0: x0, y0: y0,
		cx: (x0+hvLogoX)/2 + (h.rng.Float64()-0.5)*300, cy: -60 + h.rng.Float64()*120,
		x1: hvLogoX + (h.rng.Float64()-0.5)*16, y1: hvLogoY + (h.rng.Float64()-0.5)*16,
		t0: t, life: 1.6 + h.rng.Float64()*1.4, c: cs[h.rng.IntN(len(cs))],
	})
}

func (h *ClaudeHive) drawOrb(img *image.RGBA, o *ccOrb, t float64) {
	ph := float64(len(o.s.Name)) // a little offset so orbs don't breathe in step
	st, sc := activityLine(o.s, h.now)
	text(img, o.x-measure(fit(st, 16, 290), 16)/2, int(o.y)+hvOrbR+78, fit(st, 16, 290), 16, sc)
	ring(img, int(o.x), int(o.y), hvOrbR+10, line)
	f := ctxFrac(o.s.Context)
	ccArc(img, o.x, o.y, hvOrbR+10, f, ramp(f*1.2-0.2), 5)
	if !o.s.Busy {
		disc(img, o.x, o.y, hvOrbR*0.55, mix(bg, violet, 0.4))
		for k := 0; k < 3; k++ {
			p := math.Mod(t*0.3+float64(k)/3+ph*0.1, 1)
			text(img, o.x+18+p*34, int(o.y-hvOrbR-12-p*56), "z", 14+int(p*12), mix(bg, txt2, 1-p))
		}
		return
	}
	b := 0.5 + 0.5*math.Sin(t*3+ph)
	disc(img, o.x, o.y, hvOrbR+4+6*b, mix(bg, claudeOrange, 0.14))
	disc(img, o.x, o.y, hvOrbR*0.78, mix(bg, claudeOrange, 0.3))
	ccLogo(img, o.x, o.y, hvOrbR*0.7, t*1.6+ph, color.RGBA{255, 214, 190, 255})
	for k := 0; k < 3; k++ {
		a := t*1.8 + float64(k)*2*math.Pi/3 + ph
		disc(img, o.x+math.Cos(a)*(hvOrbR+22), o.y+math.Sin(a)*(hvOrbR+22)*0.5, 3.5, cyan)
	}
}

func (h *ClaudeHive) base(img *image.RGBA, u *feeds.ClaudeUsage) {
	text(img, pad+120, 70, "CLAUDE CODE", 30, claudeOrange)
	text(img, pad+120, 108, "TODAY", 20, txt3)
	text(img, pad, 268, "OUTPUT TOKENS TODAY", 20, txt2)
	r, rc := h.live.rateLine()
	text(img, pad, 310, r, 22, rc)
	text(img, pad, 352, fmt.Sprintf("CACHE HITS %.0f%%   %d CALLS", u.Today.CacheHit()*100, u.Today.Calls), 20, txt2)
	var wk int64
	for _, d := range u.Days {
		wk += d.Out
	}
	text(img, pad, 392, "THIS WEEK  "+tok(wk), 20, violet)
	vrule(img, hvX0-40)

	// today's hours, faint, along the bottom
	var top int64 = 1
	for _, v := range u.Hours {
		top = max(top, v.Out)
	}
	step := float64(hvX1-hvX0) / 24
	now := time.Now().Hour()
	for i, v := range u.Hours {
		bx := hvX0 + int(float64(i)*step)
		bh := int(float64(v.Out) / float64(top) * 44)
		c := mix(bg, violet, 0.45)
		if i == now {
			c = claudeOrange
		}
		if v.Out > 0 {
			rect(img, bx, 420-max(bh, 2), bx+int(step)-5, 420, c)
		}
		if i%6 == 0 {
			text(img, float64(bx), 426, fmt.Sprintf("%02d:00", i), 14, txt3)
		}
	}

	ss := u.Sessions
	if len(ss) > 6 {
		ss = ss[:6]
	}
	prev := map[string]float64{}
	for _, o := range h.orbs {
		prev[o.s.Name] = o.acc
	}
	h.orbs = h.orbs[:0]
	if len(ss) == 0 {
		msg := "ALL QUIET  ·  NO SESSIONS RUNNING"
		text(img, float64(hvX0+hvX1)/2-measure(msg, 24)/2, hvOrbY-12, msg, 24, txt3)
		return
	}
	slot := math.Min(300, float64(hvX1-hvX0)/float64(len(ss)))
	x := float64(hvX0+hvX1)/2 - slot*float64(len(ss))/2 + slot/2
	for _, s := range ss {
		h.orbs = append(h.orbs, ccOrb{x: x, y: hvOrbY, s: s, acc: prev[s.Name]})
		name := fit(strings.ToUpper(s.Name), 20, slot-20)
		text(img, x-measure(name, 20)/2, hvOrbY+hvOrbR+26, name, 20, txt)
		info := tok(s.Context) + " CTX  ·  " + tok(s.Today.Out) + " OUT"
		text(img, x-measure(info, 16)/2, hvOrbY+hvOrbR+54, info, 16, txt2)
		x += slot
	}
}

// ------------------------------------------------------------------------------------ city

// ClaudeCity is the week as a skyline at night: a district a day, a tower an hour, each as tall
// as that hour's output, its windows lit. This hour's tower is still going up under a crane.
type ClaudeCity struct {
	live   *ccLive
	cache  frameCache
	stars  [][3]float64 // x, y, phase
	beacon [2]float64
	site   [4]int // the tower going up: x0, y0, x1, y1
	rng    *rand.Rand
}

func NewClaudeCity() *ClaudeCity {
	c := &ClaudeCity{live: newCCLive(), rng: rand.New(rand.NewPCG(3, 5))}
	for range 90 {
		c.stars = append(c.stars, [3]float64{c.rng.Float64() * W, 8 + c.rng.Float64()*240, c.rng.Float64() * 6})
	}
	return c
}

const (
	ctGround = 414
	ctTall   = 290
)

func (c *ClaudeCity) Frame(t, dt float64, now time.Time) *image.RGBA {
	u, got, ok, err := c.live.get()
	if !ok {
		return waitMessage(&c.cache, "CLAUDE CODE", err)
	}
	img := c.cache.get(fmt.Sprint(got.UnixNano(), now.Format("15:04")), func(img *image.RGBA) { c.base(img, u, now) })
	for _, s := range c.stars {
		x, y := int(s[0]), int(s[1])
		o := img.PixOffset(x, y)
		if img.Pix[o] != bg.R || img.Pix[o+1] != bg.G || img.Pix[o+2] != bg.B {
			continue
		}
		k := 0.5 + 0.5*math.Sin(t*(0.6+s[2]*0.3)+s[2])
		rect(img, x, y, x+1, y+1, mix(bg, txt, 0.25+0.6*k))
	}
	// the site: windows flicker on as work goes on, and the crane's light blinks
	if s := c.site; s[2] > s[0] && c.live.rate > 0 {
		for range 3 {
			wx := s[0] + 2 + c.rng.IntN(max(1, (s[2]-s[0]-3)/3))*3
			wy := s[1] + 4 + c.rng.IntN(max(1, (s[3]-s[1]-6)/6))*6
			rect(img, wx, wy, wx+1, wy+2, color.RGBA{255, 236, 200, 255})
		}
	}
	if math.Mod(t, 1.2) < 0.5 {
		disc(img, c.beacon[0], c.beacon[1], 4, hot)
	}
	return img
}

func ctHash(a, b, c, d int) int {
	h := uint32(a*73856093 ^ b*19349663 ^ c*83492791 ^ d*2654435761)
	h ^= h >> 13
	h *= 0x5bd1e995
	h ^= h >> 15
	return int(h % 100)
}

func (c *ClaudeCity) base(img *image.RGBA, u *feeds.ClaudeUsage, now time.Time) {
	var top int64 = 1
	var best struct {
		d, h int
		v    int64
	}
	for d := range u.Week {
		for h, v := range u.Week[d] {
			top = max(top, v)
			if v > best.v {
				best.d, best.h, best.v = d, h, v
			}
		}
	}
	gap := 26.0
	tw := (float64(W-2*pad) - 6*gap) / (7 * 24)
	c.site = [4]int{}
	c.beacon = [2]float64{-100, -100}
	for d := range 7 {
		dx := float64(pad) + float64(d)*(24*tw+gap)
		today := d == 6
		for h := range 24 {
			v := u.Week[d][h]
			x0 := int(dx + float64(h)*tw)
			x1 := int(dx+float64(h+1)*tw) - 2
			if today && h > now.Hour() {
				continue // not built yet
			}
			ht := 3
			if v > 0 {
				ht = max(6, int(math.Sqrt(float64(v)/float64(top))*ctTall))
			}
			y0 := ctGround - ht
			body := mix(bg, violet, 0.3)
			win := mix(bg, amber, 0.75)
			if today {
				body, win = mix(bg, claudeOrange, 0.32), color.RGBA{255, 220, 170, 255}
			}
			rect(img, x0, y0, x1, ctGround, body)
			lit := 30 + int(55*math.Sqrt(float64(v)/float64(top)))
			for wy, row := y0+4, 0; wy+2 < ctGround-2; wy, row = wy+6, row+1 {
				for wx, col := x0+2, 0; wx+1 < x1; wx, col = wx+3, col+1 {
					if v > 0 && ctHash(d, h, row, col) < lit {
						rect(img, wx, wy, wx+1, wy+2, win)
					}
				}
			}
			if today && h == now.Hour() {
				c.site = [4]int{x0, y0, x1, ctGround}
				// a crane over the site
				mx := x1 + 6
				cy := min(y0-30, ctGround-60)
				wideLine(img, mx, ctGround, mx, cy, txt3, 2)
				wideLine(img, mx-50, cy, mx+18, cy, txt3, 2)
				wideLine(img, mx-40, cy, mx-40, y0-8, txt3, 1)
				c.beacon = [2]float64{float64(mx), float64(cy - 6)}
			}
		}
		lbl := strings.ToUpper(u.Days[d].Day.Format("Mon"))
		lc := txt3
		if today {
			lbl, lc = "TODAY", claudeOrange
		}
		text(img, dx, ctGround+10, lbl, 16, lc)
		v := tok(u.Days[d].Out)
		text(img, dx+24*tw-measure(v, 16), ctGround+10, v, 16, txt2)
	}
	rect(img, pad, ctGround, W-pad, ctGround+1, line)

	// the sign
	ccLogo(img, pad+26, pad+18, 22, 0.2, claudeOrange)
	text(img, pad+60, pad, "TOKEN CITY", 26, claudeOrange)
	var wk int64
	for _, d := range u.Days {
		wk += d.Out
	}
	sub := tok(wk) + " OUTPUT THIS WEEK"
	if best.v > 0 {
		sub += fmt.Sprintf("   ·   TALLEST %s %02d:00  %s", strings.ToUpper(u.Days[best.d].Day.Format("Mon")), best.h, tok(best.v))
	}
	text(img, pad+60, pad+36, sub, 16, txt2)
}

// ------------------------------------------------------------------------------------ flow

// ClaudeFlow is today's output as a river: one band per project, as wide as its output each
// hour, with sparks drifting downstream as fast as tokens are arriving.
type ClaudeFlow struct {
	live  *ccLive
	cache frameCache
	bands []flBand
	nowX  int
	parts []flPart
	acc   float64
	rng   *rand.Rand
}

type flBand struct {
	name     string
	c        color.RGBA
	top, bot []int16 // per column from flX0
	weight   float64 // share of this hour
}

type flPart struct {
	x, v, speed float64
	b           int
}

func NewClaudeFlow() *ClaudeFlow {
	return &ClaudeFlow{live: newCCLive(), rng: rand.New(rand.NewPCG(9, 2))}
}

const (
	flX0, flX1 = 420, W - pad
	flMid      = 226
	flHalf     = 184
)

var flColours = []color.RGBA{claudeOrange, cyan, violet, green, amber, {255, 120, 170, 255}, txt3}

func (f *ClaudeFlow) Frame(t, dt float64, now time.Time) *image.RGBA {
	u, got, ok, err := f.live.get()
	if !ok {
		return waitMessage(&f.cache, "CLAUDE CODE", err)
	}
	img := f.cache.get(fmt.Sprint(got.UnixNano(), now.Format("15:04")), func(img *image.RGBA) { f.base(img, u, now) })
	if len(f.bands) == 0 {
		return img
	}
	f.acc += dt * math.Max(2, math.Min(40, f.live.rate/200))
	for ; f.acc >= 1 && len(f.parts) < 500; f.acc-- {
		r, b := f.rng.Float64(), 0
		for i, bd := range f.bands {
			if r -= bd.weight; r <= 0 {
				b = i
				break
			}
		}
		f.parts = append(f.parts, flPart{x: float64(flX0), v: 0.15 + f.rng.Float64()*0.7, speed: 160 + f.rng.Float64()*160, b: b})
	}
	keep := f.parts[:0]
	for _, p := range f.parts {
		p.x += p.speed * dt
		xi := int(p.x) - flX0
		if xi >= f.nowX-flX0 || xi < 0 {
			continue
		}
		keep = append(keep, p)
		bd := f.bands[p.b]
		top, bot := float64(bd.top[xi]), float64(bd.bot[xi])
		if bot-top < 3 {
			continue
		}
		disc(img, p.x, top+p.v*(bot-top), 2.4, mix(bd.c, txt, 0.55))
	}
	f.parts = keep
	return img
}

func (f *ClaudeFlow) base(img *image.RGBA, u *feeds.ClaudeUsage, now time.Time) {
	ccLogo(img, pad+30, pad+22, 26, 0.4, claudeOrange)
	text(img, pad+70, pad+4, "TODAY'S FLOW", 26, claudeOrange)
	text(img, pad, 96, tok(u.Today.Out), 60, amber)
	text(img, pad, 168, "OUTPUT TOKENS", 18, txt2)
	r, rc := f.live.rateLine()
	text(img, pad, 196, r, 18, rc)

	// the biggest projects get their own band, the rest share one
	type proj struct {
		name  string
		hours [24]int64
		total int64
	}
	var ps []proj
	for name, hrs := range u.ProjectHours {
		var tot int64
		for _, v := range hrs {
			tot += v
		}
		if tot > 0 {
			ps = append(ps, proj{name, hrs, tot})
		}
	}
	sort.Slice(ps, func(i, j int) bool { return ps[i].total > ps[j].total })
	if len(ps) > 6 {
		other := proj{name: "OTHERS"}
		for _, p := range ps[6:] {
			for h := range 24 {
				other.hours[h] += p.hours[h]
			}
			other.total += p.total
		}
		ps = append(ps[:6], other)
	}
	f.bands = f.bands[:0]
	// the river runs from just before the first work today to just after now
	first := now.Hour()
	for _, p := range ps {
		for h := range 24 {
			if p.hours[h] > 0 && h < first {
				first = h
			}
		}
	}
	hEnd := float64(now.Hour()) + 1.5
	hStart := math.Max(0, float64(first)-1)
	if hEnd-hStart < 6 {
		hStart = math.Max(0, hEnd-6)
	}
	span := hEnd - hStart
	hx := func(hf float64) int { return flX0 + int(float64(flX1-flX0)*(hf-hStart)/span) }
	f.nowX = hx(minutesOfDay(now) / 60)
	for h := int(math.Ceil(hStart)); float64(h) <= hEnd; h++ {
		if step := max(1, int(span/8)); h%step == 0 {
			x := hx(float64(h))
			text(img, float64(x)-18, 428, fmt.Sprintf("%02d:00", h%24), 14, txt3)
		}
	}
	if len(ps) == 0 {
		text(img, flX0, flMid-12, "NOTHING YET TODAY", 24, txt3)
		return
	}

	// each column's value per band, eased between hour centres
	val := func(p proj, x int) float64 {
		hf := hStart + float64(x-flX0)/float64(flX1-flX0)*span - 0.5
		h0 := int(math.Floor(hf))
		k := hf - float64(h0)
		k = (1 - math.Cos(k*math.Pi)) / 2
		at := func(h int) float64 {
			if h < 0 || h > now.Hour() || h > 23 {
				return 0
			}
			return float64(p.hours[h])
		}
		return at(h0)*(1-k) + at(h0+1)*k
	}
	var top float64 = 1
	cols := f.nowX - flX0
	vals := make([][]float64, len(ps))
	for i, p := range ps {
		vals[i] = make([]float64, cols)
		for xi := range cols {
			vals[i][xi] = val(p, flX0+xi)
		}
	}
	for xi := range cols {
		var s float64
		for i := range ps {
			s += vals[i][xi]
		}
		top = math.Max(top, s)
	}
	var curTotal float64
	for i, p := range ps {
		c := flColours[min(i, len(flColours)-1)]
		if p.name == "OTHERS" {
			c = txt3
		}
		cur := float64(p.hours[now.Hour()])
		curTotal += cur
		f.bands = append(f.bands, flBand{name: p.name, c: c, top: make([]int16, cols), bot: make([]int16, cols), weight: cur})
	}
	for xi := range cols {
		var s float64
		for i := range ps {
			s += vals[i][xi]
		}
		y := float64(flMid) - s/top*flHalf
		for i := range ps {
			h := vals[i][xi] / top * 2 * flHalf
			b := &f.bands[i]
			b.top[xi], b.bot[xi] = int16(y), int16(y+h)
			if h >= 1 {
				rect(img, flX0+xi, int(y), flX0+xi, int(y+h), mix(bg, b.c, 0.5))
			}
			y += h
		}
	}
	for i := range f.bands {
		if curTotal > 0 {
			f.bands[i].weight /= curTotal
		} else {
			f.bands[i].weight = 1 / float64(len(f.bands))
		}
	}
	rect(img, f.nowX, flMid-flHalf-6, f.nowX, flMid+flHalf+6, mix(bg, txt, 0.35))
	text(img, float64(f.nowX)+6, flMid-flHalf-6, "NOW", 14, txt2)

	// the key
	for i, p := range ps {
		y := 240 + i*28
		if y > 420 {
			break
		}
		b := f.bands[i]
		rect(img, pad, y+4, pad+12, y+16, b.c)
		v := tok(p.total)
		text(img, pad+24, y, fit(strings.ToUpper(p.name), 18, 340-pad-24-measure(v, 18)-12), 18, txt)
		text(img, 360-measure(v, 18), y, v, 18, b.c)
	}
}
