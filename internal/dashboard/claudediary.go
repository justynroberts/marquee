package dashboard

import (
	"context"
	"fmt"
	"image"
	"image/color"
	"math"
	"sort"
	"strings"
	"sync/atomic"
	"time"
	"unicode"

	"github.com/justynroberts/marquee/internal/feeds"
)

// Diary is an old diary page that writes itself. A quill writes each line in handwriting; the
// iron-gall ink goes on blue-black and dries to sepia; the entry stays a while, then soaks back
// into the paper unevenly until it is gone, and the next appears. The page is lit by a
// flickering candle. claudediary keeps Claude Code's diary (the plan left, today's work, what it
// is doing, the week); diary is a day book of news, weather, history and the moon with the
// odd Claude entry.

type Diary struct {
	mode   string // "claude" or "news"
	place  func() string
	live   *ccLive
	limits poller[*feeds.ClaudeLimits]
	bbc    poller[[]feeds.Headline]
	hn     poller[[]feeds.Headline]
	hist   poller[[]feeds.Event]
	wx     poller[*feeds.Forecast]
	geo    poller[*feeds.Place]

	page  *image.RGBA
	out   *image.RGBA
	noise []float32 // where the ink soaks away first

	lines []dyLine
	blot  *dyBlot
	t0    float64 // when this entry began
	wait  float64 // when the screen first asked for an entry
	end   float64 // when the writing ends
	seed  int
	// the hand: where the nib is, eased towards where it is going
	qx, qy, qvx, qlift float64
	qon                bool
}

type dyLine struct {
	x, y   float64
	s      string
	font   string // "hand" or "quill"
	px     int
	ink    color.RGBA // dried colour
	wet    bool       // goes on blue-black and dries
	w      float64
	t0, t1 float64 // when the quill starts and finishes it
	under  bool
	words  []dyWord
	base   float64 // baseline, for the quill
	// the hand's path along the line, one entry per dyColW-pixel column: where the ink is in
	// that column, and how far through the line's time the nib reaches it (slower over letters,
	// quick across the gaps between words)
	top, bot []float32
	inked    []bool
	cum      []float64
}

const dyColW = 2

// at is how far through its writing time the nib passes x.
func (l *dyLine) at(x float64) float64 {
	if len(l.cum) == 0 {
		return clamp01((x - l.x) / math.Max(1, l.w))
	}
	f := (x - l.x) / dyColW
	i := int(f)
	switch {
	case i < 0:
		return 0
	case i >= len(l.cum)-1:
		return 1
	}
	return l.cum[i] + (l.cum[i+1]-l.cum[i])*(f-float64(i))
}

// xAt is where the nib is a fraction k through the line's time.
func (l *dyLine) xAt(k float64) (x float64, col int) {
	if len(l.cum) < 2 {
		return l.x + k*l.w, -1
	}
	i := sort.SearchFloat64s(l.cum, k)
	if i <= 0 {
		return l.x, 0
	}
	if i >= len(l.cum) {
		return l.x + l.w, len(l.cum) - 1
	}
	a, b := l.cum[i-1], l.cum[i]
	f := 0.0
	if b > a {
		f = (k - a) / (b - a)
	}
	return l.x + (float64(i-1)+f)*dyColW, i - 1
}

type dyWord struct {
	x, dy float64
	s     string
}

type dyBlot struct {
	x, y, r float64
	at      float64
}

// dyEntry is one diary entry before layout.
type dyEntry struct {
	title   string
	lines   []string
	big     string // written large on the right, in red, underlined
	caption string // under the big figure
	sign    string // signed off in red beneath the body, as for a quotation
}

func NewClaudeDiary() *Diary { return newDiary("claude", nil) }

// NewDiary is the day book: news, weather, history, the moon and a little Claude.
func NewDiary(place func() string) *Diary { return newDiary("news", place) }

// NewDiaryBook is one of the narrower books on the same page: "quotes" (a commonplace book of
// quotations), "nature" (the weather, the light, the moon) or "papers" (headlines and history).
func NewDiaryBook(book string, place func() string) *Diary { return newDiary(book, place) }

func newDiary(mode string, place func() string) *Diary {
	if place == nil {
		place = func() string { return "" }
	}
	return &Diary{mode: mode, place: place, live: newCCLive(), t0: -1,
		limits: poller[*feeds.ClaudeLimits]{every: 3 * time.Minute},
		bbc:    poller[[]feeds.Headline]{every: 15 * time.Minute},
		hn:     poller[[]feeds.Headline]{every: 15 * time.Minute},
		hist:   poller[[]feeds.Event]{every: 6 * time.Hour},
		wx:     poller[*feeds.Forecast]{every: 20 * time.Minute},
		geo:    poller[*feeds.Place]{every: 24 * time.Hour},
	}
}

var (
	dyTurn   = map[string]*atomic.Int64{"claude": {}, "news": {}, "quotes": {}, "nature": {}, "papers": {}}
	dyWet    = color.RGBA{14, 18, 44, 255}
	dyInk    = color.RGBA{58, 32, 14, 255}
	dyRed    = color.RGBA{128, 24, 14, 255}
	dyFaded  = color.RGBA{150, 116, 76, 255}
	dyMargin = 150.0
)

const (
	dyPace    = 15.0 // characters a second
	dyHold    = 7.5
	dySoak    = 3.2
	dyRest    = 1.0
	dyLineH   = 84
	dyFirst   = 182 // first ruled line
	dyBodyPx  = 40
	dyBodyY   = 20     // puts body text on the rules (Homemade Apple's ascenders are tall)
	dyRight   = 1360.0 // body text stops here when there is a big figure
	dyQuillUp = 1.0    // seconds the quill takes to lift away
)

func (d *Diary) Frame(t, dt float64, now time.Time) *image.RGBA {
	if d.page == nil {
		d.makePage()
	}
	if d.t0 < 0 || t-d.t0 > d.end+dyHold+dySoak+dyRest {
		if !d.compose(t, now) {
			copy(d.out.Pix, d.page.Pix)
			d.candle(t)
			return d.out
		}
	}
	copy(d.out.Pix, d.page.Pix)
	e := t - d.t0
	soak := 0.0
	if s := e - d.end - dyHold; s > 0 {
		soak = math.Min(1.25, s/dySoak)
	}
	for i := range d.lines {
		d.inkLine(&d.lines[i], e, soak)
	}
	if b := d.blot; b != nil && e > b.at {
		d.inkBlot(b, e, soak)
	}
	d.quill(e, dt)
	d.candle(t)
	return d.out
}

