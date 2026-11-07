package scenes

import (
	"fmt"
	"image"
	"math/rand/v2"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/shirou/gopsutil/v4/host"
	"github.com/shirou/gopsutil/v4/load"
	"github.com/shirou/gopsutil/v4/mem"
	"golang.org/x/image/font"
	"golang.org/x/image/math/fixed"

	"github.com/justynroberts/marquee/internal/board"
	"github.com/justynroberts/marquee/internal/led"
)

func init() {
	Register("matrix", "scene", "Digital rain: mirrored katakana falling in phosphor green", func() Scene { return newMatrix() })
	Register("terminal", "scene", "A green-phosphor terminal typing live system information", func() Scene { return newTerminal() })
}

// ---------------------------------------------------------------------------
// matrix

const (
	mCW = 8  // LED columns per glyph
	mCH = 10 // LED rows per glyph
)

type matrix struct {
	glyphs              []*led.Mask // mCW x mCH, mirrored
	ncols, nrows        int
	head, speed, length []float64
	chars               [][]int
}

func cjkAvailable() bool {
	for _, p := range []string{"/Library/Fonts/Arial Unicode.ttf", "/System/Library/Fonts/Supplemental/Arial Unicode.ttf"} {
		if _, err := os.Stat(p); err == nil {
			return true
		}
	}
	return false
}

var matrixGlyphCache []*led.Mask

// matrixGlyphs renders the glyph set exactly as scenes.py does: half-width katakana plus
// "012345789Z:.=*+-<>¦|", drawn at 11 px with the ascender at y=-1 into an 8x10 cell,
// thresholded at 90 and mirrored left-right. Without Arial Unicode it falls back to Menlo
// Bold, as Python does.
func matrixGlyphs() []*led.Mask {
	if matrixGlyphCache != nil {
		return matrixGlyphCache
	}
	var set []rune
	for r := rune(0xFF66); r < 0xFF9E; r++ {
		set = append(set, r)
	}
	set = append(set, []rune("012345789Z:.=*+-<>¦|")...)
	fontName := "cjk"
	if !cjkAvailable() {
		fontName = "mono"
	}
	face := led.Face(fontName, 11)
	ascent := face.Metrics().Ascent.Ceil()
	for _, r := range set {
		img := image.NewAlpha(image.Rect(0, 0, mCW, mCH))
		d := &font.Drawer{Dst: img, Src: image.Opaque, Face: face, Dot: fixed.P(0, -1+ascent)}
		d.DrawString(string(r))
		m := led.NewMask(mCW, mCH)
		for y := 0; y < mCH; y++ {
			for x := 0; x < mCW; x++ {
				if img.Pix[y*img.Stride+x] > 90 {
					m.SetBit(mCW-1-x, y, true)
				}
			}
		}
		matrixGlyphCache = append(matrixGlyphCache, m)
	}
	return matrixGlyphCache
}

func newMatrix() *matrix {
	m := &matrix{glyphs: matrixGlyphs(), ncols: cols/mCW + 1, nrows: rows/mCH + 2}
	m.head = make([]float64, m.ncols)
	m.speed = make([]float64, m.ncols)
	m.length = make([]float64, m.ncols)
	for c := range m.ncols {
		m.head[c] = uniform(-float64(m.nrows)*2, float64(m.nrows))
		m.speed[c] = uniform(4, 11)
		m.length[c] = uniform(4, 12)
	}
	m.chars = make([][]int, m.nrows)
	for r := range m.nrows {
		m.chars[r] = make([]int, m.ncols)
		for c := range m.ncols {
			m.chars[r][c] = rand.IntN(len(m.glyphs))
		}
	}
	return m
}

