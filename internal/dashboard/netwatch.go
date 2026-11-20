package dashboard

import (
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	"math"
	"net"
	"os/exec"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

// NetWatch is the internet's health from this Mac: every two seconds it times a TCP connection
// to Cloudflare (1.1.1.1) and Google (8.8.8.8) DNS, a ping to the home router, and a DNS
// lookup, and keeps five minutes of each. A verdict (ONLINE, SLOW, DOWN) is large on the left,
// each target's latency, range and loss with its trace on the right. Probing runs only while
// the screen is showing.
type NetWatch struct {
	// probe times one check: kind "tcp" dials addr (host:port), "gw" a router at addr (an IP),
	// "dns" looks addr up. Tests replace it.
	probe   func(kind, addr string) (time.Duration, error)
	gateway func() string

	mu      sync.Mutex
	targets []*nwTarget
	running bool
	seen    time.Time // last Frame
	rounds  int
	lastAt  time.Time
	gw      string
	gwAt    time.Time
	dnsN    int
	cache   frameCache
}

type nwTarget struct {
	name, kind, addr string
	bad              float64 // ms at which the colour is fully hot
	hist             []nwSample
}

type nwSample struct {
	at time.Time
	ms float64
	ok bool
}

const (
	nwEvery  = 2 * time.Second
	nwWindow = 5 * time.Minute
	nwIdle   = 15 * time.Second
)

var nwHosts = []string{"apple.com", "bbc.co.uk", "wikipedia.org", "github.com", "cloudflare.com", "google.com", "mozilla.org", "gov.uk"}

func NewNetWatch() *NetWatch {
	return &NetWatch{
		probe:   nwProbe,
		gateway: nwGateway,
		targets: []*nwTarget{
			{name: "CLOUDFLARE", kind: "tcp", addr: "1.1.1.1:443", bad: 200},
			{name: "GOOGLE", kind: "tcp", addr: "8.8.8.8:443", bad: 200},
			{name: "ROUTER", kind: "gw", bad: 40},
			{name: "DNS LOOKUP", kind: "dns", bad: 250},
		},
	}
}

// nwProbe is the real probe. A refused connection still proves the host answered, so it counts
// as reachable with the time it took.
func nwProbe(kind, addr string) (time.Duration, error) {
	start := time.Now()
	switch kind {
	case "dns":
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_, err := net.DefaultResolver.LookupHost(ctx, addr)
		return time.Since(start), err
	case "gw":
		// home routers often drop TCP from inside, so ask the router with ping
		return nwPing(addr)
	}
	return nwDial(addr)
}

var nwPingAvg = regexp.MustCompile(`= [0-9.]+/([0-9.]+)/`)

// nwPing sends one echo request with /sbin/ping (no privileges needed) and returns its round trip.
func nwPing(ip string) (time.Duration, error) {
	if net.ParseIP(ip) == nil {
		return 0, errors.New("not an address")
	}
	out, err := exec.Command("/sbin/ping", "-n", "-q", "-c", "1", "-W", "1500", ip).Output()
	if m := nwPingAvg.FindSubmatch(out); m != nil {
		ms, _ := strconv.ParseFloat(string(m[1]), 64)
		return time.Duration(ms * float64(time.Millisecond)), nil
	}
	if err == nil {
		err = errors.New("no reply")
	}
	return 0, err
}

func nwDial(addr string) (time.Duration, error) {
	start := time.Now()
	c, err := net.DialTimeout("tcp", addr, 1500*time.Millisecond)
	d := time.Since(start)
	if err == nil {
		c.Close()
		return d, nil
	}
	if errors.Is(err, syscall.ECONNREFUSED) {
		return d, nil
	}
	return d, err
}

// nwGateway is the default route's gateway ("" when there is none).
func nwGateway() string {
	if runtime.GOOS != "darwin" {
		return ""
	}
	out, err := exec.Command("/sbin/route", "-n", "get", "default").Output()
	if err != nil {
		return ""
	}
	for _, l := range strings.Split(string(out), "\n") {
		if v, ok := strings.CutPrefix(strings.TrimSpace(l), "gateway:"); ok {
			if ip := net.ParseIP(strings.TrimSpace(v)); ip != nil {
				return ip.String()
			}
		}
	}
	return ""
}

// round runs every probe once, at the same time, and records the results.
func (n *NetWatch) round(now time.Time) {
	n.mu.Lock()
	if n.gwAt.IsZero() || now.Sub(n.gwAt) > time.Minute {
		n.mu.Unlock()
		gw := n.gateway()
		n.mu.Lock()
		n.gw, n.gwAt = gw, now
	}
	host := nwHosts[n.dnsN%len(nwHosts)]
	n.dnsN++
	type job struct {
		t          *nwTarget
		kind, addr string
	}
	var jobs []job
	for _, t := range n.targets {
		addr := t.addr
		switch t.kind {
		case "gw":
			addr = n.gw
			t.addr = n.gw
		case "dns":
			addr = host
			t.addr = host
		}
		jobs = append(jobs, job{t, t.kind, addr})
	}
	n.mu.Unlock()

	res := make([]nwSample, len(jobs))
	var wg sync.WaitGroup
	for i, j := range jobs {
		if j.addr == "" {
			continue // no router known
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			d, err := n.probe(j.kind, j.addr)
			res[i] = nwSample{at: now, ms: float64(d) / float64(time.Millisecond), ok: err == nil}
		}()
	}
	wg.Wait()

	n.mu.Lock()
	defer n.mu.Unlock()
	for i, j := range jobs {
		if j.addr == "" {
			continue
		}
		h := append(j.t.hist, res[i])
		for len(h) > 0 && now.Sub(h[0].at) > nwWindow {
			h = h[1:]
		}
		j.t.hist = h
	}
	n.rounds++
	n.lastAt = now
}

func (n *NetWatch) loop() {
	tick := time.NewTicker(nwEvery)
	defer tick.Stop()
	for {
		n.round(time.Now())
		<-tick.C
		n.mu.Lock()
		idle := time.Since(n.seen) > nwIdle
		if idle {
			n.running = false
		}
		n.mu.Unlock()
		if idle {
			return
		}
	}
}

type nwStats struct {
	cur, lo, avg, hi, loss float64
	curOK, any             bool
}

func (t *nwTarget) stats() nwStats {
	var s nwStats
	s.lo = math.Inf(1)
	ok, sum := 0, 0.0
	for _, v := range t.hist {
		if v.ok {
			ok++
			sum += v.ms
			s.lo, s.hi = math.Min(s.lo, v.ms), math.Max(s.hi, v.ms)
		}
	}
	if len(t.hist) == 0 {
		return nwStats{}
	}
	s.any = true
	last := t.hist[len(t.hist)-1]
	s.cur, s.curOK = last.ms, last.ok
	if ok > 0 {
		s.avg = sum / float64(ok)
	} else {
		s.lo = 0
	}
	s.loss = 100 * float64(len(t.hist)-ok) / float64(len(t.hist))
	return s
}

// verdict looks at the last few rounds: DOWN when neither Cloudflare nor Google answered the
// last two, SLOW when they are slow or dropping, otherwise ONLINE.
func (n *NetWatch) verdict() (string, string, color.RGBA) {
	var recent []nwSample
	fails := 0
	for _, t := range n.targets[:2] {
		h := t.hist
		if len(h) > 15 {
			h = h[len(h)-15:]
		}
		recent = append(recent, h...)
		if len(t.hist) >= 2 && !t.hist[len(t.hist)-1].ok && !t.hist[len(t.hist)-2].ok {
			fails++
		}
	}
	if len(recent) == 0 {
		return "CHECKING", "FIRST RESULTS IN A MOMENT", txt3
	}
	if fails == 2 {
		why := "NO ANSWER FROM THE INTERNET"
		if r := n.targets[2]; len(r.hist) > 0 && !r.hist[len(r.hist)-1].ok {
			why = "THE ROUTER IS NOT ANSWERING EITHER"
		} else if len(r.hist) > 0 {
			why = "THE ROUTER ANSWERS; THE LINE BEYOND IT DOES NOT"
		}
		return "DOWN", why, hot
	}
	ok, sum := 0, 0.0
	for _, s := range recent {
		if s.ok {
			ok++
			sum += s.ms
		}
	}
	loss := 1 - float64(ok)/float64(len(recent))
	avg := 0.0
	if ok > 0 {
		avg = sum / float64(ok)
	}
	switch {
	case loss > 0.1:
		return "SLOW", fmt.Sprintf("%.0f%% OF CHECKS LOST IN THE LAST 30 S", 100*loss), amber
	case avg > 150:
		return "SLOW", fmt.Sprintf("%.0f MS ON AVERAGE IN THE LAST 30 S", avg), amber
	}
	return "ONLINE", fmt.Sprintf("%.0f MS ON AVERAGE IN THE LAST 30 S", avg), green
}

func (n *NetWatch) Frame(t, dt float64, now time.Time) *image.RGBA {
	n.mu.Lock()
	n.seen = time.Now()
	if !n.running && n.probe != nil {
		n.running = true
		go n.loop()
	}
	key := fmt.Sprint(n.rounds, now.Format("15:04:05"))
	img := n.cache.get(key, func(img *image.RGBA) { n.render(img, now) })
	n.mu.Unlock()
	return img
}

const (
	nwX1     = 560  // targets column
	nwStatX  = 900  // min / avg / max
	nwTraceX = 1170 // traces
	nwRowY   = 30
	nwRowH   = 102
)

func (n *NetWatch) render(img *image.RGBA, now time.Time) {
	head(img, pad, pad-4, "internet", nwX1-pad-40)
	word, why, c := n.verdict()
	text(img, pad, 80, word, fitPx(word, 120, nwX1-pad-70, 130), c)
	for i, l := range wrap(why, 22, float64(nwX1-pad-60), 2) {
		text(img, pad, 236+i*30, l, 22, txt)
	}
	gw := n.gw
	if gw == "" {
		gw = "NOT FOUND"
	}
	text(img, pad, 330, "ROUTER  "+gw, 20, txt2)
	if !n.lastAt.IsZero() {
		text(img, pad, 362, "CHECKED "+n.lastAt.In(now.Location()).Format("15:04:05")+"  EVERY 2 S", 20, txt3)
	}
	text(img, pad, 394, "LAST 5 MINUTES", 20, txt3)
	vrule(img, nwX1-30)

	for i, t := range n.targets {
		y := nwRowY + i*nwRowH
		s := t.stats()
		text(img, nwX1, y, t.name, 22, cyan)
		addr := strings.TrimSuffix(t.addr, ":443")
		if addr == "" {
			addr = "—"
		}
		text(img, nwX1, y+28, fit(addr, 16, nwStatX-nwX1-20), 16, txt3)
		switch {
		case !s.any:
			text(img, nwX1, y+50, "…", 34, txt3)
		case !s.curOK:
			text(img, nwX1, y+50, "NO REPLY", 34, hot)
		default:
			text(img, nwX1, y+50, fmt.Sprintf("%.0f MS", s.cur), 34, ramp(s.cur/t.bad))
		}
		if s.any {
			text(img, nwStatX, y+4, fmt.Sprintf("MIN %5.0f", s.lo), 20, txt2)
			text(img, nwStatX, y+30, fmt.Sprintf("AVG %5.0f", s.avg), 20, txt)
			text(img, nwStatX, y+56, fmt.Sprintf("MAX %5.0f", s.hi), 20, txt2)
			lc := green
			if s.loss > 0 {
				lc = ramp(s.loss / 20)
			}
			text(img, nwStatX+150, y+30, fmt.Sprintf("LOSS %.0f%%", s.loss), 20, lc)
		}
		nwTrace(img, nwTraceX, y+6, W-pad, y+nwRowH-18, t, now)
	}
}

// nwTrace plots five minutes of samples, newest at the right, coloured by how slow each is;
// lost checks are hot ticks on the baseline.
func nwTrace(img *image.RGBA, x0, y0, x1, y1 int, t *nwTarget, now time.Time) {
	rect(img, x0, y1, x1, y1, line)
	top := 20.0
	for _, s := range t.hist {
		if s.ok {
			top = math.Max(top, s.ms)
		}
	}
	top = nwNice(top * 1.15)
	l := fmt.Sprintf("%.0f MS", top)
	text(img, float64(x1)-measure(l, 14), y0-4, l, 14, txt3)
	xs := func(at time.Time) float64 {
		return float64(x1) - float64(now.Sub(at))/float64(nwWindow)*float64(x1-x0)
	}
	ys := func(ms float64) float64 { return float64(y1) - math.Min(ms, top)/top*float64(y1-y0) }
	var prev *nwSample
	for i := range t.hist {
		s := &t.hist[i]
		x := xs(s.at)
		if x < float64(x0) {
			continue
		}
		if !s.ok {
			rect(img, int(x)-1, y1-10, int(x)+1, y1, hot)
			prev = nil
			continue
		}
		c := ramp(s.ms / t.bad)
		if prev != nil {
			wideLine(img, int(xs(prev.at)), int(ys(prev.ms)), int(x), int(ys(s.ms)), c, 3)
		} else {
			disc(img, x, ys(s.ms), 2, c)
		}
		prev = s
	}
}

// nwNice rounds a scale up to 1, 2 or 5 times a power of ten.
func nwNice(v float64) float64 {
	p := math.Pow(10, math.Floor(math.Log10(v)))
	for _, m := range []float64{1, 2, 5, 10} {
		if m*p >= v {
			return m * p
		}
	}
	return 10 * p
}