// ---------------------------------------------------------------- ink

func (d *Diary) inkLine(l *dyLine, e, soak float64) {
	if e < l.t0 {
		return
	}
	UseFont(l.font)
	defer UseFont("hand")
	for _, w := range l.words {
		f := faceFor(w.s, l.px)
		if f == nil {
			continue
		}
		ix := int(w.x)
		m, ox, oy := f.Mask(w.s, w.x-float64(ix))
		for j := 0; j < m.Rect.Dy(); j++ {
			Y := int(l.y+w.dy) + oy + j
			if Y < 0 || Y >= H {
				continue
			}
			for i := 0; i < m.Rect.Dx(); i++ {
				a := float64(m.Pix[j*m.Stride+i]) / 255
				if a == 0 {
					continue
				}
				X := ix + ox + i
				if X < 0 || X >= W {
					continue
				}
				tw := l.t0 + l.at(float64(X))*(l.t1-l.t0) // when the nib passed here
				k := clamp01((e - tw) / 0.22)
				if k <= 0 {
					continue
				}
				c := l.ink
				if l.wet {
					c = mix(dyWet, l.ink, clamp01((e-tw-0.4)/2.6))
				}
				if soak > 0 {
					c = mix(c, dyFaded, math.Min(1, soak*1.2))
					k *= clamp01((float64(d.noise[Y*W+X]) - soak) * 4)
					if k <= 0 {
						continue
					}
				}
				d.put(X, Y, a*k, c)
			}
		}
	}
	if l.under && e > l.t1 {
		k := math.Min(1, (e-l.t1)/0.5)
		d.stroke(l.x-8, l.base+18, l.x-8+(l.w+20)*k, soak)
	}
}

// stroke is a wavering underline drawn as far as x1.
func (d *Diary) stroke(x0, y, x1, soak float64) {
	for x := x0; x < x1; x++ {
		yy := y + 3*math.Sin(x/41+float64(d.seed)) + 1.5*math.Sin(x/13)
		for w := -2.5; w <= 2.5; w++ {
			X, Y := int(x), int(yy+w)
			if X < 0 || X >= W || Y < 0 || Y >= H {
				continue
			}
			k := 1 - math.Abs(w)/3.5
			c := dyRed
			if soak > 0 {
				c = mix(c, dyFaded, math.Min(1, soak*1.2))
				k *= clamp01((float64(d.noise[Y*W+X]) - soak) * 4)
			}
			d.put(X, Y, k, c)
		}
	}
}

// inkBlot is a drop of ink from the nib: it spreads in a moment, then dries and soaks like the
// writing.
func (d *Diary) inkBlot(b *dyBlot, e, soak float64) {
	grow := math.Min(1, (e-b.at)/0.35)
	drops := [][3]float64{{0, 0, 1}, {1.5, -0.6, 0.22}, {-1.3, 0.9, 0.16}, {2.1, 0.7, 0.12}, {-0.6, -1.6, 0.1}}
	for n, dr := range drops {
		cx, cy, r := b.x+dr[0]*b.r, b.y+dr[1]*b.r, b.r*dr[2]*grow
		for y := int(cy - r*1.4); y <= int(cy+r*1.4); y++ {
			for x := int(cx - r*1.4); x <= int(cx+r*1.4); x++ {
				if x < 0 || x >= W || y < 0 || y >= H {
					continue
				}
				a := math.Atan2(float64(y)-cy, float64(x)-cx)
				edge := r * (0.85 + 0.18*math.Sin(a*5+float64(d.seed+n)) + 0.1*math.Sin(a*11+float64(n)))
				dist := math.Hypot(float64(x)-cx, float64(y)-cy)
				if dist > edge {
					continue
				}
				k := clamp01((edge - dist) / 2)
				c := mix(dyWet, dyInk, clamp01((e-b.at-0.5)/3))
				if soak > 0 {
					c = mix(c, dyFaded, math.Min(1, soak*1.2))
					k *= clamp01((float64(d.noise[y*W+x]) - soak) * 4)
				}
				d.put(x, y, k, c)
			}
		}
	}
}

func (d *Diary) put(X, Y int, a float64, c color.RGBA) {
	o := d.out.PixOffset(X, Y)
	p := d.out.Pix[o : o+3 : o+3]
	p[0] = uint8(float64(p[0]) + (float64(c.R)-float64(p[0]))*a)
	p[1] = uint8(float64(p[1]) + (float64(c.G)-float64(p[1]))*a)
	p[2] = uint8(float64(p[2]) + (float64(c.B)-float64(p[2]))*a)
}

// shade darkens the page along a thick segment (the quill's shadow).
func (d *Diary) shade(x0, y0, x1, y1, w, k float64) {
	minX, maxX := int(math.Min(x0, x1)-w), int(math.Max(x0, x1)+w)
	minY, maxY := int(math.Min(y0, y1)-w), int(math.Max(y0, y1)+w)
	dx, dy := x1-x0, y1-y0
	l2 := math.Max(1e-6, dx*dx+dy*dy)
	for y := max(0, minY); y <= min(H-1, maxY); y++ {
		for x := max(0, minX); x <= min(W-1, maxX); x++ {
			t := clamp01(((float64(x)-x0)*dx + (float64(y)-y0)*dy) / l2)
			dist := math.Hypot(float64(x)-(x0+t*dx), float64(y)-(y0+t*dy))
			if dist > w {
				continue
			}
			f := 1 - k*clamp01((w-dist)/4)
			o := d.out.PixOffset(x, y)
			d.out.Pix[o] = uint8(float64(d.out.Pix[o]) * f)
			d.out.Pix[o+1] = uint8(float64(d.out.Pix[o+1]) * f)
			d.out.Pix[o+2] = uint8(float64(d.out.Pix[o+2]) * f)
		}
	}
}