func (m *matrix) Draw(g *led.Grid, t, dt float64) {
	white := fcol{225, 255, 230}
	greenc := fcol{60, 255, 110}
	for r := range m.nrows {
		for c := range m.ncols {
			if rand.Float64() < 0.06 {
				m.chars[r][c] = rand.IntN(len(m.glyphs))
			}
		}
	}
	for c := 0; c < m.ncols; c++ {
		m.head[c] += m.speed[c] * dt
		x0 := c * mCW
		if x0 >= cols {
			break
		}
		head := m.head[c]
		for r := 0; r < m.nrows; r++ {
			d := head - float64(r)
			if d < 0 || d > m.length[c] {
				continue
			}
			y0 := r * mCH
			if y0 >= rows {
				continue
			}
			k := 1 - d/m.length[c]
			col := greenc.scale(0.25 + 0.75*pow(k, 1.4))
			if d < 1 {
				col = white
			}
			stampMask(g, m.glyphs[m.chars[r][c]], x0, y0, cols-x0, col.rgb())
		}
		if head-m.length[c] > float64(m.nrows) {
			m.head[c] = uniform(-8, 0)
			m.speed[c] = uniform(4, 11)
			m.length[c] = uniform(4, 12)
		}
	}
}

// ---------------------------------------------------------------------------
// terminal

const (
	termSize  = 11
	termLineH = 13
)

type terminal struct {
	lines    []string
	queue    []string
	typed    string
	nextChar float64
	cache    map[string]*led.Mask
}

func newTerminal() *terminal { return &terminal{cache: map[string]*led.Mask{}} }

func (tm *terminal) script() []string {
	hostname, _ := os.Hostname()
	hostname = strings.ToLower(strings.Split(hostname, ".")[0])
	upLine := "up ?"
	if up, err := host.Uptime(); err == nil {
		upLine = fmt.Sprintf("up %d days %d:%02d", up/86400, up%86400/3600, up%3600/60)
	}
	if l, err := load.Avg(); err == nil {
		upLine += fmt.Sprintf(", load %.2f %.2f %.2f", l.Load1, l.Load5, l.Load15)
	}
	memLine := "mem ?"
	if vm, err := mem.VirtualMemory(); err == nil {
		memLine = fmt.Sprintf("mem %.1fG used  %.1fG free  (%.0f%%)", float64(vm.Used)/(1<<30), float64(vm.Available)/(1<<30), vm.UsedPercent)
	}
	osName := runtime.GOOS
	if info, err := host.Info(); err == nil && info.OS != "" {
		osName = info.OS
	}
	osName = strings.ToUpper(osName[:1]) + osName[1:]
	arch := runtime.GOARCH
	if info, err := host.Info(); err == nil && info.KernelArch != "" {
		arch = info.KernelArch // uname -m, as platform.machine()
	}
	var echoes []string
	for _, m := range terminalMessages() {
		if len(echoes) == 3 {
			break
		}
		if !strings.Contains(m, "[") && !strings.Contains(strings.Trim(m, ":"), ":") {
			echoes = append(echoes, "$ echo "+pyRepr(m))
		}
	}
	out := []string{
		"$ ssh " + hostname,
		"Last login: " + time.Now().Format("Mon Jan 02 15:04:05") + " on ttys001",
		"$ uptime",
		upLine,
		"$ free -h",
		memLine,
		"$ uname -sm",
		osName + " " + arch,
	}
	out = append(out, echoes...)
	return append(out, "$ ./wake-up --follow-the-white-rabbit", "knock knock.")
}

// TerminalMessagesFile is the board's messages.txt, echoed by the terminal scene (the engine
// may point it elsewhere). Defaults to the standard data directory.
var TerminalMessagesFile = func() string {
	d, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	name := "marquee"
	if strings.Contains(d, "Library/Application Support") {
		name = "Marquee"
	}
	return filepath.Join(d, name, "messages.txt")
}()

// terminalMessages mirrors board.py read_messages(): every line that is not blank, a "#"
// comment or an "@" setting.
func terminalMessages() []string {
	text := board.DefaultMessages
	if TerminalMessagesFile != "" {
		if data, err := os.ReadFile(TerminalMessagesFile); err == nil {
			text = string(data)
		}
	}
	var out []string
	for _, raw := range strings.Split(text, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "@") {
			continue
		}
		out = append(out, line)
	}
	if len(out) == 0 {
		out = []string{"(messages.txt is empty)"}
	}
	return out
}

