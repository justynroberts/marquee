// Package dashboard draws the live system dashboard at the panel's native 1920x462 resolution
// (not the LED dot grid), so small text stays sharp.
//
// Layout, left to right, separated by hairline rules:
// clock | CPU (big %, 2-minute trace, per-core bars) | memory + network | machine.
// Near-black ground, paper ink, one vermilion accent only when something is hot.
package dashboard

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/host"
	"github.com/shirou/gopsutil/v4/load"
	"github.com/shirou/gopsutil/v4/mem"
	"github.com/shirou/gopsutil/v4/net"
	"github.com/shirou/gopsutil/v4/process"

	"github.com/justynroberts/marquee/internal/ft"
)

const (
	W, H    = 1920, 462
	history = 60 // samples at 2 s = 2 minutes
	pad     = 36
	c1, c2  = 545, 1225
	c3      = 1572
)

var (
	bg   = color.RGBA{12, 13, 11, 255}
	ink  = color.RGBA{230, 226, 214, 255}
	ink2 = color.RGBA{161, 157, 144, 255}
	ink3 = color.RGBA{96, 93, 85, 255}
	rule = color.RGBA{44, 44, 38, 255}
	hot  = color.RGBA{255, 91, 56, 255}
)

// ---------------------------------------------------------------- fonts

// Text goes through FreeType + HarfBuzz exactly as Pillow draws it (internal/ft), in SF Mono like
// dashboard.py, falling back to Menlo and then DejaVu Sans Mono.
var monoPaths = []struct {
	path  string
	index int
}{{"/System/Library/Fonts/SFNSMono.ttf", 0}, {"/System/Library/Fonts/Menlo.ttc", 0}, {"/usr/share/fonts/truetype/dejavu/DejaVuSansMono.ttf", 0}}

func face(px int) *ft.Face {
	for _, p := range monoPaths {
		if f, err := ft.Open(p.path, p.index, px); err == nil {
			return f
		}
	}
	return nil
}

// text is ImageDraw.text((x, y), s, font=font(px), fill=c) on an RGB image; returns the
// advance (ImageDraw.textlength) so callers can chain positions like dashboard.py.
func text(dst *image.RGBA, x float64, y int, s string, px int, c color.RGBA) float64 {
	f := face(px)
	if f == nil {
		return 0
	}
	ix := int(x)
	m, ox, oy := f.Mask(s, x-float64(ix))
	b := dst.Bounds()
	for j := 0; j < m.Rect.Dy(); j++ {
		for i := 0; i < m.Rect.Dx(); i++ {
			a := uint32(m.Pix[j*m.Stride+i])
			if a == 0 {
				continue
			}
			X, Y := ix+ox+i, y+oy+j
			if X < b.Min.X || X >= b.Max.X || Y < b.Min.Y || Y >= b.Max.Y {
				continue
			}
			o := dst.PixOffset(X, Y)
			// Pillow fill_mask_L: BLEND(mask, out, ink) per band.
			dst.Pix[o] = blend(a, dst.Pix[o], c.R)
			dst.Pix[o+1] = blend(a, dst.Pix[o+1], c.G)
			dst.Pix[o+2] = blend(a, dst.Pix[o+2], c.B)
		}
	}
	return f.Length(s)
}

func blend(m uint32, in1, in2 uint8) uint8 {
	t := uint32(in1)*(255-m) + uint32(in2)*m + 128
	return uint8(((t >> 8) + t) >> 8)
}

// measure is ImageDraw.textlength.
func measure(s string, px int) float64 {
	if f := face(px); f != nil {
		return f.Length(s)
	}
	return 0
}

func fit(s string, px int, width float64) string {
	if measure(s, px) <= width {
		return s
	}
	r := []rune(s)
	for len(r) > 0 && measure(string(r)+"…", px) > width {
		r = r[:len(r)-1]
	}
	return string(r) + "…"
}

func caps(dst *image.RGBA, x, y int, s string, width int) {
	text(dst, float64(x), y, fit(strings.ToUpper(s), 22, float64(width)), 22, ink3)
}

