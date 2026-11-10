package engine

import (
	"image"
	_ "image/jpeg"
	_ "image/png"
	"math"
	"time"

	xdraw "golang.org/x/image/draw"
	_ "golang.org/x/image/webp"

	"github.com/justynroberts/marquee/internal/board"
	"github.com/justynroberts/marquee/internal/dashboard"
	"github.com/justynroberts/marquee/internal/emoji"
	"github.com/justynroberts/marquee/internal/led"
	"github.com/justynroberts/marquee/internal/nowplaying"
	"github.com/justynroberts/marquee/internal/scenes"
)

// Program produces frames. Frame returns nil when the program has finished.
type Program interface {
	Name() string
	// Frame renders the frame for time t (seconds since start) and dt (since last frame).
	Frame(t, dt float64) *image.RGBA
	// Interval is the minimum time between frames (0 = as fast as the panel takes them).
	Interval() time.Duration
}

// ---------------------------------------------------------------- scenes and the board

type sceneProgram struct {
	name    string
	scene   scenes.Scene
	seconds float64
}

func (p *sceneProgram) Name() string            { return p.name }
func (p *sceneProgram) Interval() time.Duration { return 0 }
func (p *sceneProgram) Frame(t, dt float64) *image.RGBA {
	if p.seconds > 0 && t > p.seconds {
		return nil
	}
	g := led.NewGrid()
	p.scene.Draw(g, t, dt)
	return g.Image(led.Ground)
}

type boardProgram struct {
	b      *board.Board
	passes int
	start  int
}

func newBoardProgram(path string, passes int) *boardProgram {
	b := board.New(path)
	return &boardProgram{b: b, passes: passes, start: b.Passes()}
}

func (p *boardProgram) Name() string            { return "board" }
func (p *boardProgram) Interval() time.Duration { return 0 }
func (p *boardProgram) Frame(t, dt float64) *image.RGBA {
	if p.passes > 0 && p.b.Passes()-p.start >= p.passes {
		return nil
	}
	g := led.NewGrid()
	p.b.Draw(g, t, dt)
	return g.Image(p.b.Ground())
}

// ---------------------------------------------------------------- native-resolution dashboard

type dashProgram struct {
	d       *dashboard.Dashboard
	seconds float64
}

func (p *dashProgram) Name() string            { return "dashboard" }
func (p *dashProgram) Interval() time.Duration { return 500 * time.Millisecond }
func (p *dashProgram) Frame(t, dt float64) *image.RGBA {
	if p.seconds > 0 && t > p.seconds {
		return nil
	}
	return p.d.Render()
}

type npProgram struct {
	v       *nowplaying.View
	seconds float64
	karaoke bool
}

func (p *npProgram) Name() string {
	if p.karaoke {
		return "karaoke"
	}
	return "nowplaying"
}
func (p *npProgram) Interval() time.Duration { return 0 }
func (p *npProgram) Frame(t, dt float64) *image.RGBA {
	if p.seconds > 0 && t > p.seconds {
		return nil
	}
	if p.karaoke {
		return p.v.Karaoke(time.Now())
	}
	return p.v.Frame(time.Now())
}

// fullRes runs a native-resolution visual from the dashboard package.
type fullRes struct {
	name string
	v    interface {
		Frame(t, dt float64, now time.Time) *image.RGBA
	}
	seconds float64
}

func (p *fullRes) Name() string            { return p.name }
func (p *fullRes) Interval() time.Duration { return 0 }
func (p *fullRes) Frame(t, dt float64) *image.RGBA {
	if p.seconds > 0 && t > p.seconds {
		return nil
	}
	return p.v.Frame(t, dt, time.Now())
}

type interferenceProgram struct {
	v       *dashboard.Interference
	seconds float64
}

func (p *interferenceProgram) Name() string            { return "interference" }
func (p *interferenceProgram) Interval() time.Duration { return 0 }
func (p *interferenceProgram) Frame(t, dt float64) *image.RGBA {
	if p.seconds > 0 && t > p.seconds {
		return nil
	}
	return p.v.Frame(t, dt, time.Now())
}

