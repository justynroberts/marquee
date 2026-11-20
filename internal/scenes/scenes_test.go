package scenes

import (
	"math"
	"testing"
	"time"

	"github.com/justynroberts/marquee/internal/audio"
	"github.com/justynroberts/marquee/internal/led"
)

// fakeAudio returns a loud, beating spectrum so the visualisers have something to draw.
func fakeAudio() func() audio.Features {
	n := 0
	return func() audio.Features {
		n++
		var f audio.Features
		for i := range f.Bands {
			f.Bands[i] = 0.5 + 0.5*math.Sin(float64(i)*0.3+float64(n)*0.2)
		}
		f.Level, f.Bass, f.Mid, f.Treble = 0.6, 0.8, 0.5, 0.4
		f.Beat = n%6 == 0
		f.Wave = make([]float32, audio.N)
		for i := range f.Wave {
			f.Wave[i] = float32(0.5 * math.Sin(float64(i)*0.05))
		}
		return f
	}
}

func litCount(g *led.Grid) int {
	n := 0
	for y := range rows {
		for x := range cols {
			if g.Lit[y][x] {
				n++
			}
		}
	}
	return n
}

func TestAllScenes(t *testing.T) {
	AudioSource = fakeAudio()
	CameraSource = fakeCamera()
	want := map[string]string{}
	for _, s := range []string{"matrix", "terminal", "fire", "life", "warp", "plasma", "rain", "clock", "bounce", "fireworks", "aquarium", "dashboard"} {
		want[s] = "scene"
	}
	for _, s := range []string{"mirror", "waterfall", "scope", "pulse", "meter", "spectrum"} {
		want[s] = "audio"
	}
	for _, s := range []string{"camera", "popart", "motion", "slitscan", "snowfall", "bubbles", "airpaint", "thermal", "kaleido", "neon", "clones"} {
		want[s] = "camera"
	}
	for name, group := range want {
		info, ok := Get(name)
		if !ok || info.Group != group {
			t.Errorf("%s: registered=%v group=%q want %q", name, ok, info.Group, group)
		}
	}
	for _, info := range List() {
		t.Run(info.Name, func(t *testing.T) {
			sc := info.New()
			maxLit := 0
			var worst time.Duration
			const dt = 1.0 / 12
			for i := 0; i < 60; i++ {
				g := led.NewGrid()
				start := time.Now()
				sc.Draw(g, float64(i)*dt, dt)
				if i > 0 { // first frame warms font/emoji caches
					worst = max(worst, time.Since(start))
				}
				maxLit = max(maxLit, litCount(g))
			}
			if maxLit == 0 {
				t.Fatal("never lit an LED")
			}
			if worst > 15*time.Millisecond {
				t.Errorf("slowest frame %v > 15ms", worst)
			}
			t.Logf("%-10s max lit %5d  slowest frame %v", info.Name, maxLit, worst)
		})
	}
}

func TestAudioProblemScreens(t *testing.T) {
	for _, f := range []audio.Features{{Silent: true}, {Err: "no microphone"}} {
		AudioSource = func() audio.Features { return f }
		g := led.NewGrid()
		(&mirror{}).Draw(g, 0, 0.1)
		if litCount(g) == 0 {
			t.Fatalf("no message for %+v", f)
		}
	}
}

func TestCountdown(t *testing.T) {
	c := NewCountdown(90, "tea")
	if r := c.Remaining(); r < 89 || r > 90 {
		t.Fatalf("remaining %v", r)
	}
	g := led.NewGrid()
	c.Draw(g, 0, 0.1)
	if litCount(g) == 0 {
		t.Fatal("countdown drew nothing")
	}
	done := NewCountdown(0, "")
	g = led.NewGrid()
	done.Draw(g, 0, 0.1)
	if litCount(g) == 0 {
		t.Fatal("TIME'S UP not drawn")
	}
	if _, ok := Get("countdown"); ok {
		t.Fatal("countdown must not be registered")
	}
}

func BenchmarkScenes(b *testing.B) {
	AudioSource = fakeAudio()
	for _, info := range List() {
		b.Run(info.Name, func(b *testing.B) {
			sc := info.New()
			sc.Draw(led.NewGrid(), 0, 1.0/12)
			for i := 0; b.Loop(); i++ {
				sc.Draw(led.NewGrid(), float64(i)/12, 1.0/12)
			}
		})
	}
}