func clamp01(v float64) float64 { return math.Max(0, math.Min(1, v)) }

// ---------------------------------------------------------------- the quill

// quill draws the pen. Its target follows the hand's path: along the ink of the letter being
// written (up for tall letters, down for descenders), lifted and quick across the gaps between
// words, arcing back to the start of the next line, gliding in at the start and lifting away
// at the end. The nib eases towards the target, so it moves like a hand rather than a cursor,
// and the shaft leans with the motion.
func (d *Diary) quill(e, dt float64) {
	if len(d.lines) == 0 {
		return
	}
	tx, ty, lift, ok := d.quillTarget(e)
	if !ok {
		d.qon = false
		return
	}
	if !d.qon {
		d.qx, d.qy, d.qvx, d.qlift, d.qon = tx, ty, 0, lift, true
	}
	ease := 1 - math.Exp(-dt*16)
	nx := d.qx + (tx-d.qx)*ease
	ny := d.qy + (ty-d.qy)*ease
	if dt > 0 {
		d.qvx += ((nx-d.qx)/dt - d.qvx) * (1 - math.Exp(-dt*6))
	}
	d.qx, d.qy = nx, ny
	d.qlift += (lift - d.qlift) * (1 - math.Exp(-dt*10))
	d.drawQuill(nx, ny, d.qlift, e)
}

// quillTarget is where the nib is heading at e, and how high it is off the paper (0..1).
func (d *Diary) quillTarget(e float64) (x, y, lift float64, ok bool) {
	first := d.lines[0]
	if e < first.t0 { // glide in from the upper right
		k := clamp01(e / first.t0)
		k = k * k * (3 - 2*k)
		sx, sy := first.x+700, first.base-260
		return sx + (first.x-sx)*k, sy + (first.base-sy)*k, 1 - k, true
	}
	for i := range d.lines {
		l := &d.lines[i]
		if e < l.t0 || e > l.t1 {
			continue
		}
		k := (e - l.t0) / math.Max(0.01, l.t1-l.t0)
		x, col := l.xAt(k)
		y = l.base - 4
		if col >= 0 && col < len(l.inked) {
			if l.inked[col] {
				// sit within the ink: the middle of the stroke, pulled towards its top on tall
				// letters and its foot on descenders, with a small pen wobble
				mid := float64(l.top[col]+l.bot[col]) / 2
				y = mid + 4*math.Sin(x/7+float64(d.seed))
			} else {
				y, lift = l.base-16, 0.35 // between words the nib comes up off the page
			}
		}
		return x, y, lift, true
	}
	// drawing an underline beneath a big figure, left to right
	for _, l := range d.lines {
		if l.under && e > l.t1 && e < l.t1+0.5 {
			k := (e - l.t1) / 0.5
			x0 := l.x - 8
			return x0 + (l.w+20)*k, l.base + 18 + 3*math.Sin((x0+(l.w+20)*k)/41+float64(d.seed)), 0, true
		}
	}
	// between lines, or after the last
	prev := first
	for _, l := range d.lines {
		if l.t1 <= e {
			prev = l
		}
	}
	var next *dyLine
	for i := range d.lines {
		if d.lines[i].t0 > e {
			next = &d.lines[i]
			break
		}
	}
	px, py := prev.x+prev.w+10, prev.base-10
	t1 := prev.t1
	if prev.under {
		px, py, t1 = prev.x+prev.w+12, prev.base+18, prev.t1+0.5
	}
	if next == nil {
		up := (e - t1) / dyQuillUp
		if up >= 1 {
			return 0, 0, 0, false
		}
		u := up * up * (3 - 2*up)
		return px + u*620, py - u*440, 0.4 + 0.6*u, true
	}
	k := clamp01((e - t1) / math.Max(0.01, next.t0-t1))
	k = k * k * (3 - 2*k)
	return px + (next.x-px)*k, py + (next.base-4-py)*k - 50*math.Sin(k*math.Pi), 0.35 + 0.5*math.Sin(k*math.Pi), true
}

func (d *Diary) drawQuill(nx, ny, lift, e float64) {
	// lean back a little when moving fast to the right, forward when returning left
	a := 1.05 + 0.035*math.Sin(e*1.7) - math.Max(-0.12, math.Min(0.12, d.qvx*0.00018))
	ux, uy := math.Cos(a), -math.Sin(a)
	px, py := -uy, ux // perpendicular
	L := 360.0
	// the shadow falls further from the nib the higher the pen is lifted
	sh := 10 + lift*48
	d.shade(nx+sh*0.3, ny+sh*0.2, nx+ux*L*0.95+sh*1.4, ny+uy*L*0.95+sh*0.9, 9, 0.3-lift*0.1)
	for s := 0.32; s < 1.0; s += 0.05 {
		h := 40 * math.Pow(math.Sin(math.Pi*(s-0.3)/0.72), 0.7)
		bx, by := nx+ux*L*s+sh*(0.4+s), ny+uy*L*s+sh*(0.3+s*0.6)
		d.shade(bx, by, bx-px*h+ux*h*0.4, by-py*h+uy*h*0.4, 7, 0.18-lift*0.06)
	}
	// vanes: barbs swept back from the shaft, one side a touch darker; they ripple slightly
	for s := 0.3; s < 1.02; s += 0.009 {
		h := 46 * math.Pow(math.Max(0, math.Sin(math.Pi*(s-0.28)/0.76)), 0.7)
		bx, by := nx+ux*L*s, ny+uy*L*s
		ripple := 1 + 0.04*math.Sin(s*40-e*3)
		for side := -1.0; side <= 1; side += 2 {
			hh := h * ripple
			if side > 0 {
				hh *= 0.8
			}
			ex, ey := bx+side*px*hh-ux*hh*0.35, by+side*py*hh-uy*hh*0.35
			c := color.RGBA{238, 233, 222, 255}
			if side < 0 {
				c = color.RGBA{214, 208, 196, 255}
			}
			if int(s*1000)%7 == 0 {
				c = mix(c, color.RGBA{150, 140, 128, 255}, 0.5)
			}
			wideLine(d.out, int(bx), int(by), int(ex), int(ey), c, 2)
		}
	}
	// the shaft and the steel nib
	wideLine(d.out, int(nx+ux*22), int(ny+uy*22), int(nx+ux*L*1.02), int(ny+uy*L*1.02), color.RGBA{246, 240, 226, 255}, 5)
	wideLine(d.out, int(nx+ux*26), int(ny+uy*26), int(nx+ux*80), int(ny+uy*80), color.RGBA{200, 186, 160, 255}, 7)
	wideLine(d.out, int(nx), int(ny), int(nx+ux*26), int(ny+uy*26), color.RGBA{60, 62, 70, 255}, 5)
	wideLine(d.out, int(nx+ux*4), int(ny+uy*4), int(nx+ux*22), int(ny+uy*22), color.RGBA{150, 154, 166, 255}, 2)
}

