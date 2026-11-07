// Package board is the scrolling LED message board: messages.txt drawn as a dot-matrix sign
// with colour, background, font and size tags, animated effects, pixel-art sprites, colour emoji,
// live tokens and board-wide @settings.
//
// The message strip is laid out once at LED resolution (lit, colour, unlit colour and effect per
// LED); each frame takes a 275-column window of it, applies the effects and sprites for the
// current time, and writes the window into the grid.
package board

import (
	"math"
	"os"
	"sync"
	"time"

	"github.com/justynroberts/marquee/internal/emoji"
	"github.com/justynroberts/marquee/internal/led"
	"github.com/shirou/gopsutil/v4/cpu"
)

const emojiGap = 6 // LEDs around emoji and sprites

// column is one LED column of the strip.
type column struct {
	lit  [led.Rows]bool
	col  [led.Rows]led.RGB
	back [led.Rows]led.RGB
	fx   [led.Rows]uint8
}

type placedSprite struct {
	x     int
	name  string
	style Style
}

type strip struct {
	cols     []column
	sprites  []placedSprite
	settings Settings
}

// Board implements scenes.Scene.
type Board struct {
	path string

	mu       sync.Mutex
	strip    *strip
	offset   int
	passes   int
	mtime    time.Time
	minute   string
	checked  time.Time
	building bool
}

// New returns a board for messages.txt at path, creating it with DefaultMessages if missing.
func New(path string) *Board {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		_ = os.WriteFile(path, []byte(DefaultMessages), 0o644)
	}
	_, _ = cpu.Percent(0, false) // prime: the first {cpu} would otherwise read 0 (board.py does the same)
	b := &Board{path: path}
	text, mtime := b.read()
	b.strip = build(text)
	b.mtime = mtime
	b.minute = time.Now().Format("15:04")
	b.checked = time.Now()
	return b
}

func (b *Board) read() (string, time.Time) {
	st, err := os.Stat(b.path)
	if err != nil {
		return "", time.Time{}
	}
	data, err := os.ReadFile(b.path)
	if err != nil {
		return "", st.ModTime()
	}
	return string(data), st.ModTime()
}

// refresh rebuilds the strip in the background when the file changed or a new minute began.
// Checked at most once a second; frames keep using the old strip until the new one is ready.
func (b *Board) refresh() {
	now := time.Now()
	b.mu.Lock()
	if now.Sub(b.checked) < time.Second || b.building {
		b.mu.Unlock()
		return
	}
	b.checked = now
	st, err := os.Stat(b.path)
	var mtime time.Time
	if err == nil {
		mtime = st.ModTime()
	}
	minute := now.Format("15:04")
	if mtime.Equal(b.mtime) && minute == b.minute {
		b.mu.Unlock()
		return
	}
	b.mtime, b.minute = mtime, minute
	b.building = true
	b.mu.Unlock()
	go func() {
		text, _ := b.read()
		s := build(text)
		b.mu.Lock()
		b.strip = s
		if n := len(s.cols); n > 0 {
			b.offset %= n
		}
		b.building = false
		b.mu.Unlock()
	}()
}

// Passes is the number of completed full scrolls since New.
func (b *Board) Passes() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.passes
}

// Ground is the colour between LEDs (@bg) to pass to Grid.Image.
func (b *Board) Ground() led.RGB {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.strip.settings.Bg
}

// Settings returns the current board settings.
func (b *Board) Settings() Settings {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.strip.settings
}

// Draw renders the current window into g and advances the scroll.
func (b *Board) Draw(g *led.Grid, t, dt float64) {
	b.refresh()
	b.mu.Lock()
	s := b.strip
	offset := b.offset
	n := len(s.cols)
	b.offset += s.settings.Step
	if b.offset >= n {
		b.offset %= n
		b.passes++
	}
	b.mu.Unlock()
	render(g, s, offset, t)
}

// ---------------------------------------------------------------------------
// Building the strip
// ---------------------------------------------------------------------------

type piece struct {
	kind   string // text | emoji | sprite | gap
	mask   *led.Mask
	sprite *emoji.Sprite
	name   string
	w      int
	style  *Style
}

