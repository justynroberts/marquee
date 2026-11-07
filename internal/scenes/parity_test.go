package scenes

import (
	"encoding/json"
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/justynroberts/marquee/internal/led"
)

// TestParityRender writes frames for side-by-side comparison with the Python reference.
// Run with MARQUEE_PARITY_DIR=/some/dir (optionally MARQUEE_PARITY_SCENES=a,b).
func TestParityRender(t *testing.T) {
	dir := os.Getenv("MARQUEE_PARITY_DIR")
	if dir == "" {
		t.Skip("set MARQUEE_PARITY_DIR")
	}
	names := []string{"matrix", "terminal", "fire", "life", "warp", "plasma", "rain", "clock", "bounce", "fireworks", "aquarium", "countdown"}
	if s := os.Getenv("MARQUEE_PARITY_SCENES"); s != "" {
		names = strings.Split(s, ",")
	}
	runs := 1
	if r := os.Getenv("MARQUEE_PARITY_RUNS"); r != "" {
		fmt.Sscan(r, &runs)
	}
	for run := 0; run < runs; run++ {
		for _, name := range names {
			for _, T := range []float64{0.5, 2, 5} {
				var sc Scene
				if name == "countdown" {
					sc = NewCountdown(300, "standup")
				} else {
					info, ok := Get(name)
					if !ok {
						t.Fatalf("no scene %s", name)
					}
					sc = info.New()
				}
				const dt = 1.0 / 12
				var g *led.Grid
				for tt := 0.0; tt <= T+1e-9; tt += dt {
					g = led.NewGrid()
					sc.Draw(g, tt, dt)
				}
				fn := fmt.Sprintf("go-%s-%g.png", name, T)
				if runs > 1 {
					fn = fmt.Sprintf("go-%s-%g-r%d.png", name, T, run)
				}
				f, err := os.Create(filepath.Join(dir, fn))
				if err != nil {
					t.Fatal(err)
				}
				_ = png.Encode(f, g.Image(led.Ground))
				f.Close()
			}
		}
	}
}

// TestParityMatrixAtlas writes the glyph masks as raw bytes (10 x 8*n, row-major).
func TestParityMatrixAtlas(t *testing.T) {
	dir := os.Getenv("MARQUEE_PARITY_DIR")
	if dir == "" {
		t.Skip("set MARQUEE_PARITY_DIR")
	}
	gl := matrixGlyphs()
	w := mCW * len(gl)
	buf := make([]byte, mCH*w)
	for i, m := range gl {
		for y := 0; y < mCH; y++ {
			for x := 0; x < mCW; x++ {
				if m.At(x, y) {
					buf[y*w+i*mCW+x] = 1
				}
			}
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "go_matrix_atlas.bin"), buf, 0o644); err != nil {
		t.Fatal(err)
	}
}

var parityTexts = []struct {
	s, font string
	size    int
}{{"18:39", "sans", 54}, {"05:00", "sans", 54}, {"FRI", "mono", 16}, {"06 NOV", "mono", 11}, {"2026 W45", "mono", 9}, {"STANDUP", "mono", 13}, {"$ uptime", "mono", 11}}

// TestParityText writes led.Text masks as "W\n" + rows of 0/1 for comparison with board.text_cells.
func TestParityText(t *testing.T) {
	dir := os.Getenv("MARQUEE_PARITY_DIR")
	if dir == "" {
		t.Skip("set MARQUEE_PARITY_DIR")
	}
	for i, p := range parityTexts {
		m := led.Text(p.s, p.font, p.size)
		var b strings.Builder
		fmt.Fprintf(&b, "%d\n", m.W)
		for y := 0; y < m.H; y++ {
			for x := 0; x < m.W; x++ {
				if m.At(x, y) {
					b.WriteByte('1')
				} else {
					b.WriteByte('0')
				}
			}
			b.WriteByte('\n')
		}
		_ = os.WriteFile(filepath.Join(dir, fmt.Sprintf("go-text-%d.txt", i)), []byte(b.String()), 0o644)
	}
}

func TestTerminalEchoParity(t *testing.T) {
	path := os.Getenv("MARQUEE_PARITY_DIR")
	if path == "" {
		t.Skip("set MARQUEE_PARITY_DIR")
	}
	data, err := os.ReadFile(filepath.Join(path, "terminal_cases.json"))
	if err != nil {
		t.Skip(err)
	}
	var c struct {
		Repr    [][2]string `json:"repr"`
		Default string      `json:"default"`
		Echo    []string    `json:"echo"`
	}
	if err := json.Unmarshal(data, &c); err != nil {
		t.Fatal(err)
	}
	for _, r := range c.Repr {
		if got := pyRepr(r[0]); got != r[1] {
			t.Errorf("pyRepr(%q) = %s, Python %s", r[0], got, r[1])
		}
	}
	f := filepath.Join(t.TempDir(), "messages.txt")
	_ = os.WriteFile(f, []byte(c.Default), 0o644)
	old := TerminalMessagesFile
	TerminalMessagesFile = f
	defer func() { TerminalMessagesFile = old }()
	var echoes []string
	for _, l := range newTerminal().script() {
		if strings.HasPrefix(l, "$ echo ") {
			echoes = append(echoes, l)
		}
	}
	if fmt.Sprint(echoes) != fmt.Sprint(c.Echo) {
		t.Errorf("echoes\n go %q\n py %q", echoes, c.Echo)
	}
}