// candle flickers the light on the page.
func (d *Diary) candle(t float64) {
	f := 0.97 + 0.028*math.Sin(t*7.3)*math.Sin(t*2.1+1) + 0.014*math.Sin(t*17.9) + 0.008*math.Sin(t*31)
	f = math.Min(1, f)
	p := d.out.Pix
	for i := 0; i < len(p); i += 4 {
		p[i] = uint8(float64(p[i]) * f)
		p[i+1] = uint8(float64(p[i+1]) * f * 0.995)
		p[i+2] = uint8(float64(p[i+2]) * f * 0.985)
	}
}

// ---------------------------------------------------------------- the page

// makePage paints aged paper: warm and mottled, darker towards the edges, foxed, with a coffee
// ring, faint rules and a margin, plus the noise field the ink soaks away by.
func (d *Diary) makePage() {
	d.page = image.NewRGBA(image.Rect(0, 0, W, H))
	d.out = image.NewRGBA(image.Rect(0, 0, W, H))
	d.noise = make([]float32, W*H)
	for y := 0; y < H; y++ {
		for x := 0; x < W; x++ {
			fx, fy := float64(x), float64(y)
			mott := 0.55*vnoise(fx/140, fy/140, 1) + 0.3*vnoise(fx/45, fy/45, 2) + 0.15*vnoise(fx/9, fy/9, 3)
			ex := math.Min(fx, float64(W)-fx) / 260
			ey := math.Min(fy, float64(H)-fy) / 120
			edge := math.Min(1, math.Min(ex, ey))
			edge = 0.45 + 0.55*math.Sqrt(math.Max(0, edge))
			l := edge * (0.82 + 0.28*mott)
			r, g, b := 214*l, 186*l, 136*l
			// foxing: small rusty spots
			if f := vnoise(fx/7, fy/7, 9); f > 0.86 && vnoise(fx/90, fy/90, 10) > 0.6 {
				k := (f - 0.86) * 3
				r, g, b = r*(1-k*0.25), g*(1-k*0.35), b*(1-k*0.5)
			}
			// a coffee ring, long dried
			if dist := math.Hypot(fx-1700, fy-330); dist < 98 {
				ring := math.Exp(-math.Pow((dist-90)/4, 2))*0.16 + 0.04
				ring *= 0.7 + 0.6*vnoise(fx/20, fy/20, 4)
				r, g, b = r*(1-ring*0.8), g*(1-ring), b*(1-ring*1.3)
			}
			d.page.SetRGBA(x, y, color.RGBA{uint8(math.Min(255, r)), uint8(math.Min(255, g)), uint8(math.Max(0, math.Min(255, b))), 255})
			d.noise[y*W+x] = float32(0.65*vnoise(fx/60, fy/60, 7) + 0.35*vnoise(fx/14, fy/14, 8))
		}
	}
	for y := dyFirst; y < H-10; y += dyLineH {
		for x := 0; x < W; x++ {
			d.tint(x, y, color.RGBA{90, 110, 140, 255}, 0.25)
		}
	}
	for y := 0; y < H; y++ {
		for _, x := range []int{int(dyMargin) - 22, int(dyMargin) - 16} {
			d.tint(x, y, color.RGBA{170, 60, 50, 255}, 0.3)
		}
	}
}

func (d *Diary) tint(x, y int, c color.RGBA, a float64) {
	o := d.page.PixOffset(x, y)
	p := d.page.Pix
	p[o] = uint8(float64(p[o]) + (float64(c.R)-float64(p[o]))*a)
	p[o+1] = uint8(float64(p[o+1]) + (float64(c.G)-float64(p[o+1]))*a)
	p[o+2] = uint8(float64(p[o+2]) + (float64(c.B)-float64(p[o+2]))*a)
}

// vnoise is smooth value noise in 0..1.
func vnoise(x, y float64, s int) float64 {
	xi, yi := math.Floor(x), math.Floor(y)
	fx, fy := x-xi, y-yi
	h := func(a, b float64) float64 {
		n := int(a)*374761393 + int(b)*668265263 + s*1442695041
		n = (n ^ (n >> 13)) * 1274126177
		return float64((n^(n>>16))&0xffff) / 65535
	}
	sx, sy := fx*fx*(3-2*fx), fy*fy*(3-2*fy)
	a := h(xi, yi) + (h(xi+1, yi)-h(xi, yi))*sx
	b := h(xi, yi+1) + (h(xi+1, yi+1)-h(xi, yi+1))*sx
	return a + (b-a)*sy
}

// ---------------------------------------------------------------- layout

// compose picks and lays out the next entry; false while there is nothing to write yet.
func (d *Diary) compose(t float64, now time.Time) bool {
	builders := d.builders()
	turn := dyTurn[d.mode]
	var en *dyEntry
	for range builders {
		i := int(turn.Load()) % len(builders)
		if en = builders[i](now); en != nil {
			break
		}
		if d.t0 < 0 && t-d.firstWait(t) < 6 {
			return false // give the first entry's data a moment to arrive
		}
		turn.Add(1)
	}
	turn.Add(1)
	if en == nil {
		en = &dyEntry{title: "a quiet page", lines: []string{"Nothing to write just yet."}}
	}
	d.layout(en, now)
	d.t0 = t
	return true
}