// ---------------------------------------------------------------- playlists

type playlistProgram struct {
	name   string
	items  []string
	per    float64
	e      *Engine
	i      int
	cur    Program
	curT0  float64
	lastDt float64
}

func (p *playlistProgram) Name() string            { return p.name }
func (p *playlistProgram) Interval() time.Duration { return 0 }
func (p *playlistProgram) Frame(t, dt float64) *image.RGBA {
	for tries := 0; tries <= len(p.items); tries++ {
		if p.cur == nil {
			item := p.items[p.i%len(p.items)]
			p.i++
			var err error
			p.cur, err = p.e.makeItem(item, p.per)
			if err != nil {
				continue
			}
			p.curT0 = t
		}
		if img := p.cur.Frame(t-p.curT0, dt); img != nil {
			return img
		}
		p.cur = nil
	}
	return nil
}

// ---------------------------------------------------------------- media

type mediaProgram struct {
	name   string
	frames []*image.RGBA
	delays []float64
	total  float64
}

func (m *mediaProgram) Name() string { return m.name }
func (m *mediaProgram) Interval() time.Duration {
	if len(m.frames) == 1 {
		return 2 * time.Second
	}
	return 0
}
func (m *mediaProgram) Frame(t, dt float64) *image.RGBA {
	if len(m.frames) == 1 {
		return m.frames[0]
	}
	pos := math.Mod(t, m.total)
	for i, d := range m.delays {
		if pos < d {
			return m.frames[i]
		}
		pos -= d
	}
	return m.frames[len(m.frames)-1]
}

// ---------------------------------------------------------------- external frames (pushed by apps)

type pushProgram struct {
	img *image.RGBA
}

func (p *pushProgram) Name() string            { return "push" }
func (p *pushProgram) Interval() time.Duration { return 2 * time.Second }
func (p *pushProgram) Frame(t, dt float64) *image.RGBA {
	return p.img
}

// ---------------------------------------------------------------- drawing pad

type drawProgram struct {
	cols [led.Rows][led.Cols]led.RGB
}

func (p *drawProgram) Name() string            { return "draw" }
func (p *drawProgram) Interval() time.Duration { return 100 * time.Millisecond }
func (p *drawProgram) Frame(t, dt float64) *image.RGBA {
	g := led.NewGrid()
	g.FromColours(&p.cols, 12)
	return g.Image(led.Ground)
}

// SetImage loads a canvas image (any size; scaled to the LED grid with nearest neighbour).
func (p *drawProgram) SetImage(src image.Image) {
	small := image.NewRGBA(image.Rect(0, 0, led.Cols, led.Rows))
	xdraw.NearestNeighbor.Scale(small, small.Bounds(), src, src.Bounds(), xdraw.Src, nil)
	for y := 0; y < led.Rows; y++ {
		for x := 0; x < led.Cols; x++ {
			o := y*small.Stride + x*4
			p.cols[y][x] = led.RGB{R: small.Pix[o], G: small.Pix[o+1], B: small.Pix[o+2]}
		}
	}
}

// ---------------------------------------------------------------- overlays: flash and timer

type flashProgram struct {
	text    string
	mask    *led.Mask
	colour  led.RGB
	emoji   []flashEmoji
	seconds float64
	scroll  bool
}

type flashEmoji struct {
	x, y int
	s    *emoji.Sprite
}

