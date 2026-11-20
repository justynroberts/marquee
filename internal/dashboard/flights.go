package dashboard

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/justynroberts/marquee/internal/feeds"
)

// Flights is the sky overhead: a radar of the aircraft within 40 nautical miles on the left,
// moved on between fetches by their own speed and track, and the nearest listed on the right.
// Planes squawking an emergency take the accent.
type Flights struct {
	place  func() string
	locate func(string) (feeds.Place, error)
	fetch  func(lat, lon float64, nm int) ([]feeds.Aircraft, error)

	poll  poller[flightData]
	geoMu sync.Mutex
	geo   map[string]feeds.Place
	cache frameCache
}

type flightData struct {
	home feeds.Place
	ac   []feeds.Aircraft
}

const (
	flRange = 40 // nautical miles
	flR     = 192
	flCX    = pad + flR + 8
	flCY    = H/2 + 4
	flTab   = flCX + flR + 70 // table's left edge
)

func NewFlights(place func() string) *Flights {
	return &Flights{place: place, locate: feeds.Geocode, fetch: feeds.FetchAircraft,
		poll: poller[flightData]{every: 15 * time.Second}, geo: map[string]feeds.Place{}}
}

func (f *Flights) load(name string) (flightData, error) {
	f.geoMu.Lock()
	home, ok := f.geo[name]
	f.geoMu.Unlock()
	if !ok {
		var err error
		if home, err = f.locate(name); err != nil {
			return flightData{}, err
		}
		f.geoMu.Lock()
		f.geo[name] = home
		f.geoMu.Unlock()
	}
	ac, err := f.fetch(home.Latitude, home.Longitude, flRange)
	return flightData{home, ac}, err
}

// plane is an aircraft moved on to now, in nautical miles east and north of home.
type plane struct {
	feeds.Aircraft
	x, y float64
}

func (p plane) bearing() float64 {
	b := math.Atan2(p.x, p.y) * 180 / math.Pi
	if b < 0 {
		b += 360
	}
	return b
}

// flRanges are the radar's zoom steps; it uses the smallest that still shows flBusy planes.
var flRanges = []float64{10, 15, 20, 30, 40}

const flBusy = 35

func pickRange(ps []plane) float64 {
	for _, r := range flRanges {
		n := 0
		for _, p := range ps {
			if p.Dist <= r {
				n++
			}
		}
		if n >= flBusy || n == len(ps) {
			return r
		}
	}
	return flRange
}

func project(d flightData, since float64) []plane {
	k := math.Cos(d.home.Latitude * math.Pi / 180)
	var out []plane
	for _, a := range d.ac {
		p := plane{Aircraft: a, x: (a.Lon - d.home.Longitude) * 60 * k, y: (a.Lat - d.home.Latitude) * 60}
		if !a.Ground {
			run := a.Speed * since / 3600
			p.x += run * math.Sin(a.Track*math.Pi/180)
			p.y += run * math.Cos(a.Track*math.Pi/180)
		}
		p.Dist = math.Hypot(p.x, p.y)
		if p.Dist <= flRange {
			out = append(out, p)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Dist < out[j].Dist })
	return out
}

func (f *Flights) Frame(t, dt float64, now time.Time) *image.RGBA {
	d, got, ok, err := f.poll.get(f.place(), f.load)
	if !ok {
		return waitMessage(&f.cache, "AIRCRAFT", err)
	}
	since := time.Since(got).Seconds()
	ps := project(d, since)
	rng := pickRange(ps)
	img := f.cache.get(fmt.Sprint(got.UnixNano(), now.Unix()), func(img *image.RGBA) { f.render(img, d.home, ps, rng) })
	// sweep: one turn every four seconds, a fading wedge behind the line
	a0 := math.Mod(t/4, 1) * 2 * math.Pi
	for i := 0; i < 12; i++ {
		a := a0 - float64(i)*0.03
		c := mix(bg, green, (1-float64(i)/12)*0.7)
		wideLine(img, flCX, flCY, flCX+int(flR*math.Sin(a)), flCY-int(flR*math.Cos(a)), c, 2)
	}
	// aircraft, on top of the sweep; nearest first get labels, skipping any that would overlap
	var taken []image.Rectangle
	for _, p := range ps {
		if p.Dist > rng {
			break // nearest first
		}
		sx := float64(flCX) + p.x/rng*flR
		sy := float64(flCY) - p.y/rng*flR
		c := altColour(p.Aircraft)
		tr := p.Track * math.Pi / 180
		wideLine(img, int(sx), int(sy), int(sx+16*math.Sin(tr)), int(sy-16*math.Cos(tr)), c, 2)
		disc(img, sx, sy, 4.5, c)
		lb := image.Rect(int(sx)+8, int(sy)+2, int(sx)+12+int(measure(p.Callsign, 16)), int(sy)+22)
		free := len(taken) < 12
		for _, r := range taken {
			if r.Overlaps(lb) {
				free = false
				break
			}
		}
		if free {
			taken = append(taken, lb)
			text(img, sx+9, int(sy)+4, p.Callsign, 16, txt)
		}
	}
	return img
}

// altColour runs green near the ground through cyan to violet at cruising height; an
// emergency is vermilion.
func altColour(a feeds.Aircraft) color.RGBA {
	switch {
	case a.Emergency:
		return hot
	case a.Ground:
		return txt3
	}
	k := math.Min(1, float64(a.Alt)/38000)
	if k < 0.5 {
		return mix(green, cyan, k*2)
	}
	return mix(cyan, violet, (k-0.5)*2)
}