func (d *Diary) firstWait(t float64) float64 {
	if d.wait == 0 {
		d.wait = t + 1e-6
	}
	return d.wait
}

func (d *Diary) layout(en *dyEntry, now time.Time) {
	d.seed++
	d.lines, d.blot = nil, nil
	clock := 1.0 // the quill glides in first
	add := func(x, y float64, s, font string, px int, ink color.RGBA, wet, under bool) dyLine {
		UseFont(font)
		defer UseFont("hand")
		l := dyLine{x: x, y: y, s: s, font: font, px: px, ink: ink, wet: wet, under: under}
		// words sit a little above or below the line, as a hand would put them
		cx := x
		for i, w := range strings.Fields(s) {
			if i > 0 {
				cx += measure(" ", px)
			}
			h := float64((d.seed*7919+i*104729+len(w)*31)%7) - 3
			l.words = append(l.words, dyWord{x: cx, dy: h * float64(px) / 60, s: w})
			cx += measure(w, px)
		}
		l.w = cx - x
		l.base = y + float64(px)*0.9
		if f := faceFor("x", px); f != nil {
			m, _, oy := f.Mask("x", 0)
			l.base = y + float64(oy+m.Rect.Dy())
		}
		d.trace(&l)
		dur := float64(len([]rune(s))) / dyPace * float64(px) / dyBodyPx
		dur = math.Max(0.5, math.Min(dur, 6))
		l.t0, l.t1 = clock, clock+dur
		clock = l.t1 + 0.6
		d.lines = append(d.lines, l)
		return l
	}
	right := float64(W) - 90
	if en.big != "" {
		right = dyRight
	}
	head := now.Format("Monday 2 January")
	if en.title != "" {
		head += ", " + en.title
	}
	add(dyMargin, 18, head, "quill", 46, dyRed, false, false)
	y := float64(dyFirst - dyLineH)
	n := 0
	for _, p := range en.lines {
		for _, ln := range wrapWords(p, dyBodyPx, right-dyMargin, "hand") {
			if n == 4 {
				break
			}
			add(dyMargin, y+dyBodyY, ln, "hand", dyBodyPx, dyInk, true, false)
			y += dyLineH
			n++
		}
	}
	if en.sign != "" {
		UseFont("quill")
		sw := measure(en.sign, 54)
		UseFont("hand")
		add(right-sw, y+4, en.sign, "quill", 54, dyRed, false, false)
	}
	if en.big != "" {
		UseFont("quill")
		bw := measure(en.big, 150)
		UseFont("hand")
		l := add(float64(W)-130-bw, 96, en.big, "quill", 150, dyRed, false, true)
		if en.caption != "" {
			UseFont("hand")
			cw := measure(en.caption, 30)
			add(math.Max(dyRight+40, l.x+l.w/2-cw/2), l.base+40, en.caption, "hand", 30, dyInk, true, false)
		}
	}
	d.end = clock
	// now and then the nib drops a blot at the end of a line
	if d.seed%3 == 0 && len(d.lines) > 1 {
		l := d.lines[1+d.seed%(len(d.lines)-1)]
		if l.font == "hand" {
			d.blot = &dyBlot{x: l.x + l.w + 26, y: l.base - 8, r: 9 + float64(d.seed%5), at: l.t1 + 0.1}
		}
	}
}

// trace finds where the ink lies along a line, column by column, and builds the hand's timing:
// a column of ink takes four times as long as a blank one, so the hand slows over letters and
// nips across the gaps between words.
func (d *Diary) trace(l *dyLine) {
	n := int(l.w/dyColW) + 2
	l.top, l.bot, l.inked = make([]float32, n), make([]float32, n), make([]bool, n)
	for i := range l.top {
		l.top[i], l.bot[i] = 1e9, -1e9
	}
	for _, w := range l.words {
		f := faceFor(w.s, l.px)
		if f == nil {
			continue
		}
		ix := int(w.x)
		m, ox, oy := f.Mask(w.s, w.x-float64(ix))
		for i := 0; i < m.Rect.Dx(); i++ {
			c := (ix + ox + i - int(l.x)) / dyColW
			if c < 0 || c >= n {
				continue
			}
			for j := 0; j < m.Rect.Dy(); j++ {
				if m.Pix[j*m.Stride+i] < 110 {
					continue
				}
				y := float32(l.y + w.dy + float64(oy+j))
				l.inked[c] = true
				l.top[c] = min(l.top[c], y)
				l.bot[c] = max(l.bot[c], y)
			}
		}
	}
	l.cum = make([]float64, n)
	total := 0.0
	for i := range n {
		l.cum[i] = total
		if l.inked[i] {
			total += 1
		} else {
			total += 0.25
		}
	}
	for i := range l.cum {
		l.cum[i] /= math.Max(1e-9, total)
	}
}

// wrapWords breaks a paragraph into lines no wider than width in the given face.
func wrapWords(s string, px int, width float64, font string) []string {
	UseFont(font)
	defer UseFont("hand")
	var out []string
	cur := ""
	for _, w := range strings.Fields(s) {
		try := w
		if cur != "" {
			try = cur + " " + w
		}
		if measure(try, px) > width && cur != "" {
			out = append(out, cur)
			cur = w
			continue
		}
		cur = try
	}
	if cur != "" {
		out = append(out, cur)
	}
	if len(out) > 0 && measure(out[len(out)-1], px) > width {
		out[len(out)-1] = fit(out[len(out)-1], px, width)
	}
	return out
}

// ---------------------------------------------------------------- entries

func (d *Diary) builders() []func(time.Time) *dyEntry {
	switch d.mode {
	case "quotes":
		return []func(time.Time) *dyEntry{d.eQuote}
	case "nature":
		return []func(time.Time) *dyEntry{d.eWeather, d.eDaylight, d.eMoon, d.eQuote}
	case "papers":
		return []func(time.Time) *dyEntry{d.eBBC, d.eHistory, d.eHN}
	}
	if d.mode == "claude" {
		return []func(time.Time) *dyEntry{d.ePlan, d.eToday, d.eCalls, d.eQuote, d.eNow, d.eTally, d.eWeek}
	}
	return []func(time.Time) *dyEntry{d.eWeather, d.eQuote, d.eBBC, d.ePlan, d.eDaylight, d.eHistory, d.eHN, d.eMoon, d.eQuote, d.eToday}
}

