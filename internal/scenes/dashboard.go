package scenes

import (
	"fmt"
	"math"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/host"
	"github.com/shirou/gopsutil/v4/load"
	"github.com/shirou/gopsutil/v4/mem"
	"github.com/shirou/gopsutil/v4/net"

	"github.com/justynroberts/marquee/internal/led"
)

func init() {
	Register("dashboard", "scene", "System dashboard: clock, CPU, memory, network and machine", func() Scene { return newDashboard() })
}

// The dashboard samples the system once a second; history covers about two minutes.
const dashHistory = 60

type dashboard struct {
	mu                   sync.Mutex
	busy                 bool
	lastSample           float64
	cpu                  float64
	cores                []float64
	load1, load5, load15 float64
	memPct               float64
	memUsed, memTotal    uint64
	swap                 uint64
	rx, tx               float64
	netAt                time.Time
	netRx, netTx         uint64
	cpuHist, rxHist      []float64
	txHist               []float64
	host                 string
	uptime               uint64
	procs                uint64
	masks                map[string]*led.Mask
}

func newDashboard() *dashboard {
	h, _ := os.Hostname()
	d := &dashboard{host: strings.ToUpper(strings.Split(h, ".")[0]), lastSample: -10, masks: map[string]*led.Mask{}}
	_, _ = cpu.Percent(0, true) // prime the counters
	return d
}

// sample refreshes the system figures at most once a second, off the draw path: gopsutil
// calls can take several milliseconds.
func (d *dashboard) sample(t float64) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.busy || t-d.lastSample < 1 {
		return
	}
	first := d.lastSample < 0
	d.lastSample = t
	d.busy = true
	go func() {
		d.collect(t, first)
		d.mu.Lock()
		d.busy = false
		d.mu.Unlock()
	}()
}

