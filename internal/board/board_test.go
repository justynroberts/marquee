package board

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/justynroberts/marquee/internal/led"
)

func kinds(segs []Segment) string {
	var b []string
	for _, s := range segs {
		b = append(b, s.Kind+":"+s.Value)
	}
	return strings.Join(b, "|")
}

func TestTagsColoursAndNesting(t *testing.T) {
	s := DefaultSettings()
	segs := parseMessage("a [red]b [#00ff00]c[/] d[/] e", s)
	want := []struct {
		v string
		c led.RGB
	}{{"a ", led.Ink}, {"b ", led.Palette["red"]}, {"c", led.RGB{G: 255}}, {" d", led.Palette["red"]}, {" e", led.Ink}}
	if len(segs) != len(want) {
		t.Fatalf("got %s", kinds(segs))
	}
	for i, w := range want {
		if segs[i].Value != w.v || segs[i].Style.Colour != w.c {
			t.Errorf("seg %d = %q %v, want %q %v", i, segs[i].Value, segs[i].Style.Colour, w.v, w.c)
		}
	}
	// colour= and bg=
	segs = parseMessage("[colour=#fff][bg=blue]x", s)
	if segs[0].Style.Colour != (led.RGB{R: 255, G: 255, B: 255}) || !segs[0].Style.HasBg || segs[0].Style.Bg != led.Palette["blue"] {
		t.Errorf("colour/bg: %+v", segs[0].Style)
	}
}

func TestUnknownTagsAreText(t *testing.T) {
	segs := parseMessage("hi [nope] [colour=zzz] [size=big]", DefaultSettings())
	joined := ""
	for _, sg := range segs {
		joined += sg.Value
	}
	if joined != "hi [nope] [colour=zzz] [size=big]" {
		t.Errorf("unknown tags should render as text, got %q", joined)
	}
}

func TestEffectsFontsSizes(t *testing.T) {
	s := DefaultSettings()
	segs := parseMessage("[rainbow][wave]x[/]y[/][serif][big]z[/][font=Rockwell][size=20]w[/][/][small][tiny]q[plain][pulse]p", s)
	byV := map[string]Style{}
	for _, sg := range segs {
		byV[sg.Value] = sg.Style
	}
	if byV["x"].Effects != FxRainbow|FxWave || byV["y"].Effects != FxRainbow {
		t.Errorf("effects combine: x=%b y=%b", byV["x"].Effects, byV["y"].Effects)
	}
	if byV["z"].Font != "serif" || byV["z"].Size != 58 {
		t.Errorf("z %+v", byV["z"])
	}
	if byV["w"].Font != "Rockwell" || byV["w"].Size != 20 {
		t.Errorf("w %+v", byV["w"])
	}
	if byV["q"].Size != 14 || byV["p"].Effects != FxPulse {
		t.Errorf("q %+v p %+v", byV["q"], byV["p"])
	}
	// size clamps
	segs = parseMessage("[size=200]a[size=2]b", s)
	if segs[0].Style.Size != led.Rows || segs[1].Style.Size != 8 {
		t.Errorf("clamp: %d %d", segs[0].Style.Size, segs[1].Style.Size)
	}
}

func TestAlertAndSprites(t *testing.T) {
	segs := parseMessage("! Down :fire: :nope:", DefaultSettings())
	if segs[0].Style.Colour != led.Alert || segs[0].Style.Effects != FxPulse {
		t.Errorf("alert style %+v", segs[0].Style)
	}
	got := kinds(segs)
	if !strings.Contains(got, "sprite:fire") || !strings.Contains(got, ":nope:") {
		t.Errorf("sprites: %s", got)
	}
}

func TestEmojiSegments(t *testing.T) {
	segs := parseMessage("Coffee ☕️ ok", DefaultSettings())
	if kinds(segs) != "text:Coffee |emoji:☕️|text: ok" {
		t.Errorf("emoji split: %s", kinds(segs))
	}
}

func TestSettingsAndComments(t *testing.T) {
	s, lines := parseFile("# c\n@step=5\n@gap=10\n@font=serif\n@size=30\n@colour=#123456\n@dim=red\n@bg=000\n@theme=Green\n@location=Paris\n@step=999\nhello\n\n")
	if s.Step != 40 || s.Gap != 10 || s.Font != "serif" || s.Size != 30 || s.Colour != (led.RGB{R: 0x12, G: 0x34, B: 0x56}) ||
		s.Dim != led.Palette["red"] || s.Bg != (led.RGB{}) || s.Theme != "green" || s.Location != "Paris" {
		t.Errorf("settings: %+v", s)
	}
	if len(lines) != 1 || lines[0] != "hello" {
		t.Errorf("lines: %v", lines)
	}
	_, lines = parseFile("# only comments\n")
	if len(lines) != 1 || !strings.Contains(lines[0], "empty") {
		t.Errorf("empty file: %v", lines)
	}
}