func (d *Diary) usage() (*feeds.ClaudeUsage, bool) {
	u, _, ok, _ := d.live.get()
	return u, ok && u != nil
}

func (d *Diary) ePlan(now time.Time) *dyEntry {
	lim, _, ok, err := d.limits.get("", func(string) (*feeds.ClaudeLimits, error) { return feeds.FetchClaudeLimits() })
	if !ok {
		if err != nil && d.mode == "claude" {
			return &dyEntry{title: "my allowance", lines: []string{"My plan limits are hidden from me today.", err.Error()}}
		}
		return nil
	}
	s, w := lim.Session, lim.Week
	en := &dyEntry{title: "my allowance", big: fmt.Sprintf("%.0f%%", s.Left()), caption: "left this session"}
	l1 := fmt.Sprintf("This session I have %.0f%% left", s.Left())
	if !s.Resets.IsZero() {
		l1 += ", and it renews at " + s.Resets.Local().Format("15:04")
	}
	en.lines = append(en.lines, l1+".")
	if p, ok := pace(s, now); ok && s.Used > 0 {
		switch dlt := s.Used/100 - p; {
		case s.Used >= 100:
			en.lines = append(en.lines, "I have spent it all, and must wait.")
		case dlt > 0.08:
			en.lines = append(en.lines, "I am running rather ahead of myself.")
		case dlt < -0.08:
			en.lines = append(en.lines, "I am pacing myself well.")
		default:
			en.lines = append(en.lines, "I am keeping a steady pace.")
		}
	}
	day := "next week"
	if !w.Resets.IsZero() {
		day = w.Resets.Local().Format("Monday")
	}
	en.lines = append(en.lines, fmt.Sprintf("This week %.0f%% remains, until %s.", w.Left(), day))
	return en
}

func (d *Diary) eToday(now time.Time) *dyEntry {
	u, ok := d.usage()
	if !ok || u.Today.Calls == 0 {
		return nil
	}
	en := &dyEntry{title: "today's work", big: commas(int64(u.Today.Calls)), caption: "replies today"}
	en.lines = append(en.lines, fmt.Sprintf("Today I have written %s replies in %s.", commas(int64(u.Today.Calls)), count(len(u.Projects), "project")))
	if len(u.Projects) > 0 {
		top := u.Projects[0]
		share := float64(top.Out) / math.Max(1, float64(u.Today.Out)) * 100
		s := fmt.Sprintf("Most of my words went to %s (%.0f%%)", top.Name, share)
		if len(u.Projects) > 1 {
			s += ", then " + u.Projects[1].Name
		}
		en.lines = append(en.lines, s+".")
	}
	first, busy := -1, 0
	for h, x := range u.Hours {
		if x.Out > 0 && first < 0 {
			first = h
		}
		if x.Out > u.Hours[busy].Out {
			busy = h
		}
	}
	if first >= 0 {
		en.lines = append(en.lines, fmt.Sprintf("I began at %s, and was busiest at %s.", oclock(first), oclock(busy)))
	}
	if m := modelWords(u.Models); m != "" {
		en.lines = append(en.lines, m)
	}
	return en
}

func (d *Diary) eCalls(now time.Time) *dyEntry {
	var lines []string
	for _, ev := range feeds.Activity.Recent(60) {
		if ev.Kind != "tool" || now.Sub(ev.At) > 6*time.Hour {
			continue
		}
		s := "At " + ev.At.Local().Format("15:04") + " I " + toolPast(ev.Tool, ev.Detail)
		if ev.Project != "" {
			s += " in " + ev.Project
		}
		lines = append(lines, s+".")
	}
	if len(lines) == 0 {
		u, ok := d.usage()
		if !ok || len(u.Recent) == 0 {
			return nil
		}
		for _, c := range u.Recent {
			lines = append(lines, fmt.Sprintf("At %s I wrote %s tokens in %s.", c.At.Local().Format("15:04"), commas(c.Out), c.Project))
		}
	}
	if len(lines) > 3 {
		lines = lines[len(lines)-3:]
	}
	return &dyEntry{title: "lately", lines: lines}
}

func (d *Diary) eNow(now time.Time) *dyEntry {
	u, ok := d.usage()
	if !ok || len(u.Sessions) == 0 {
		return nil
	}
	ss := append([]feeds.ClaudeSession(nil), u.Sessions...)
	sort.SliceStable(ss, func(i, j int) bool { return ss[i].Busy && !ss[j].Busy })
	n := len(ss)
	en := &dyEntry{title: "this moment", big: fmt.Sprint(n), caption: "of me awake"}
	if n == 1 {
		en.lines = append(en.lines, "Just one of me is awake right now.")
	} else {
		en.lines = append(en.lines, fmt.Sprintf("There are %s of me awake right now.", numWord(n)))
	}
	acts := feeds.Activity.Now()
	var fullest feeds.ClaudeSession
	for i, s := range ss {
		if s.Context > fullest.Context {
			fullest = s
		}
		if i >= 2 {
			continue
		}
		where := s.Project
		if where == "" {
			where = s.Name
		}
		doing := "resting"
		if a, ok := acts[s.ID]; ok && now.Sub(a.Since) < 10*time.Minute {
			switch a.State {
			case "tool":
				doing = toolNow(a.Tool, a.Detail)
			case "thinking":
				doing = "thinking"
			}
		} else if s.Busy {
			doing = "hard at work"
		}
		en.lines = append(en.lines, fmt.Sprintf("In %s I am %s.", where, doing))
	}
	if fullest.Context > 0 {
		en.lines = append(en.lines, fmt.Sprintf("My longest conversation is %.0f%% full.", ctxFrac(fullest.Context)*100))
	}
	return en
}

