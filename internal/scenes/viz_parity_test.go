package scenes

// Parity harness against thermalright-display/viz.py. Opt-in: set MARQUEE_VIZ_PARITY to a
// directory holding signal.f32 (written by the Python harness); frames and a log are written
// next to it as go_*.png and go_log.json.

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/justynroberts/marquee/internal/audio"
	"github.com/justynroberts/marquee/internal/led"
)

func TestVizParityHarness(t *testing.T) {
	dir := os.Getenv("MARQUEE_VIZ_PARITY")
	if dir == "" {
		t.Skip("set MARQUEE_VIZ_PARITY to run the parity harness")
	}
	raw, err := os.ReadFile(filepath.Join(dir, "signal.f32"))
	if err != nil {
		t.Fatal(err)
	}
	sig := make([]float32, len(raw)/4)
	for i := range sig {
		sig[i] = math.Float32frombits(binary.LittleEndian.Uint32(raw[i*4:]))
	}
	const fps, secs = 12, 5.0
	capture := []float64{2.0, 3.0, 4.5}
	clock := 1000.0
	e := audio.NewEngine(func() float64 { return clock })
	old := AudioSource
	defer func() { AudioSource = old }()
	AudioSource = e.Analyse

	names := []string{"mirror", "waterfall", "scope", "pulse", "meter", "spectrum"}
	vs := map[string]Scene{}
	for _, n := range names {
		info, _ := Get(n)
		vs[n] = info.New()
	}
	vs["pulse"].(*pulse).hue = 0.3
	type entry struct {
		T                        float64   `json:"t"`
		Bands                    []float64 `json:"bands"`
		Beat                     bool      `json:"beat"`
		Level, Bass, Mid, Treble float64
	}
	var log []entry
	for frame := 1; ; frame++ {
		tt := float64(frame) / fps
		if tt > secs+1e-9 {
			break
		}
		clock = 1000 + tt
		end := int(math.Round(tt * audio.Rate))
		buf := make([]float32, 4096)
		if end >= 4096 {
			copy(buf, sig[end-4096:end])
		} else {
			copy(buf[4096-end:], sig[:end])
		}
		e.Feed(buf)
		grids := map[string]*led.Grid{}
		for _, n := range names {
			g := led.NewGrid()
			vs[n].Draw(g, tt, 1.0/fps)
			grids[n] = g
		}
		f := e.Analyse()
		log = append(log, entry{tt, append([]float64(nil), f.Bands[:]...), f.Beat, f.Level, f.Bass, f.Mid, f.Treble})
		if len(capture) > 0 && math.Abs(tt-capture[0]) < 1e-6 {
			for n, g := range grids {
				save(t, filepath.Join(dir, fmt.Sprintf("go_%s_%.1f.png", n, capture[0])), g)
			}
			capture = capture[1:]
		}
	}
	data, _ := json.Marshal(log)
	_ = os.WriteFile(filepath.Join(dir, "go_log.json"), data, 0o644)

	AudioSource = func() audio.Features { return audio.Features{Silent: true} }
	g := led.NewGrid()
	(&mirror{}).Draw(g, 0, 0)
	save(t, filepath.Join(dir, "go_silent.png"), g)
	AudioSource = func() audio.Features { return audio.Features{Err: "no microphone: device unavailable"} }
	g = led.NewGrid()
	(&mirror{}).Draw(g, 0, 0)
	save(t, filepath.Join(dir, "go_error.png"), g)
}

func save(t *testing.T, path string, g *led.Grid) {
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, g.Image(led.Ground)); err != nil {
		t.Fatal(err)
	}
}
