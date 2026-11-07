// Package audio captures the default microphone and turns it into features the visualisers
// draw from: 64 log-spaced bands with automatic gain, level, bass/mid/treble, beats, and the
// raw waveform.
//
// One Engine is shared by every visualiser (Shared). Tests build their own with NewEngine and
// push samples with Feed, so nothing here needs a microphone to be exercised.
package audio

import (
	"encoding/binary"
	"fmt"
	"log/slog"
	"math"
	"sort"
	"sync"
	"time"

	"github.com/gen2brain/malgo"
)

const (
	Rate = 44100
	N    = 2048 // FFT size
	NB   = 64   // bands
	ring = N * 2
)

// Features is one analysis of the most recent audio.
type Features struct {
	Bands             [NB]float64 // 0..1, low to high frequency
	Level             float64     // overall loudness 0..1
	Bass, Mid, Treble float64     // 0..1
	Beat              bool        // a beat was detected in this analysis
	SinceBeat         float64     // seconds since the last beat
	Beats             int         // beats so far
	Wave              []float32   // latest N samples
	Silent            bool        // nothing but exact zeros for more than 3 s (no mic permission)
	Err               string      // capture could not start
}

// Engine holds the capture ring buffer and the analysis state.
type Engine struct {
	mu      sync.Mutex
	buf     []float32
	heard   bool
	clock   func() float64
	started float64

	bins   [NB][]int
	centre [NB]float64
	window []float64

	ref      float64
	smooth   [NB]float64
	bassAvg  float64
	lastBeat float64
	beats    int
	tLast    float64
	feat     Features
	haveFeat bool

	errMsg string
	ctx    *malgo.AllocatedContext
	dev    *malgo.Device
}

// NewEngine returns an engine with no capture attached. clock returns seconds (monotonic);
// nil means real time.
func NewEngine(clock func() float64) *Engine {
	if clock == nil {
		t0 := time.Now()
		clock = func() float64 { return time.Since(t0).Seconds() + 1000 }
	}
	e := &Engine{buf: make([]float32, ring), clock: clock, ref: -20, bassAvg: 1e-6, lastBeat: -1e9}
	e.started = clock()
	lo, hi := math.Log(30), math.Log(16000)
	edges := make([]float64, NB+1)
	for i := range edges {
		edges[i] = math.Exp(lo + (hi-lo)*float64(i)/NB)
	}
	for b := 0; b < NB; b++ {
		var idx []int
		for k := 0; k <= N/2; k++ {
			f := float64(k) * Rate / N
			if f >= edges[b] && f < edges[b+1] {
				idx = append(idx, k)
			}
		}
		if len(idx) == 0 {
			best, bestD := 0, math.Inf(1)
			for k := 0; k <= N/2; k++ {
				if d := math.Abs(float64(k)*Rate/N - edges[b]); d < bestD {
					best, bestD = k, d
				}
			}
			idx = []int{best}
		}
		e.bins[b] = idx
		e.centre[b] = math.Sqrt(edges[b] * edges[b+1])
	}
	e.window = make([]float64, N)
	for i := range e.window {
		e.window[i] = 0.5 - 0.5*math.Cos(2*math.Pi*float64(i)/float64(N-1))
	}
	return e
}

// Feed appends samples to the ring buffer (called by the capture callback, or by tests).
func (e *Engine) Feed(samples []float32) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if len(samples) >= len(e.buf) {
		copy(e.buf, samples[len(samples)-len(e.buf):])
		return
	}
	copy(e.buf, e.buf[len(samples):])
	copy(e.buf[len(e.buf)-len(samples):], samples)
}

// Start opens the default capture device (mono, 44.1 kHz, float32).
func (e *Engine) Start() error {
	ctx, err := malgo.InitContext(nil, malgo.ContextConfig{}, nil)
	if err != nil {
		return fmt.Errorf("audio context: %w", err)
	}
	cfg := malgo.DefaultDeviceConfig(malgo.Capture)
	cfg.Capture.Format = malgo.FormatF32
	cfg.Capture.Channels = 1
	cfg.SampleRate = Rate
	cb := malgo.DeviceCallbacks{
		Data: func(_, in []byte, frames uint32) {
			n := len(in) / 4
			s := make([]float32, n)
			for i := 0; i < n; i++ {
				s[i] = math.Float32frombits(binary.LittleEndian.Uint32(in[i*4:]))
			}
			e.Feed(s)
		},
	}
	dev, err := malgo.InitDevice(ctx.Context, cfg, cb)
	if err != nil {
		_ = ctx.Uninit()
		ctx.Free()
		return fmt.Errorf("audio device: %w", err)
	}
	if err := dev.Start(); err != nil {
		dev.Uninit()
		_ = ctx.Uninit()
		ctx.Free()
		return fmt.Errorf("audio start: %w", err)
	}
	e.mu.Lock()
	e.ctx, e.dev = ctx, dev
	e.mu.Unlock()
	return nil
}

// Stop closes the capture device if one is open.
func (e *Engine) Stop() {
	e.mu.Lock()
	dev, ctx := e.dev, e.ctx
	e.dev, e.ctx = nil, nil
	e.mu.Unlock()
	if dev != nil {
		_ = dev.Stop()
		dev.Uninit()
	}
	if ctx != nil {
		_ = ctx.Uninit()
		ctx.Free()
	}
}