func TestTokens(t *testing.T) {
	SetTokenSource("probe", func() string { return "XYZ" })
	defer SetTokenSource("probe", nil)
	out := fillTokens("{time} {probe} {unknown}")
	if !strings.Contains(out, "XYZ") || !strings.Contains(out, "{unknown}") || strings.Contains(out, "{time}") {
		t.Errorf("tokens: %q", out)
	}
}

func TestSpriteFrames(t *testing.T) {
	for _, name := range SpriteNames() {
		frames := spriteFrames[name]
		if len(frames) != len(spriteSpecs[name].frames) {
			t.Errorf("%s: %d frames", name, len(frames))
		}
		lit := 0
		for y := range spriteH {
			for x := range spriteW {
				if frames[0].lit[y][x] {
					lit++
				}
			}
		}
		if lit == 0 || lit%16 != 0 { // every art pixel is a 4x4 block
			t.Errorf("%s: %d lit LEDs", name, lit)
		}
	}
	// heart beats at 3 fps: frame changes between t=0 and t=0.4
	if spriteFrameAt("heart", 0) == spriteFrameAt("heart", 0.4) {
		t.Error("heart should animate")
	}
	// the R in the first heart frame at art (2,0) is red
	if c := spriteFrames["heart"][0].col[0][2*spriteScale]; c != led.Palette["red"] {
		t.Errorf("heart colour %v", c)
	}
}

func newBoard(t *testing.T, content string) *Board {
	t.Helper()
	p := filepath.Join(t.TempDir(), "messages.txt")
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return New(p)
}

func TestEmojiPlacement(t *testing.T) {
	b := newBoard(t, "🚀\n")
	s := b.strip
	// the emoji sits after the leading blank screen plus the gap, vertically centred
	start := led.Cols + emojiGap
	litRows := map[int]bool{}
	colourful := false
	for x := start; x < start+40; x++ {
		for y := range led.Rows {
			if s.cols[x].lit[y] {
				litRows[y] = true
				c := s.cols[x].col[y]
				if c.R != c.G || c.G != c.B {
					colourful = true
				}
			}
		}
	}
	if len(litRows) < 20 || !colourful {
		t.Fatalf("emoji not drawn in colour: rows=%d colourful=%v", len(litRows), colourful)
	}
	for x := led.Cols; x < start; x++ {
		for y := range led.Rows {
			if s.cols[x].lit[y] {
				t.Fatalf("gap before emoji is lit at %d,%d", x, y)
			}
		}
	}
}

func TestScrollWrapAndPasses(t *testing.T) {
	b := newBoard(t, "@step=7\nhi\n")
	n := len(b.strip.cols)
	if n <= led.Cols {
		t.Fatalf("strip too short: %d", n)
	}
	g := led.NewGrid()
	frames := (n+6)/7 + 1
	for i := 0; i < frames; i++ {
		b.Draw(g, float64(i)/12, 1.0/12)
	}
	if b.Passes() != 1 {
		t.Errorf("passes = %d after %d frames of a %d-column strip", b.Passes(), frames, n)
	}
	if b.offset >= n {
		t.Errorf("offset %d not wrapped (n=%d)", b.offset, n)
	}
}

func TestDrawLightsTextAndBackground(t *testing.T) {
	b := newBoard(t, "@step=1\n[bg=blue]HELLO\n")
	b.offset = led.Cols // first column of the message
	g := led.NewGrid()
	b.Draw(g, 0, 0)
	lit, blue := 0, 0
	for y := range led.Rows {
		for x := range led.Cols {
			if g.Lit[y][x] {
				lit++
			} else if g.Back[y][x] == led.Palette["blue"] {
				blue++
			}
		}
	}
	if lit < 100 || blue < 100 {
		t.Errorf("lit=%d blue background=%d", lit, blue)
	}
	if b.Ground() != led.Ground {
		t.Errorf("ground %v", b.Ground())
	}
}

func TestPhosphorTheme(t *testing.T) {
	b := newBoard(t, "@theme=green\n[red]X\n")
	b.offset = led.Cols
	g := led.NewGrid()
	b.Draw(g, 0, 0)
	for y := range led.Rows {
		for x := range led.Cols {
			if g.Lit[y][x] {
				c := g.Col[y][x]
				if c.G <= c.R {
					t.Fatalf("green phosphor should be green, got %v", c)
				}
				return
			}
		}
	}
	t.Fatal("nothing lit")
}

func TestBlinkAndWave(t *testing.T) {
	b := newBoard(t, "[blink]IIII\n")
	countLit := func(tt float64) int {
		b.offset = led.Cols
		g := led.NewGrid()
		b.Draw(g, tt, 0)
		n := 0
		for y := range led.Rows {
			for x := range led.Cols {
				if g.Lit[y][x] {
					n++
				}
			}
		}
		return n
	}
	if on, off := countLit(0), countLit(0.5); on == 0 || off != 0 {
		t.Errorf("blink: on=%d off=%d", on, off)
	}
}