func build(text string) *strip {
	settings, lines := parseFile(text)
	setLocation(settings.Location)
	var pieces []piece
	for _, line := range lines {
		for _, seg := range parseMessage(line, settings) {
			st := seg.Style
			switch seg.Kind {
			case "text":
				m := led.Text(seg.Value, st.Font, st.Size)
				pieces = append(pieces, piece{kind: "text", mask: m, w: m.W, style: &st})
			case "emoji":
				if sp, ok := emoji.Render(seg.Value, max(8, min(st.Size, led.Rows))); ok {
					pieces = append(pieces, piece{kind: "emoji", sprite: sp, w: sp.W + 2*emojiGap, style: &st})
				} else {
					m := led.Text("?", st.Font, st.Size)
					pieces = append(pieces, piece{kind: "text", mask: m, w: m.W, style: &st})
				}
			case "sprite":
				pieces = append(pieces, piece{kind: "sprite", name: seg.Value, w: spriteW + 2*emojiGap, style: &st})
			}
		}
		pieces = append(pieces, piece{kind: "gap", w: settings.Gap})
	}
	total := led.Cols
	for _, p := range pieces {
		total += p.w
	}
	s := &strip{cols: make([]column, total), settings: settings}
	for i := range s.cols {
		for y := range led.Rows {
			s.cols[i].back[y] = settings.Dim
		}
	}
	x := led.Cols
	for _, p := range pieces {
		if p.style != nil && p.style.HasBg {
			for cx := x; cx < x+p.w; cx++ {
				for y := range led.Rows {
					s.cols[cx].back[y] = p.style.Bg
				}
			}
		}
		switch p.kind {
		case "text":
			for mx := 0; mx < p.w; mx++ {
				c := &s.cols[x+mx]
				for y := 0; y < led.Rows && y < p.mask.H; y++ {
					if p.mask.At(mx, y) {
						c.lit[y] = true
					}
					c.col[y] = p.style.Colour
					c.fx[y] = p.style.Effects
				}
			}
		case "emoji":
			sp := p.sprite
			top := (led.Rows - sp.H) / 2
			ex := x + emojiGap
			fx := p.style.Effects &^ FxRainbow
			for sy := 0; sy < sp.H; sy++ {
				y := top + sy
				if y < 0 || y >= led.Rows {
					continue
				}
				for sx := 0; sx < sp.W; sx++ {
					i := sy*sp.W + sx
					if !sp.Lit[i] {
						continue
					}
					c := &s.cols[ex+sx]
					cc := sp.Col[i]
					c.lit[y] = true
					c.col[y] = led.RGB{R: cc.R, G: cc.G, B: cc.B}
					c.fx[y] = fx
				}
			}
		case "sprite":
			s.sprites = append(s.sprites, placedSprite{x: x + emojiGap, name: p.name, style: *p.style})
		}
		x += p.w
	}
	return s
}

// ---------------------------------------------------------------------------
// Frames
// ---------------------------------------------------------------------------

func rainbow(x, t float64) led.RGB {
	h := math.Mod(x*0.012+t*0.35, 1)
	if h < 0 {
		h++
	}
	i := int(math.Floor(h*6)) % 6
	f := h*6 - math.Floor(h*6)
	q := 1 - f
	var r, g, b float64
	switch i {
	case 0:
		r, g, b = 1, f, 0
	case 1:
		r, g, b = q, 1, 0
	case 2:
		r, g, b = 0, 1, f
	case 3:
		r, g, b = 0, q, 1
	case 4:
		r, g, b = f, 0, 1
	default:
		r, g, b = 1, 0, q
	}
	return led.RGB{R: uint8(r*235 + 20), G: uint8(g*235 + 20), B: uint8(b*235 + 20)}
}

func waveDY(x int, t float64) int {
	return int(math.Round(3 * math.Sin(float64(x)*0.09-t*6.0)))
}

func roll(dy int) func(y int) int {
	// np.roll(a, dy): out[(y+dy) mod R] = a[y], so out[y] = a[(y-dy) mod R]
	return func(y int) int { return ((y-dy)%led.Rows + led.Rows) % led.Rows }
}