// Analyse returns features for now. Calls within 20 ms share one FFT.
func (e *Engine) Analyse() Features {
	now := e.clock()
	e.mu.Lock()
	if e.haveFeat && now-e.tLast < 0.02 {
		f := e.feat
		e.mu.Unlock()
		return f
	}
	dt := 1.0 / 12
	if e.haveFeat {
		dt = math.Min(0.2, now-e.tLast)
	}
	e.tLast = now
	buf := append([]float32(nil), e.buf...)
	errMsg := e.errMsg
	e.mu.Unlock()

	frame := buf[len(buf)-N:]
	in := make([]float64, N)
	nonzero := false
	for i, v := range frame {
		if v != 0 {
			nonzero = true
		}
		in[i] = float64(v) * e.window[i]
	}
	if nonzero {
		e.heard = true
	}
	mag := magnitudes(in)
	var raw, db [NB]float64
	for b := 0; b < NB; b++ {
		s := 0.0
		for _, k := range e.bins[b] {
			s += mag[k] / N
		}
		raw[b] = s / float64(len(e.bins[b])) * math.Pow(e.centre[b]/200, 0.35) // tilt toward treble
		db[b] = 20 * math.Log10(raw[b]+1e-9)
	}
	sorted := db
	s := sorted[:]
	sort.Float64s(s)
	top := percentile(s, 96)
	if top > e.ref {
		e.ref += (top - e.ref) * 0.25
	} else {
		e.ref += (top - e.ref) * 0.02
	}
	e.ref = math.Max(e.ref, -75)
	rel := math.Exp(-dt * 6)
	for b := 0; b < NB; b++ {
		norm := clamp01((db[b] - (e.ref - 42)) / 42)
		e.smooth[b] = math.Max(norm, e.smooth[b]*rel)
	}
	bassE := mean(raw[:8])
	e.bassAvg += (bassE - e.bassAvg) * math.Min(1, dt*2.5)
	beat := bassE > e.bassAvg*1.45 && mean(e.smooth[:8]) > 0.35 && now-e.lastBeat > 0.24
	if beat {
		e.lastBeat = now
		e.beats++
	}
	f := Features{
		Bands:     e.smooth,
		Level:     clamp01(mean(e.smooth[:]) * 1.4),
		Bass:      mean(e.smooth[:10]),
		Mid:       mean(e.smooth[10:40]),
		Treble:    mean(e.smooth[40:]),
		Beat:      beat,
		SinceBeat: now - e.lastBeat,
		Beats:     e.beats,
		Wave:      buf[len(buf)-N:],
		Silent:    !e.heard && now-e.started > 3,
		Err:       errMsg,
	}
	e.mu.Lock()
	e.feat, e.haveFeat = f, true
	e.mu.Unlock()
	return f
}

func percentile(sorted []float64, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	pos := p / 100 * float64(len(sorted)-1)
	lo := int(math.Floor(pos))
	hi := int(math.Ceil(pos))
	return sorted[lo] + (sorted[hi]-sorted[lo])*(pos-float64(lo))
}

func mean(v []float64) float64 {
	if len(v) == 0 {
		return 0
	}
	s := 0.0
	for _, x := range v {
		s += x
	}
	return s / float64(len(v))
}

func clamp01(v float64) float64 { return math.Max(0, math.Min(1, v)) }

var (
	sharedMu   sync.Mutex
	shared     *Engine
	sharedTry  time.Time
	sharedLast time.Time // last Shared call: visualisers call it every frame
	sharedIdle sync.Once
)

// idleRelease is how long the microphone stays open after the last visualiser stops asking.
const idleRelease = 10 * time.Second

// Shared returns the process-wide engine, starting microphone capture on first use. If
// capture cannot start, the engine still works, reports Err in every analysis, and tries
// again every few seconds (permission may be granted, or a device plugged in, meanwhile).
func Shared() *Engine {
	sharedMu.Lock()
	defer sharedMu.Unlock()
	if shared == nil {
		shared = NewEngine(nil)
	}
	sharedLast = time.Now()
	sharedIdle.Do(func() { go releaseWhenIdle() })
	shared.mu.Lock()
	running, failed := shared.dev != nil, shared.errMsg != ""
	shared.mu.Unlock()
	if running || (failed && time.Since(sharedTry) < 5*time.Second) {
		return shared
	}
	sharedTry = time.Now()
	err := shared.Start()
	shared.mu.Lock()
	if err != nil {
		if shared.errMsg == "" {
			slog.Warn("audio capture failed", "err", err)
		}
		shared.errMsg = "no microphone: " + err.Error()
	} else {
		if failed {
			slog.Info("audio capture started")
		}
		shared.errMsg = ""
	}
	shared.mu.Unlock()
	return shared
}

// releaseWhenIdle closes the microphone once no visualiser has used it for idleRelease, so
// macOS stops showing it in use; the next Shared call opens it again.
func releaseWhenIdle() {
	for range time.Tick(2 * time.Second) {
		sharedMu.Lock()
		e, idle := shared, time.Since(sharedLast) > idleRelease
		sharedMu.Unlock()
		if e == nil || !idle {
			continue
		}
		e.mu.Lock()
		running := e.dev != nil
		e.mu.Unlock()
		if running {
			e.Stop()
			slog.Info("microphone released (no visualiser playing)")
		}
	}
}

// StopShared releases the microphone if the shared engine was ever started.
func StopShared() {
	sharedMu.Lock()
	defer sharedMu.Unlock()
	if shared != nil {
		shared.Stop()
	}
}