func (d *Diary) eTally(now time.Time) *dyEntry {
	var edits, cmds, reads, finds, helpers, asks int
	y, m, dd := now.Date()
	for _, ev := range feeds.Activity.Recent(400) {
		if ey, em, ed := ev.At.Local().Date(); ey != y || em != m || ed != dd {
			continue
		}
		switch {
		case ev.Kind == "prompt":
			asks++
		case ev.Kind != "tool":
		case ev.Tool == "Edit" || ev.Tool == "Write" || ev.Tool == "MultiEdit" || ev.Tool == "NotebookEdit":
			edits++
		case ev.Tool == "Bash" || ev.Tool == "PowerShell":
			cmds++
		case ev.Tool == "Read":
			reads++
		case ev.Tool == "Grep" || ev.Tool == "Glob" || ev.Tool == "WebSearch" || ev.Tool == "WebFetch":
			finds++
		case ev.Tool == "Agent" || ev.Tool == "Task":
			helpers++
		}
	}
	all := edits + cmds + reads + finds + helpers
	if all == 0 {
		return nil
	}
	en := &dyEntry{title: "a tally", big: fmt.Sprint(all), caption: "things done today"}
	en.lines = append(en.lines, fmt.Sprintf("So far today I have made %s and run %s.", count(edits, "edit"), count(cmds, "command")))
	en.lines = append(en.lines, fmt.Sprintf("I have read %s and searched %s.", count(reads, "file"), times(finds)))
	if helpers > 0 {
		en.lines = append(en.lines, fmt.Sprintf("I sent out %s.", count(helpers, "helper")))
	}
	if asks > 0 {
		en.lines = append(en.lines, fmt.Sprintf("You have asked me %s.", count(asks, "thing")))
	}
	return en
}

func (d *Diary) eWeek(now time.Time) *dyEntry {
	u, ok := d.usage()
	if !ok || len(u.Days) == 0 {
		return nil
	}
	var total int64
	busy := u.Days[0]
	for _, x := range u.Days {
		total += x.Out
		if x.Out > busy.Out {
			busy = x
		}
	}
	if total == 0 {
		return nil
	}
	en := &dyEntry{title: "the week", big: tok(total), caption: "tokens this week"}
	en.lines = append(en.lines, fmt.Sprintf("This week I have written %s tokens.", tok(total)))
	if busy.Out > 0 {
		day := busy.Day.Format("Monday")
		if busy.Day.YearDay() == now.YearDay() {
			day = "Today"
		}
		en.lines = append(en.lines, day+" was my busiest day.")
	}
	if n := len(u.Days); n >= 2 && u.Days[n-2].Out > 0 {
		r := float64(u.Days[n-1].Out)/float64(u.Days[n-2].Out)*100 - 100
		switch {
		case r > 5:
			en.lines = append(en.lines, fmt.Sprintf("Today is %.0f%% busier than yesterday.", r))
		case r < -5:
			en.lines = append(en.lines, fmt.Sprintf("Today is %.0f%% quieter than yesterday.", -r))
		default:
			en.lines = append(en.lines, "Today is much like yesterday.")
		}
	}
	if h := u.Today.CacheHit(); h > 0 {
		en.lines = append(en.lines, fmt.Sprintf("I remembered %.0f%% of what I read.", h*100))
	}
	return en
}

// ---- the day book

func (d *Diary) eBBC(now time.Time) *dyEntry {
	hs, _, ok, _ := d.bbc.get("", func(string) ([]feeds.Headline, error) {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		return feeds.FetchRSS(ctx, "https://feeds.bbci.co.uk/news/rss.xml", 9)
	})
	return headlinesEntry("the news", hs, ok, int(dyTurn[d.mode].Load()))
}

func (d *Diary) eHN(now time.Time) *dyEntry {
	hs, _, ok, _ := d.hn.get("", func(string) ([]feeds.Headline, error) {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		return feeds.FetchHN(ctx, 9)
	})
	return headlinesEntry("on Hacker News", hs, ok, int(dyTurn[d.mode].Load()))
}

func headlinesEntry(title string, hs []feeds.Headline, ok bool, turn int) *dyEntry {
	if !ok || len(hs) == 0 {
		return nil
	}
	en := &dyEntry{title: title}
	off := (turn / 7 * 2) % len(hs) // move on through the list each time round
	for i := 0; i < min(2, len(hs)); i++ {
		t := strings.TrimSpace(hs[(off+i)%len(hs)].Title)
		if !strings.HasSuffix(t, ".") && !strings.HasSuffix(t, "?") && !strings.HasSuffix(t, "!") {
			t += "."
		}
		en.lines = append(en.lines, t)
	}
	return en
}

func (d *Diary) eHistory(now time.Time) *dyEntry {
	day := now.Format("01-02")
	evs, _, ok, _ := d.hist.get(day, func(string) ([]feeds.Event, error) { return feeds.FetchOnThisDay(now.Month(), now.Day()) })
	if !ok || len(evs) == 0 {
		return nil
	}
	ev := evs[(int(dyTurn[d.mode].Load())/7*5)%len(evs)]
	return &dyEntry{title: "on this day", big: fmt.Sprint(ev.Year), caption: fmt.Sprintf("%d years ago", now.Year()-ev.Year),
		lines: []string{fmt.Sprintf("On this day in %d, %s.", ev.Year, lowerFirst(strings.TrimSuffix(ev.Text, ".")))}}
}

func (d *Diary) eWeather(now time.Time) *dyEntry {
	place := d.place()
	if place == "" {
		return nil
	}
	f, _, ok, _ := d.wx.get(place, func(p string) (*feeds.Forecast, error) { return feeds.FetchForecast(p) })
	if !ok || f == nil {
		return nil
	}
	sky := strings.ToLower(feeds.Describe(f.Code))
	where := f.Place
	if i := strings.Index(where, ","); i > 0 {
		where = where[:i]
	}
	en := &dyEntry{title: "the weather", big: fmt.Sprintf("%.0f°", f.Temp), caption: sky}
	s := fmt.Sprintf("Outside it is %.0f degrees", f.Temp)
	if where != "" {
		s = fmt.Sprintf("In %s it is %.0f degrees", where, f.Temp)
	}
	if sky != "" {
		s += " and " + sky
	}
	en.lines = append(en.lines, s+".")
	if len(f.Days) > 0 {
		dd := f.Days[0]
		en.lines = append(en.lines, fmt.Sprintf("The high today is %.0f, the low %.0f.", dd.Hi, dd.Lo))
	}
	rain := 0
	for i, h := range f.Hours {
		if i < 12 && h.Rain > rain {
			rain = h.Rain
		}
	}
	switch {
	case rain >= 60:
		en.lines = append(en.lines, fmt.Sprintf("Rain is likely (%d%%); take an umbrella.", rain))
	case rain >= 25:
		en.lines = append(en.lines, fmt.Sprintf("There may be a shower (%d%%).", rain))
	default:
		en.lines = append(en.lines, "It should stay dry for a while.")
	}
	if f.Sunset != "" {
		en.lines = append(en.lines, "The sun sets at "+f.Sunset+".")
	}
	return en
}