// collect reads the system; it holds the lock only while storing results.
func (d *dashboard) collect(t float64, first bool) {
	var (
		cores                 []float64
		l1, l5, l15           float64
		memPct                float64
		used, total, swap     uint64
		ioRecv, ioSent        uint64
		haveIO, haveVM, haveL bool
		haveSwap              bool
		uptime, procs         uint64
		haveUp, haveProcs     bool
	)
	cores, _ = cpu.Percent(0, true)
	if l, err := load.Avg(); err == nil {
		l1, l5, l15, haveL = l.Load1, l.Load5, l.Load15, true
	}
	if vm, err := mem.VirtualMemory(); err == nil {
		memPct, used, total, haveVM = vm.UsedPercent, vm.Total-vm.Available, vm.Total, true
	}
	if sw, err := mem.SwapMemory(); err == nil {
		swap, haveSwap = sw.Used, true
	}
	if io, err := net.IOCounters(false); err == nil && len(io) > 0 {
		ioRecv, ioSent, haveIO = io[0].BytesRecv, io[0].BytesSent, true
	}
	if first || int(t)%10 == 0 {
		if up, err := host.Uptime(); err == nil {
			uptime, haveUp = up, true
		}
		if info, err := host.Info(); err == nil {
			procs, haveProcs = info.Procs, true
		}
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if len(cores) > 0 {
		d.cores = cores
		s := 0.0
		for _, v := range cores {
			s += v
		}
		d.cpu = s / float64(len(cores))
	}
	if haveL {
		d.load1, d.load5, d.load15 = l1, l5, l15
	}
	if haveVM {
		d.memPct, d.memUsed, d.memTotal = memPct, used, total
	}
	if haveSwap {
		d.swap = swap
	}
	if haveIO {
		now := time.Now()
		if !d.netAt.IsZero() {
			if sec := now.Sub(d.netAt).Seconds(); sec > 0 {
				d.rx = float64(ioRecv-d.netRx) / sec
				d.tx = float64(ioSent-d.netTx) / sec
			}
		}
		d.netAt, d.netRx, d.netTx = now, ioRecv, ioSent
	}
	if haveUp {
		d.uptime = uptime
	}
	if haveProcs {
		d.procs = procs
	}
	// keep one history point every two seconds: ~2 minutes in 60 points
	if len(d.cpuHist) == 0 || int(t)%2 == 0 {
		d.cpuHist = push(d.cpuHist, d.cpu)
		d.rxHist = push(d.rxHist, d.rx)
		d.txHist = push(d.txHist, d.tx)
	}
}

func push(h []float64, v float64) []float64 {
	h = append(h, v)
	if len(h) > dashHistory {
		h = h[len(h)-dashHistory:]
	}
	return h
}

func humanBytes(n float64, perSec bool) string {
	units := []string{"B", "K", "M", "G", "T"}
	i := 0
	for n >= 1024 && i < len(units)-1 {
		n /= 1024
		i++
	}
	s := fmt.Sprintf("%.1f%s", n, units[i])
	if n >= 100 || i == 0 {
		s = fmt.Sprintf("%.0f%s", n, units[i])
	}
	if perSec {
		s += "/s"
	}
	return s
}

// trace draws a history line in box [x0,x1) x [y0,y1], newest at the right.
func trace(g *led.Grid, vals []float64, vmax float64, x0, y0, x1, y1 int, c led.RGB) {
	for x := x0; x < x1; x++ {
		g.Set(x, y1, rule)
	}
	if len(vals) < 2 || vmax <= 0 {
		return
	}
	w := x1 - x0
	prev := -1
	for i := 0; i < w; i++ {
		// sample the history across the box width
		pos := float64(i) / float64(w-1) * float64(dashHistory-1)
		idx := int(pos) - (dashHistory - len(vals))
		if idx < 0 {
			continue
		}
		v := math.Min(vals[idx], vmax) / vmax
		y := y1 - int(math.Round(v*float64(y1-y0)))
		if prev >= 0 {
			for yy := min(prev, y); yy <= max(prev, y); yy++ {
				g.Set(x0+i, yy, c)
			}
		} else {
			g.Set(x0+i, y, c)
		}
		prev = y
	}
}

func (d *dashboard) big(s string, size int) *led.Mask {
	k := fmt.Sprintf("%d|%s", size, s)
	if m, ok := d.masks[k]; ok {
		return m
	}
	if len(d.masks) > 64 {
		d.masks = map[string]*led.Mask{}
	}
	m := led.TextAt(s, "sans", size, 12)
	d.masks[k] = m
	return m
}

func vrule(g *led.Grid, x int) {
	for y := 3; y < rows-3; y++ {
		g.Set(x, y, rule)
	}
}

// Columns: clock | CPU | memory + network | machine.
const (
	dc1 = 72  // divider after the clock
	dc2 = 172 // after CPU
	dc3 = 224 // after memory
)

func (d *dashboard) Draw(g *led.Grid, t, dt float64) {
	d.sample(t)
	d.mu.Lock()
	defer d.mu.Unlock()
	now := time.Now()

	// clock
	label(g, strings.ToUpper(now.Format("Monday")), "mono", 6, 3, 3, dc1-5, ink3)
	m := d.big(now.Format("15:04"), 20)
	stampMask(g, m, 2, 0, dc1-4, ink)
	label(g, strings.ToUpper(now.Format("02 Jan 2006")), "mono", 7, 3, 37, dc1-5, ink2)
	_, week := now.ISOWeek()
	label(g, fmt.Sprintf(":%02d  W%02d", now.Second(), week), "mono", 7, 3, 50, dc1-5, ink3)
	vrule(g, dc1)

	// CPU
	x := dc1 + 4
	cw := dc2 - 4 - x
	hot := d.cpu >= 85
	label(g, fmt.Sprintf("CPU %dC LOAD %.2f", len(d.cores), d.load1), "mono", 6, x, 3, cw, ink3)
	col := ink
	if hot {
		col = alert
	}
	pct := d.big(fmt.Sprintf("%.0f%%", d.cpu), 20)
	stampMask(g, pct, x, 0, 44, col)
	trace(g, d.cpuHist, 100, x+46, 13, x+cw, 33, col)
	if n := len(d.cores); n > 0 {
		bw := max(1, (cw-(n-1))/n)
		for i, c := range d.cores {
			bx := x + i*(bw+1)
			if bx+bw > x+cw {
				break
			}
			h := int(math.Round(c / 100 * 22))
			g.Fill(bx, 40, bx+bw, 62, rule)
			bc := ink2
			if c >= 90 {
				bc = alert
			}
			g.Fill(bx, 62-h, bx+bw, 62, bc)
		}
	}
	vrule(g, dc2)

	// memory + network
	x = dc2 + 4
	mw := dc3 - 4 - x
	memHot := d.memPct >= 85
	mc := ink
	if memHot {
		mc = alert
	}
	label(g, "MEMORY", "mono", 6, x, 3, mw, ink3)
	stampMask(g, d.big(fmt.Sprintf("%.0f%%", d.memPct), 20), x, 0, mw, mc)
	label(g, humanBytes(float64(d.memUsed), false)+"/"+humanBytes(float64(d.memTotal), false), "mono", 6, x, 36, mw, ink2)
	bar := int(float64(mw) * d.memPct / 100)
	g.Fill(x, 45, x+mw, 47, rule)
	g.Fill(x, 45, x+bar, 47, mc)
	label(g, "D "+humanBytes(d.rx, true), "mono", 6, x, 51, mw, ink)
	label(g, "U "+humanBytes(d.tx, true), "mono", 6, x, 59, mw, ink2)
	vrule(g, dc3)

	// machine
	x = dc3 + 4
	hw := cols - 2 - x
	label(g, d.host, "mono", 6, x, 3, hw, ink3)
	nmax := 64 * 1024.0
	for _, v := range append(append([]float64{}, d.rxHist...), d.txHist...) {
		nmax = math.Max(nmax, v)
	}
	trace(g, d.txHist, nmax, x, 13, x+hw, 27, ink3)
	trace(g, d.rxHist, nmax, x, 13, x+hw, 27, ink)
	label(g, fmt.Sprintf("UP %dD%dH", d.uptime/86400, d.uptime%86400/3600), "mono", 6, x, 33, hw, ink2)
	label(g, fmt.Sprintf("%d PROC", d.procs), "mono", 6, x, 43, hw, ink3)
	label(g, "SW "+humanBytes(float64(d.swap), false), "mono", 6, x, 53, hw, ink3)
}