var radarRing = color.RGBA{38, 84, 52, 255}

func (f *Flights) render(img *image.RGBA, home feeds.Place, ps []plane, rng float64) {
	// radar rings, cross and north
	for r := 1; r <= 4; r++ {
		ring(img, flCX, flCY, float64(flR*r)/4, radarRing)
	}
	rect(img, flCX-flR, flCY, flCX+flR, flCY, radarRing)
	rect(img, flCX, flCY-flR, flCX, flCY+flR, radarRing)
	text(img, flCX-measure("N", 18)/2, flCY-flR-26, "N", 18, txt3)
	lbl := fmt.Sprintf("%.0fNM", rng)
	text(img, flCX+flR+6, flCY+4, lbl, 16, txt3)
	disc(img, flCX, flCY, 3, txt2)

	x := flTab
	head(img, x, pad-4, fmt.Sprintf("overhead  %s  within %d nm", home.Name, flRange), 900)
	var air []plane
	for _, p := range ps {
		if !p.Ground {
			air = append(air, p)
		}
	}
	n := fmt.Sprintf("%d AIRBORNE", len(air))
	if len(air) < len(ps) {
		n += fmt.Sprintf("   %d ON THE GROUND", len(ps)-len(air))
	}
	text(img, float64(W-pad)-measure(n, 22), pad-4, n, 22, txt3)
	cols := []struct {
		name  string
		right int // right edge, or 0 for left-aligned at at
		at    int
	}{{"CALLSIGN", 0, 0}, {"TYPE", 0, 230}, {"REG", 0, 370}, {"ALT FT", 720, 0}, {"KT", 850, 0}, {"NM", 980, 0}, {"BRG", 1100, 0}}
	hy := 84
	for _, c := range cols {
		if c.right > 0 {
			text(img, float64(x+c.right)-measure(c.name, 18), hy, c.name, 18, txt3)
		} else {
			text(img, float64(x+c.at), hy, c.name, 18, txt3)
		}
	}
	barX := x + 1140
	text(img, float64(barX), hy, "ALTITUDE", 18, txt3)
	rect(img, x, hy+30, W-pad, hy+30, line)
	if len(air) == 0 {
		text(img, float64(x), hy+60, "CLEAR SKIES: NOTHING IN RANGE", 26, txt3)
	}
	for i, p := range air {
		if i >= 8 {
			break
		}
		y := hy + 50 + i*38
		ac := altColour(p.Aircraft)
		colours := []color.RGBA{cyan, txt2, txt3, ac, txt, amber, txt2}
		if p.Emergency {
			colours = []color.RGBA{hot, hot, hot, hot, hot, hot, hot}
		}
		alt := fmt.Sprintf("%d", p.Alt)
		if p.Ground {
			alt = "GND"
		}
		cells := []string{p.Callsign, p.Type, p.Reg, alt, fmt.Sprintf("%.0f", p.Speed), fmt.Sprintf("%.1f", p.Dist), fmt.Sprintf("%03.0f", p.bearing())}
		for j, s := range cells {
			cl := cols[j]
			s = strings.ToUpper(s)
			if cl.right > 0 {
				text(img, float64(x+cl.right)-measure(s, 26), y, s, 26, colours[j])
			} else {
				text(img, float64(x+cl.at), y, fit(s, 26, 200), 26, colours[j])
			}
		}
		if p.Emergency {
			text(img, float64(barX), y, "SQUAWK "+p.Squawk, 26, hot)
			continue
		}
		bw := W - pad - barX
		rect(img, barX, y+10, barX+bw, y+12, line)
		if !p.Ground {
			rect(img, barX, y+8, barX+int(float64(bw)*math.Min(1, float64(p.Alt)/45000)), y+14, ac)
		}
	}
}

// ------------------------------------------------------------------ shared shapes

func mix(a, b color.RGBA, k float64) color.RGBA {
	l := func(x, y uint8) uint8 { return uint8(float64(x) + (float64(y)-float64(x))*k) }
	return color.RGBA{l(a.R, b.R), l(a.G, b.G), l(a.B, b.B), 255}
}

// disc fills a circle with a soft edge.
func disc(img *image.RGBA, cx, cy, r float64, c color.RGBA) {
	b := img.Bounds()
	for y := int(cy - r - 1); y <= int(cy+r+1); y++ {
		for x := int(cx - r - 1); x <= int(cx+r+1); x++ {
			if x < 0 || y < 0 || x >= b.Max.X || y >= b.Max.Y {
				continue
			}
			cov := r + 0.5 - math.Hypot(float64(x)+0.5-cx, float64(y)+0.5-cy)
			if cov <= 0 {
				continue
			}
			a := uint32(math.Min(1, cov) * 255)
			o := img.PixOffset(x, y)
			img.Pix[o] = blend(a, img.Pix[o], c.R)
			img.Pix[o+1] = blend(a, img.Pix[o+1], c.G)
			img.Pix[o+2] = blend(a, img.Pix[o+2], c.B)
		}
	}
}

// ring draws a circle outline 2px wide.
func ring(img *image.RGBA, cx, cy int, r float64, c color.RGBA) {
	n := max(24, int(r*0.8))
	px, py := cx+int(r), cy
	for i := 1; i <= n; i++ {
		a := 2 * math.Pi * float64(i) / float64(n)
		x, y := cx+int(math.Round(r*math.Cos(a))), cy+int(math.Round(r*math.Sin(a)))
		wideLine(img, px, py, x, y, c, 2)
		px, py = x, y
	}
}