var phosphorTints = map[string]led.RGB{"green": {R: 70, G: 255, B: 120}, "amber": {R: 255, G: 176, B: 40}}

func render(g *led.Grid, s *strip, offset int, t float64) {
	n := len(s.cols)
	blinkOff := int(t*2.5)%2 == 1
	pulseK := 0.45 + 0.55*(0.5+0.5*math.Sin(t*5.0))
	idx := make([]int, led.Cols)
	for cx := range led.Cols {
		i := (offset + cx) % n
		idx[cx] = i
		c := &s.cols[i]
		var any uint8
		for y := range led.Rows {
			any |= c.fx[y]
		}
		src := func(y int) int { return y }
		if any&FxWave != 0 {
			src = roll(waveDY(i, t))
		}
		var shimmerK float64
		if any&FxShimmer != 0 {
			shimmerK = 0.55 + 0.45*math.Sin(float64(i)*0.35-t*9.0)
		}
		var rb led.RGB
		if any&FxRainbow != 0 {
			rb = rainbow(float64(i), t)
		}
		for y := range led.Rows {
			sy := src(y)
			lit := c.lit[sy]
			col := c.col[sy]
			fx := c.fx[sy]
			if fx&FxRainbow != 0 {
				col = rb
			}
			if fx&FxShimmer != 0 {
				col = col.Scale(shimmerK)
			}
			if fx&FxPulse != 0 {
				col = col.Scale(pulseK)
			}
			if fx&FxBlink != 0 && blinkOff {
				lit = false
			}
			g.Lit[y][cx] = lit
			g.Col[y][cx] = col
			g.Back[y][cx] = c.back[y]
		}
	}
	// Sprites: paint the current frame wherever one overlaps the window.
	for _, sp := range s.sprites {
		rel := ((sp.x-offset)%n + n) % n
		if rel >= led.Cols && rel+spriteW < n {
			continue
		}
		fr := spriteFrameAt(sp.name, t)
		bob := 0
		if spriteSpecs[sp.name].bob {
			bob = int(math.Round(3 * math.Sin(t*3.0)))
		}
		hidden := sp.style.Effects&FxBlink != 0 && blinkOff
		top := (led.Rows-textRows)/2 + bob
		for dx := range spriteW {
			cx := (rel + dx) % n
			if cx >= led.Cols {
				continue
			}
			dy := 0
			if sp.style.Effects&FxWave != 0 {
				dy = waveDY(idx[cx], t)
			}
			for sy := range spriteH {
				y := top + sy
				if y < 0 || y >= led.Rows {
					continue
				}
				fy := ((sy-dy)%spriteH + spriteH) % spriteH
				g.Lit[y][cx] = fr.lit[fy][dx] && !hidden
				g.Col[y][cx] = fr.col[fy][dx]
			}
		}
	}
	if tint, ok := phosphorTints[s.settings.Theme]; ok {
		phosphor(g, tint)
	}
}

// phosphor turns the frame into a monochrome CRT: brightness kept, colour replaced by the tint,
// with a soft glow around lit LEDs and dimmer alternate rows.
func phosphor(g *led.Grid, tint led.RGB) {
	var lum [led.Rows][led.Cols]float64
	for y := range led.Rows {
		for x := range led.Cols {
			if g.Lit[y][x] {
				lum[y][x] = 0.35 + 0.65*float64(g.Col[y][x].Luma())/255
			}
		}
	}
	base := led.RGB{R: 28, G: 20, B: 10}
	if tint.G > tint.R {
		base = led.RGB{R: 16, G: 26, B: 18}
	}
	for y := range led.Rows {
		scan := 1.0
		if y%2 == 1 {
			scan = 0.85
		}
		for x := range led.Cols {
			var glow float64
			for dy := -1; dy <= 1; dy++ {
				for dx := -1; dx <= 1; dx++ {
					glow += lum[(y+dy+led.Rows)%led.Rows][(x+dx+led.Cols)%led.Cols]
				}
			}
			level := math.Max(lum[y][x], glow/9*0.3) * scan
			c := base.Lerp(tint, level)
			g.Col[y][x] = c
			g.Back[y][x] = c
		}
	}
}