// rect fills like Pillow's ImageDraw.rectangle: both corners inclusive.
func rect(dst *image.RGBA, x0, y0, x1, y1 int, c color.Color) {
	draw.Draw(dst, image.Rect(x0, y0, x1+1, y1+1), image.NewUniform(c), image.Point{}, draw.Src)
}

func fill(dst *image.RGBA, c color.Color) {
	draw.Draw(dst, dst.Bounds(), image.NewUniform(c), image.Point{}, draw.Src)
}

// vline is d.line([(x, pad), (x, H - pad)], fill=RULE, width=2).
func vline(dst *image.RGBA, x int) { wideLine(dst, x, pad, x, H-pad, rule, 2) }

func bar(dst *image.RGBA, x0, y0, x1, y1 int, frac float64, c color.Color) {
	rect(dst, x0, y0, x1, y1, rule)
	w := int(float64(x1-x0) * math.Max(0, math.Min(frac, 1)))
	if w > 0 {
		rect(dst, x0, y0, x0+w, y1, c)
	}
}

// trace is dashboard.py's trace(): a baseline rule and a 3 px stepped line, newest sample at the
// right edge, drawn with Pillow's line algorithm.
func trace(dst *image.RGBA, x0, y0, x1, y1 int, values []float64, vmax float64, c color.RGBA) {
	rect(dst, x0, y1, x1, y1, rule)
	if len(values) < 2 || vmax <= 0 {
		return
	}
	step := float64(x1-x0) / (history - 1)
	start := history - len(values)
	pts := make([][2]float64, len(values))
	for i, v := range values {
		pts[i] = [2]float64{float64(x0) + float64(start+i)*step, float64(y1) - math.Min(v, vmax)/vmax*float64(y1-y0)}
	}
	polyline(dst, pts, c, 3)
}

func human(n float64, perS bool) string {
	units := []string{"B", "KB", "MB", "GB", "TB"}
	i := 0
	for n >= 1024 && i < len(units)-1 {
		n /= 1024
		i++
	}
	s := fmt.Sprintf("%.1f", n)
	if n >= 100 || i == 0 {
		s = fmt.Sprintf("%.0f", n)
	}
	if perS {
		return s + " " + units[i] + "/s"
	}
	return s + " " + units[i]
}

// ---------------------------------------------------------------- sampling

type sample struct {
	cpu            float64
	cores          []float64
	load           [3]float64
	memPct         float64
	memUsed, memT  float64
	swap           float64
	rx, tx         float64
	uptime         time.Duration
	procs          int
	top            string
	batt           *battery
	cpuH, rxH, txH []float64
}

type battery struct {
	percent float64
	plugged bool
}

// readBattery parses pmset (macOS); nil when there is no battery, like psutil.
func readBattery() *battery {
	out, err := exec.Command("pmset", "-g", "batt").Output()
	if err != nil {
		return nil
	}
	text := string(out)
	i := strings.Index(text, "InternalBattery")
	if i < 0 {
		return nil
	}
	var pct float64
	rest := text[i:]
	if j := strings.Index(rest, "%"); j > 0 {
		k := j
		for k > 0 && (rest[k-1] >= '0' && rest[k-1] <= '9' || rest[k-1] == '.') {
			k--
		}
		fmt.Sscanf(rest[k:j], "%g", &pct)
	}
	return &battery{percent: pct, plugged: strings.Contains(text, "'AC Power'")}
}

// Dashboard samples the machine every 2 s in the background and renders on demand.
type Dashboard struct {
	mu    sync.Mutex
	s     sample
	host  string
	osVer string
	stop  chan struct{}
}

// New starts sampling. Call Close to stop.
func New() *Dashboard {
	d := &Dashboard{stop: make(chan struct{})}
	d.host, _ = os.Hostname()
	d.host, _, _ = strings.Cut(d.host, ".")
	if info, err := host.Info(); err == nil {
		d.osVer = info.Platform + " " + info.PlatformVersion
		if runtime.GOOS == "darwin" {
			d.osVer = "macOS " + info.PlatformVersion
		}
	}
	go d.loop()
	return d
}

