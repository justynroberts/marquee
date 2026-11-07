package audio

import (
	"math"
	"math/rand/v2"
	"testing"
)

func TestFFTPeak(t *testing.T) {
	in := make([]float64, N)
	f := 1000.0
	for i := range in {
		in[i] = math.Sin(2 * math.Pi * f * float64(i) / Rate)
	}
	mag := magnitudes(in)
	best := 0
	for k := range mag {
		if mag[k] > mag[best] {
			best = k
		}
	}
	want := int(math.Round(f * N / Rate))
	if best != want {
		t.Fatalf("peak bin %d, want %d", best, want)
	}
}

// Synth is the kick + chord + hi-hat test signal: a kick every 0.5 s.
func Synth(t0 float64, n int, rng *rand.Rand) []float32 {
	out := make([]float32, n)
	for i := range out {
		t := t0 + float64(i)/Rate
		beat := math.Mod(t, 0.5)
		if beat < 0 {
			beat += 0.5
		}
		kick := math.Sin(2*math.Pi*(55+80*math.Exp(-beat*30))*t) * math.Exp(-beat*9) * 0.9
		chord := 0.0
		for _, fr := range []float64{220, 277, 330, 660} {
			chord += math.Sin(2 * math.Pi * fr * t)
		}
		chord *= 0.06
		hb := math.Mod(t+0.25, 0.5)
		hat := rng.NormFloat64() * 0.04 * math.Exp(-hb*40)
		out[i] = float32(kick + chord + hat)
	}
	return out
}

func TestBeatDetection(t *testing.T) {
	clock := 1000.0
	e := NewEngine(func() float64 { return clock })
	rng := rand.New(rand.NewPCG(1, 2))
	beats := 0
	tt := 0.0
	for i := 0; i < 48; i++ { // 4 s at 12 fps = 8 kicks
		tt += 1.0 / 12
		clock += 1.0 / 12
		e.Feed(Synth(tt-0.093, ring, rng))
		f := e.Analyse()
		if f.Beat {
			beats++
		}
		if f.Silent || f.Err != "" {
			t.Fatalf("unexpected silent/err: %+v", f.Err)
		}
	}
	if beats != 8 {
		t.Fatalf("beats %d of 8", beats)
	}
}

func TestSilentWithoutInput(t *testing.T) {
	clock := 0.0
	e := NewEngine(func() float64 { return clock })
	clock = 4
	if !e.Analyse().Silent {
		t.Fatal("expected silent after 3 s of zeros")
	}
}

func TestAnalyseCached(t *testing.T) {
	clock := 0.0
	e := NewEngine(func() float64 { return clock })
	a := e.Analyse()
	clock += 0.01
	b := e.Analyse()
	if a.Beats != b.Beats || &a.Wave[0] != &b.Wave[0] {
		t.Fatal("analysis within 20 ms should be shared")
	}
}
