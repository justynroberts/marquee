package engine

import (
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"math"
	"os"
	"testing"
)

func TestMediaParityWithPillow(t *testing.T) {
	dir := os.Getenv("MEDIA_PARITY")
	if dir == "" {
		t.Skip()
	}
	var want map[string][]float64
	b, _ := os.ReadFile(dir + "/py.json")
	_ = json.Unmarshal(b, &want)
	for _, n := range []string{"t.gif", "t.webp", "t.png"} {
		m, err := loadMedia(dir + "/" + n)
		if err != nil {
			t.Fatal(n, err)
		}
		if len(m.frames) != len(want[n]) {
			t.Errorf("%s: %d frames, Pillow %d", n, len(m.frames), len(want[n]))
			continue
		}
		for i := range m.frames {
			if math.Abs(m.delays[i]-want[n][i]) > 1e-9 {
				t.Errorf("%s frame %d: delay %v, Pillow %v", n, i, m.delays[i], want[n][i])
			}
			f, _ := os.Open(fmt.Sprintf("%s/py-%s-%d.png", dir, n, i))
			ref, _ := png.Decode(f)
			f.Close()
			t.Logf("%s frame %d: mean abs diff %.2f", n, i, diff(m.frames[i], ref))
			if d := diff(m.frames[i], ref); d > 3 {
				t.Errorf("%s frame %d: mean abs diff %.2f", n, i, d)
			}
		}
	}
}

func diff(a *image.RGBA, b image.Image) float64 {
	var sum float64
	n := 0
	for y := 0; y < a.Bounds().Dy(); y++ {
		for x := 0; x < a.Bounds().Dx(); x++ {
			r, g, bb, _ := b.At(x, y).RGBA()
			o := y*a.Stride + x*4
			sum += math.Abs(float64(a.Pix[o])-float64(r>>8)) + math.Abs(float64(a.Pix[o+1])-float64(g>>8)) + math.Abs(float64(a.Pix[o+2])-float64(bb>>8))
			n += 3
		}
	}
	return sum / float64(n)
}