func TestLint(t *testing.T) {
	text := "@step=x\n@wat=1\n@theme=purple\n[nope]a [colour=zzz]b [size=q]c [font=NoSuchFontXYZ]d :blob: [red]ok[/] :fire:\n# [nope] in a comment\n"
	w := strings.Join(Lint(text), "\n")
	for _, want := range []string{"line 1:", "line 2:", "theme \"purple\"", "unknown tag [nope]", "bad colour \"zzz\"", "size must be a number", "font \"NoSuchFontXYZ\" not found", "unknown sprite :blob:"} {
		if !strings.Contains(w, want) {
			t.Errorf("lint missing %q in:\n%s", want, w)
		}
	}
	if strings.Contains(w, "line 5") || strings.Contains(w, "[red]") || strings.Contains(w, ":fire:") {
		t.Errorf("lint flagged valid content:\n%s", w)
	}
	if got := Lint(DefaultMessages); len(got) != 0 {
		t.Errorf("default messages should lint clean, got %v", got)
	}
}

func TestAppend(t *testing.T) {
	p := filepath.Join(t.TempDir(), "m.txt")
	if err := Append(p, "first\nline"); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(p)
	if !strings.HasPrefix(string(data), DefaultMessages) || !strings.HasSuffix(string(data), "first line\n") {
		t.Errorf("append to new file: %q", string(data)[len(string(data))-40:])
	}
	os.WriteFile(p, []byte("no newline"), 0o644)
	Append(p, "x")
	data, _ = os.ReadFile(p)
	if string(data) != "no newline\nx\n" {
		t.Errorf("append after missing newline: %q", data)
	}
	if Append(p, "  \n ") == nil {
		t.Error("empty message should fail")
	}
}

func TestReloadOnChange(t *testing.T) {
	b := newBoard(t, "one\n")
	before := len(b.strip.cols)
	time.Sleep(10 * time.Millisecond)
	os.WriteFile(b.path, []byte("one two three four five six\n"), 0o644)
	future := time.Now().Add(time.Hour)
	os.Chtimes(b.path, future, future)
	b.checked = time.Time{}
	g := led.NewGrid()
	b.Draw(g, 0, 0)
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		b.mu.Lock()
		n := len(b.strip.cols)
		b.mu.Unlock()
		if n > before {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("board did not reload the changed file")
}

func TestFrameTime(t *testing.T) {
	b := newBoard(t, DefaultMessages)
	g := led.NewGrid()
	b.Draw(g, 0, 0) // warm caches
	const n = 60
	start := time.Now()
	for i := 0; i < n; i++ {
		b.Draw(g, float64(i)/12, 1.0/12)
	}
	per := time.Since(start) / n
	if per > 15*time.Millisecond {
		t.Errorf("frame took %s (limit 15ms)", per)
	}
	t.Logf("frame %s", per)
}

func BenchmarkDraw(bm *testing.B) {
	p := filepath.Join(bm.TempDir(), "m.txt")
	os.WriteFile(p, []byte(DefaultMessages), 0o644)
	b := New(p)
	g := led.NewGrid()
	for i := 0; i < bm.N; i++ {
		b.Draw(g, float64(i)/12, 1.0/12)
	}
}

func TestPythonParityRules(t *testing.T) {
	// Palette: exactly board.py's names.
	for _, n := range []string{"white", "grey", "red", "orange", "yellow", "green", "cyan", "blue", "purple", "pink", "alert", "black"} {
		if _, ok := parseColour(n); !ok {
			t.Errorf("palette %q missing", n)
		}
	}
	for _, n := range []string{"gray", "navy", "#12345", "zzz", "#ggg"} {
		if _, ok := parseColour(n); ok {
			t.Errorf("%q should not be a colour", n)
		}
	}
	if c, _ := parseColour(" #0F8 "); c != (led.RGB{R: 0, G: 0xff, B: 0x88}) {
		t.Errorf("short hex = %v", c)
	}
	// Tags: digits only for size; Python's font shortcuts only.
	if _, ok := applyTag(Style{}, "size=-5"); ok {
		t.Error("size=-5 should be unknown")
	}
	if st, ok := applyTag(Style{}, "size=99"); !ok || st.Size != led.Rows {
		t.Error("size clamps to 66")
	}
	if _, ok := applyTag(Style{}, "cjk"); ok {
		t.Error("[cjk] is not a board.py font shortcut")
	}
	// Settings: isdigit, step clamps to 1..40.
	s := DefaultSettings()
	if !applySetting(&s, "step", "0") || s.Step != 1 {
		t.Errorf("@step=0 -> %d, want 1", s.Step)
	}
	if applySetting(&s, "gap", "+5") || applySetting(&s, "size", "-1") {
		t.Error("signed numbers must be rejected like str.isdigit")
	}
	if !applySetting(&s, "step", "99") || s.Step != 40 {
		t.Errorf("@step=99 -> %d", s.Step)
	}
}
