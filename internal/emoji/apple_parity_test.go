package emoji

import (
	"encoding/json"
	"image/png"
	"os"
	"testing"
)

// TestAppleParity compares Render with board.py's emoji_cells (EMOJI_PARITY=<dir>).
func TestAppleParity(t *testing.T) {
	dir := os.Getenv("EMOJI_PARITY")
	if dir == "" {
		t.Skip()
	}
	var cases []struct {
		E    string
		Z    int
		F    string
		None bool
	}
	b, _ := os.ReadFile(dir + "/cases.json")
	_ = json.Unmarshal(b, &cases)
	exact := 0
	for _, c := range cases {
		s := renderApple(c.E, c.Z)
		if c.None {
			if s != nil {
				t.Errorf("%q: Python has none", c.E)
			}
			continue
		}
		if s == nil {
			t.Errorf("%q %d: not rendered", c.E, c.Z)
			continue
		}
		fl, _ := os.Open(c.F + "-lit.png")
		lit, _ := png.Decode(fl)
		fl.Close()
		fr, _ := os.Open(c.F + "-rgb.png")
		rgb, _ := png.Decode(fr)
		fr.Close()
		if lit.Bounds().Dx() != s.W || lit.Bounds().Dy() != s.H {
			t.Errorf("%q %d: size %dx%d vs %dx%d", c.E, c.Z, s.W, s.H, lit.Bounds().Dx(), lit.Bounds().Dy())
			continue
		}
		litDiff, colDiff := 0, 0
		for y := 0; y < s.H; y++ {
			for x := 0; x < s.W; x++ {
				r, _, _, _ := lit.At(x, y).RGBA()
				if (r > 0) != s.Lit[y*s.W+x] {
					litDiff++
				}
				pr, pg, pb, _ := rgb.At(x, y).RGBA()
				gc := s.Col[y*s.W+x]
				if uint8(pr>>8) != gc.R || uint8(pg>>8) != gc.G || uint8(pb>>8) != gc.B {
					colDiff++
				}
			}
		}
		if litDiff == 0 && colDiff == 0 {
			exact++
		} else {
			t.Logf("%q %d: lit diff %d, colour diff %d of %d", c.E, c.Z, litDiff, colDiff, s.W*s.H)
		}
	}
	t.Logf("%d of %d identical", exact, len(cases))
}