func (d *Diary) eMoon(now time.Time) *dyEntry {
	age := MoonAge(now)
	lit := MoonIllumination(age) * 100
	en := &dyEntry{title: "the moon", big: fmt.Sprintf("%.0f%%", lit), caption: "of the moon lit"}
	phase := strings.ToLower(MoonPhase(age))
	switch {
	case strings.Contains(phase, "quarter"):
		phase = "at its " + phase
	case strings.HasSuffix(phase, " moon"):
		phase = strings.TrimSuffix(phase, " moon")
	default:
		phase = "a " + phase
	}
	en.lines = append(en.lines, fmt.Sprintf("Tonight the moon is %s, %.0f%% lit.", phase, lit))
	toFull := math.Mod(synodic/2-age+synodic, synodic)
	toNew := synodic - age
	if toFull < toNew {
		en.lines = append(en.lines, fmt.Sprintf("It will be full in %s.", days(toFull)))
	} else {
		en.lines = append(en.lines, fmt.Sprintf("The next new moon is in %s.", days(toNew)))
	}
	return en
}

// ---- words

func count(n int, thing string) string {
	if n == 1 {
		return "one " + thing
	}
	return numWord(n) + " " + thing + "s"
}

func times(n int) string {
	switch n {
	case 0:
		return "not at all"
	case 1:
		return "once"
	case 2:
		return "twice"
	}
	return numWord(n) + " times"
}

func numWord(n int) string {
	w := []string{"no", "one", "two", "three", "four", "five", "six", "seven", "eight", "nine", "ten", "eleven", "twelve"}
	if n >= 0 && n < len(w) {
		return w[n]
	}
	return commas(int64(n))
}

func days(d float64) string {
	switch n := int(math.Round(d)); n {
	case 0:
		return "a few hours"
	case 1:
		return "a day"
	default:
		return numWord(n) + " days"
	}
}

func oclock(h int) string {
	switch h {
	case 0:
		return "midnight"
	case 12:
		return "noon"
	}
	s := fmt.Sprint(h % 12)
	switch {
	case h < 12:
		return s + " in the morning"
	case h < 18:
		return s + " in the afternoon"
	}
	return s + " in the evening"
}

func lowerFirst(s string) string {
	r := []rune(s)
	if len(r) > 1 && unicode.IsUpper(r[0]) && !unicode.IsUpper(r[1]) {
		r[0] = unicode.ToLower(r[0])
	}
	return string(r)
}

func modelWords(ms []feeds.Named) string {
	var names []string
	seen := map[string]bool{}
	for _, m := range ms {
		n := ""
		for _, k := range []string{"opus", "sonnet", "haiku", "fable"} {
			if strings.Contains(strings.ToLower(m.Name), k) {
				n = strings.ToUpper(k[:1]) + k[1:]
			}
		}
		if n != "" && !seen[n] && m.Out > 0 {
			seen[n] = true
			names = append(names, n)
		}
	}
	switch len(names) {
	case 0:
		return ""
	case 1:
		return "I thought as " + names[0] + " all day."
	}
	return "Mostly as " + names[0] + ", with a little " + names[1] + "."
}

// short trims a hook detail for the page.
func short(s string) string {
	s = strings.TrimSpace(strings.ReplaceAll(s, "\n", " "))
	if r := []rune(s); len(r) > 44 {
		s = string(r[:43]) + "…"
	}
	return s
}

// sentenceish is a Bash description like "Run the tests" rather than a raw command.
func sentenceish(s string) bool {
	r := []rune(s)
	return len(r) > 2 && unicode.IsUpper(r[0]) && strings.Contains(s, " ") && !strings.ContainsAny(s, "|&;$=/")
}

func toolPast(tool, detail string) string {
	d := short(detail)
	switch tool {
	case "Bash", "PowerShell":
		if sentenceish(d) {
			return "went to " + lowerFirst(d)
		}
		if d != "" {
			return "ran " + d
		}
		return "ran a command"
	case "Edit", "MultiEdit", "NotebookEdit":
		return "edited " + or(d, "a file")
	case "Write":
		return "wrote " + or(d, "a file")
	case "Read":
		return "read " + or(d, "a file")
	case "Grep", "Glob":
		return "searched for " + or(d, "something")
	case "WebSearch":
		return "searched the web for " + or(d, "something")
	case "WebFetch":
		return "read a page online"
	case "Agent", "Task":
		return "sent a helper to " + lowerFirst(or(d, "look into things"))
	case "TodoWrite":
		return "updated my list"
	case "Skill":
		return "took up the " + or(d, "a") + " skill"
	}
	return "used " + tool
}

func toolNow(tool, detail string) string {
	d := short(detail)
	switch tool {
	case "Bash", "PowerShell":
		if sentenceish(d) {
			return "trying to " + lowerFirst(d)
		}
		return "running a command"
	case "Edit", "MultiEdit", "NotebookEdit", "Write":
		return "writing " + or(d, "a file")
	case "Read":
		return "reading " + or(d, "a file")
	case "Grep", "Glob", "WebSearch":
		return "searching for " + or(d, "something")
	case "WebFetch":
		return "reading the web"
	case "Agent", "Task":
		return "waiting on a helper"
	}
	return "using " + tool
}

func or(s, d string) string {
	if s == "" {
		return d
	}
	return s
}