// pyRepr formats a string like Python's repr(): single quotes unless the text contains a
// single quote and no double quote.
func pyRepr(s string) string {
	q := "'"
	if strings.Contains(s, "'") && !strings.Contains(s, `"`) {
		q = `"`
	}
	var b strings.Builder
	b.WriteString(q)
	for _, r := range s {
		switch {
		case r == '\\':
			b.WriteString(`\\`)
		case string(r) == q:
			b.WriteString(`\` + q)
		case r == '\n':
			b.WriteString(`\n`)
		case r == '\r':
			b.WriteString(`\r`)
		case r == '\t':
			b.WriteString(`\t`)
		case r < 0x20 || r == 0x7f:
			fmt.Fprintf(&b, `\x%02x`, r)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteString(q)
	return b.String()
}

func (tm *terminal) text(s string) *led.Mask {
	if m, ok := tm.cache[s]; ok {
		return m
	}
	if len(tm.cache) > 64 {
		tm.cache = map[string]*led.Mask{}
	}
	t := s
	if t == "" {
		t = " "
	}
	m := led.TextAt(t, "mono", termSize, 0)
	tm.cache[s] = m
	return m
}

func (tm *terminal) Draw(g *led.Grid, t, dt float64) {
	if len(tm.queue) == 0 && tm.typed == "" {
		tm.queue = tm.script()
		tm.lines = append(tm.lines, "")
	}
	if t >= tm.nextChar {
		if len(tm.queue) > 0 {
			target := []rune(tm.queue[0])
			if n := len([]rune(tm.typed)); n < len(target) {
				tm.typed = string(target[:n+1])
				pause := 0.012
				if strings.HasPrefix(tm.queue[0], "$") {
					pause = 0.09
				}
				tm.nextChar = t + pause + rand.Float64()*0.05
			} else {
				tm.lines = append(tm.lines, tm.queue[0])
				tm.queue = tm.queue[1:]
				tm.typed = ""
				tm.nextChar = t + 0.6
			}
		} else {
			tm.nextChar = t + 2.5
		}
	}
	if len(tm.lines) > 50 {
		tm.lines = tm.lines[len(tm.lines)-50:]
	}
	visible := append([]string{}, tm.lines...)
	if len(tm.queue) > 0 {
		visible = append(visible, tm.typed)
	}
	if len(visible) > 5 {
		visible = visible[len(visible)-5:]
	}
	var lit [rows][cols]bool
	cx, cy := 4, 4
	for i, line := range visible {
		m := tm.text(line)
		y := 4 + i*termLineH
		w := min(m.W, cols-4)
		for my := 0; my < termSize+3 && y+my < rows; my++ {
			for mx := 0; mx < w; mx++ {
				if m.At(mx, my) {
					lit[y+my][4+mx] = true
				}
			}
		}
		cx, cy = 4+2, y
		if line != "" {
			cx = 4 + w + 2
		}
	}
	if int(t*2)%2 == 0 && cx+6 < cols {
		for y := cy; y < min(cy+termSize-1, rows); y++ {
			for x := cx; x < cx+6; x++ {
				lit[y][x] = true
			}
		}
	}
	phosphor(g, &lit, fcol{70, 255, 120})
}

// phosphor draws lit LEDs in tint with a soft 3x3 glow and dimmer alternate rows (scanlines).
func phosphor(g *led.Grid, lit *[rows][cols]bool, tint fcol) {
	for y := range rows {
		scan := 1.0
		if y%2 == 1 {
			scan = 0.82
		}
		for x := range cols {
			level := 0.0
			if lit[y][x] {
				level = 1
			} else {
				n := 0
				for dy := -1; dy <= 1; dy++ {
					for dx := -1; dx <= 1; dx++ {
						if lit[(y+dy+rows)%rows][(x+dx+cols)%cols] {
							n++
						}
					}
				}
				level = float64(n) / 9 * 0.22
			}
			c := tint.scale(level * scan).rgb()
			if c.Luma() >= 22 {
				g.Set(x, y, c)
			}
		}
	}
}
