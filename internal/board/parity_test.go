package board

import (
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/justynroberts/marquee/internal/led"
)

// TestParityRender writes frames for the parity cases in $PARITY_DIR/cases to $PARITY_DIR/go,
// matching the offsets and times of the Python reference harness. Skipped unless PARITY_DIR is set.
func TestParityRender(t *testing.T) {
	dir := os.Getenv("PARITY_DIR")
	if dir == "" {
		t.Skip("PARITY_DIR not set")
	}
	frozenTokens = map[string]string{"time": "12:34", "date": "06 Nov 2026", "day": "Friday", "cpu": "37", "mem": "64"}
	defer func() { frozenTokens = nil }()
	cases, _ := filepath.Glob(filepath.Join(dir, "cases", "*.txt"))
	times := []float64{0.37, 1.3, 2.05}
	for _, c := range cases {
		data, err := os.ReadFile(c)
		if err != nil {
			t.Fatal(err)
		}
		s := build(string(data))
		n := len(s.cols)
		name := strings.TrimSuffix(filepath.Base(c), ".txt")
		_ = os.WriteFile(filepath.Join(dir, "go", name+".meta"), []byte(fmt.Sprintf("%d\n", n)), 0o644)
		count := max(1, (n-led.Cols)/230+1)
		count = min(count, 8)
		for i := 0; i < count; i++ {
			off := (led.Cols + i*230) % n
			for j, tm := range times {
				g := led.NewGrid()
				render(g, s, off, tm)
				f, _ := os.Create(filepath.Join(dir, "go", fmt.Sprintf("%s-%d-%d.png", name, i, j)))
				_ = png.Encode(f, g.Image(s.settings.Bg))
				f.Close()
			}
		}
	}
}