// Close stops sampling.
func (d *Dashboard) Close() {
	select {
	case <-d.stop:
	default:
		close(d.stop)
	}
}

func push(h []float64, v float64) []float64 {
	h = append(h, v)
	if len(h) > history {
		h = h[len(h)-history:]
	}
	return h
}

func (d *Dashboard) loop() {
	_, _ = cpu.Percent(0, true) // prime
	lastNet, _ := net.IOCounters(false)
	lastT := time.Now()
	tick := time.NewTicker(2 * time.Second)
	defer tick.Stop()
	first := time.After(300 * time.Millisecond)
	for {
		select {
		case <-d.stop:
			return
		case <-first:
		case <-tick.C:
		}
		var s sample
		s.cores, _ = cpu.Percent(0, true)
		for _, c := range s.cores {
			s.cpu += c
		}
		if len(s.cores) > 0 {
			s.cpu /= float64(len(s.cores))
		}
		if l, err := load.Avg(); err == nil {
			s.load = [3]float64{l.Load1, l.Load5, l.Load15}
		}
		if vm, err := mem.VirtualMemory(); err == nil {
			s.memPct, s.memUsed, s.memT = vm.UsedPercent, float64(vm.Total-vm.Available), float64(vm.Total)
		}
		if sw, err := mem.SwapMemory(); err == nil {
			s.swap = float64(sw.Used)
		}
		now := time.Now()
		if n, err := net.IOCounters(false); err == nil && len(n) > 0 && len(lastNet) > 0 {
			dt := math.Max(now.Sub(lastT).Seconds(), 1e-3)
			s.rx = float64(n[0].BytesRecv-lastNet[0].BytesRecv) / dt
			s.tx = float64(n[0].BytesSent-lastNet[0].BytesSent) / dt
			lastNet = n
		}
		lastT = now
		if bt, err := host.BootTime(); err == nil {
			s.uptime = time.Since(time.Unix(int64(bt), 0))
		}
		if pids, err := process.Pids(); err == nil {
			s.procs = len(pids)
		}
		s.top = busiest()
		s.batt = readBattery()
		d.mu.Lock()
		s.cpuH = push(d.s.cpuH, s.cpu)
		s.rxH = push(d.s.rxH, s.rx)
		s.txH = push(d.s.txH, s.tx)
		d.s = s
		d.mu.Unlock()
	}
}

// busiest names the process using the most CPU (ps is far cheaper than walking every process).
func busiest() string {
	out, err := exec.Command("ps", "-Ao", "%cpu=,comm=", "-r").Output()
	if err != nil {
		return "?"
	}
	line, _, _ := strings.Cut(strings.TrimSpace(string(out)), "\n")
	f := strings.Fields(line)
	if len(f) < 2 {
		return "?"
	}
	return filepath.Base(strings.Join(f[1:], " "))
}

// ---------------------------------------------------------------- render

// Render draws one frame.
func (d *Dashboard) Render() *image.RGBA {
	d.mu.Lock()
	s := d.s
	d.mu.Unlock()
	return d.renderAt(s, time.Now())
}