// flashStrip lays out text with colour emoji at one font size.
func flashStrip(text string, size int) (*led.Mask, []flashEmoji) {
	type part struct {
		m *led.Mask
		e *emoji.Sprite
	}
	var parts []part
	w := 0
	eh := size * 3 / 4
	for _, r := range emoji.Split(text) {
		if r.Emoji {
			if sp, ok := emoji.Render(r.Text, eh); ok {
				parts = append(parts, part{e: sp})
				w += sp.W + 4
			}
			continue
		}
		m := led.Text(r.Text, "sans", size)
		parts = append(parts, part{m: m})
		w += m.W
	}
	out := led.NewMask(max(w, 1), led.Rows)
	var em []flashEmoji
	x := 0
	for _, p := range parts {
		if p.e != nil {
			em = append(em, flashEmoji{x: x + 2, y: (led.Rows - p.e.H) / 2, s: p.e})
			x += p.e.W + 4
			continue
		}
		for yy := 0; yy < p.m.H && yy < led.Rows; yy++ {
			for xx := 0; xx < p.m.W; xx++ {
				if p.m.At(xx, yy) {
					out.SetBit(x+xx, yy, true)
				}
			}
		}
		x += p.m.W
	}
	return out, em
}

func newFlash(text string, seconds float64, colour led.RGB) *flashProgram {
	f := newFlashStrip(text, seconds, colour)
	f.text = text
	return f
}

func newFlashStrip(text string, seconds float64, colour led.RGB) *flashProgram {
	var m *led.Mask
	var em []flashEmoji
	for _, size := range []int{58, 50, 44, 38} {
		m, em = flashStrip(text, size)
		if m.W <= led.Cols-16 {
			return &flashProgram{mask: m, emoji: em, colour: colour, seconds: seconds}
		}
	}
	// Too long at any readable size: glide across at a large size.
	m, em = flashStrip(text, 50)
	return &flashProgram{mask: m, emoji: em, colour: colour, seconds: seconds, scroll: true}
}

func (f *flashProgram) Name() string            { return "flash" }
func (f *flashProgram) Interval() time.Duration { return 0 }
func (f *flashProgram) Frame(t, dt float64) *image.RGBA {
	if t > f.seconds {
		return nil
	}
	g := led.NewGrid()
	x := (led.Cols - f.mask.W) / 2
	if f.scroll {
		// Start with the beginning visible, glide until the end is visible, hold, glide back.
		overflow := float64(f.mask.W - (led.Cols - 16))
		glide := math.Max(1.5, overflow/40)
		hold := 1.0
		phase := math.Mod(t, 2*(glide+hold))
		var off float64
		switch {
		case phase < hold:
			off = 0
		case phase < hold+glide:
			off = (phase - hold) / glide * overflow
		case phase < 2*hold+glide:
			off = overflow
		default:
			off = overflow * (1 - (phase-2*hold-glide)/glide)
		}
		x = 8 - int(off)
	}
	g.Stamp(f.mask, x, 0, f.colour)
	for _, e := range f.emoji {
		for yy := 0; yy < e.s.H; yy++ {
			for xx := 0; xx < e.s.W; xx++ {
				if i := yy*e.s.W + xx; e.s.Lit[i] {
					c := e.s.Col[i]
					g.Set(x+e.x+xx, e.y+yy, led.RGB{R: c.R, G: c.G, B: c.B})
				}
			}
		}
	}
	k := 0.3 + 0.7*(0.5+0.5*math.Sin(t*8))
	border := f.colour.Scale(k)
	g.Fill(0, 0, led.Cols, 2, border)
	g.Fill(0, led.Rows-2, led.Cols, led.Rows, border)
	g.Fill(0, 0, 2, led.Rows, border)
	g.Fill(led.Cols-2, 0, led.Cols, led.Rows, border)
	return g.Image(led.Ground)
}

type timerProgram struct {
	c *scenes.Countdown
}

func (p *timerProgram) Name() string            { return "timer" }
func (p *timerProgram) Interval() time.Duration { return 0 }
func (p *timerProgram) Frame(t, dt float64) *image.RGBA {
	if p.c.Remaining() < -8 {
		return nil
	}
	g := led.NewGrid()
	p.c.Draw(g, t, dt)
	return g.Image(led.Ground)
}

func baseName(p string) string {
	for i := len(p) - 1; i >= 0; i-- {
		if p[i] == '/' || p[i] == '\\' {
			return p[i+1:]
		}
	}
	return p
}