func (d *Dashboard) renderAt(s sample, now time.Time) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, W, H))
	fill(img, bg)
	pick := func(isHot bool) color.RGBA {
		if isHot {
			return hot
		}
		return ink
	}

	// clock
	x := pad
	caps(img, x, pad, now.Format("Monday"), c1-2*pad)
	text(img, float64(x-6), 66, now.Format("15:04"), 150, ink)
	text(img, float64(x), 240, now.Format("02 January 2006"), 38, ink2)
	text(img, float64(x), 292, now.Format(":05"), 38, ink3)
	_, week := now.ISOWeek()
	text(img, float64(x), H-pad-30, fmt.Sprintf("WEEK %02d  ·  DAY %03d", week, now.YearDay()), 26, ink3)
	vline(img, c1)

	// CPU
	x = c1 + pad
	cw := c2 - pad - x
	cpuHot := s.cpu >= 85
	caps(img, x, pad, fmt.Sprintf("CPU  ·  %d cores  ·  load %.2f %.2f %.2f", len(s.cores), s.load[0], s.load[1], s.load[2]), cw)
	num := fmt.Sprintf("%.0f", s.cpu)
	nw := text(img, float64(x-4), 66, num, 150, pick(cpuHot))
	text(img, float64(x)+nw+4, 152, "%", 56, ink2)
	trace(img, x+300, 90, x+cw, 230, s.cpuH, 100, pick(cpuHot))
	text(img, float64(x+300), 238, "last 2 min", 20, ink3)
	if n := len(s.cores); n > 0 {
		by := 290
		bw := (float64(cw) - 6*float64(n-1)) / float64(n)
		for i, c := range s.cores {
			// Pillow truncates each float coordinate.
			bx0 := float64(x) + float64(i)*(bw+6)
			bx, bx1 := int(bx0), int(bx0+bw)
			top := int(float64(by) + (1-c/100)*110)
			rect(img, bx, by, bx1, by+110, rule)
			col := ink2
			if c >= 90 {
				col = hot
			}
			rect(img, bx, top, bx1, by+110, col)
		}
	}
	caps(img, x, H-pad-22, "busiest: "+s.top, cw)
	vline(img, c2)

	// memory + network
	x = c2 + pad
	mw := c3 - pad - x
	memHot := s.memPct >= 85
	caps(img, x, pad, "Memory", mw)
	mnum := fmt.Sprintf("%.0f", s.memPct)
	w := text(img, float64(x-3), 66, mnum, 110, pick(memHot))
	text(img, float64(x)+w+4, 128, "%", 44, ink2)
	text(img, float64(x), 200, fit(human(s.memUsed, false)+" / "+human(s.memT, false), 26, float64(mw)), 26, ink2)
	bar(img, x, 244, x+mw, 254, s.memPct/100, pick(memHot))
	text(img, float64(x), 270, "swap "+human(s.swap, false), 24, ink3)
	caps(img, x, 320, "Network", mw)
	text(img, float64(x), 350, "↓ "+human(s.rx, true), 28, ink)
	text(img, float64(x), 386, "↑ "+human(s.tx, true), 28, ink2)
	vline(img, c3)

	// network trace + machine
	x = c3 + pad
	w4 := W - pad - x
	caps(img, x, pad, d.host, w4)
	rmax := 64.0 * 1024
	for _, v := range append(append([]float64{}, s.rxH...), s.txH...) {
		rmax = math.Max(rmax, v)
	}
	trace(img, x, 74, W-pad, 160, s.rxH, rmax, ink)
	trace(img, x, 74, W-pad, 160, s.txH, rmax, ink3)
	text(img, float64(x), 168, "network ↓ light  ↑ dim", 20, ink3)
	text(img, float64(x), 214, fit(d.osVer, 26, float64(w4)), 26, ink2)
	up := s.uptime
	text(img, float64(x), 250, fmt.Sprintf("up %dd %dh %dm", int(up.Hours())/24, int(up.Hours())%24, int(up.Minutes())%60), 26, ink2)
	text(img, float64(x), 286, fmt.Sprintf("%d processes", s.procs), 26, ink3)
	if b := s.batt; b != nil {
		low := b.percent < 15 && !b.plugged
		state := "battery"
		if b.plugged {
			state = "charging"
		}
		pct := fmt.Sprintf("%.0f%%", b.percent)
		col := ink
		if low {
			col = hot
		}
		text(img, float64(x), 334, pct, 56, col)
		text(img, float64(x)+measure(pct, 56)+16, 358, state, 24, ink3)
		barCol := ink2
		if low {
			barCol = hot
		}
		bar(img, x, 412, W-pad, 420, b.percent/100, barCol)
	}
	return img
}
