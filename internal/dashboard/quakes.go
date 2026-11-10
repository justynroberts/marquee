package dashboard

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"strings"
	"time"

	"github.com/justynroberts/marquee/internal/feeds"
)

// Quakes is the planet's earthquakes from the USGS: the last 30 days' 4.5+ faintly and the last
// day's 2.5+ brightly on a world map (between them they trace the plate boundaries, no
// coastline needed), and the latest listed on the right. Magnitude 6+ takes the accent; a quake
// in the last hour pulses.
type Quakes struct {
	fetch func() (day, week []feeds.Quake, err error)
	poll  poller[quakeData]
	cache frameCache
}

type quakeData struct{ day, week []feeds.Quake }

const (
	qmX, qmY = pad, 76
	qmW      = 860
	qmH      = 358
	qmTop    = 80.0  // latitude at the top edge
	qmSpan   = 150.0 // degrees of latitude shown
	qList    = qmX + qmW + 56
	qHot     = 6.0
)

func NewQuakes() *Quakes {
	return &Quakes{fetch: feeds.FetchQuakes, poll: poller[quakeData]{every: 5 * time.Minute}}
}

func qxy(lat, lon float64) (float64, float64) {
	lat = math.Max(qmTop-qmSpan, math.Min(qmTop, lat))
	return qmX + (lon+180)/360*qmW, qmY + (qmTop-lat)/qmSpan*qmH
}

func (q *Quakes) Frame(t, dt float64, now time.Time) *image.RGBA {
	d, got, ok, err := q.poll.get("", func(string) (quakeData, error) {
		day, week, err := q.fetch()
		return quakeData{day, week}, err
	})
	if !ok {
		return waitMessage(&q.cache, "EARTHQUAKES", err)
	}
	img := q.cache.get(fmt.Sprint(got.UnixNano(), now.Format("15:04")), func(img *image.RGBA) { q.render(img, d, now) })
	// a quake in the last hour sends out rings
	for _, e := range d.day {
		if now.Sub(e.Time) > time.Hour {
			break // newest first
		}
		x, y := qxy(e.Lat, e.Lon)
		k := math.Mod(t/2, 1)
		ring(img, int(x), int(y), 6+k*34, mix(bg, quakeColour(e.Mag), 1-k))
	}
	return img
}

func quakeColour(m float64) color.RGBA {
	if m >= qHot {
		return hot
	}
	return ink
}

func (q *Quakes) render(img *image.RGBA, d quakeData, now time.Time) {
	caps(img, qmX, pad-4, "earthquakes  last 24 hours m2.5+  last 30 days m4.5+", qmW)
	// graticule every 30 degrees
	for lon := -180.0; lon <= 180; lon += 30 {
		x, _ := qxy(0, lon)
		rect(img, int(x), qmY, int(x), qmY+qmH, rule)
	}
	for lat := 60.0; lat >= -60; lat -= 30 {
		_, y := qxy(lat, 0)
		rect(img, qmX, int(y), qmX+qmW, int(y), rule)
	}
	for _, e := range d.week {
		x, y := qxy(e.Lat, e.Lon)
		disc(img, x, y, 1.6+(e.Mag-4.5)*1.4, ink3)
	}
	for i := len(d.day) - 1; i >= 0; i-- { // oldest first, so the newest sit on top
		e := d.day[i]
		x, y := qxy(e.Lat, e.Lon)
		c := ink2
		if e.Mag >= qHot {
			c = hot
		}
		disc(img, x, y, 2+(e.Mag-2.5)*1.6, c)
	}

	// the list
	x := qList
	biggest := 0.0
	for _, e := range d.day {
		biggest = math.Max(biggest, e.Mag)
	}
	head := fmt.Sprintf("%d TODAY   LARGEST M%.1f", len(d.day), biggest)
	text(img, float64(W-pad)-measure(head, 22), pad-4, head, 22, ink3)
	for i, e := range d.day {
		if i >= 7 {
			break
		}
		y := 84 + i*52
		c := ink
		if e.Mag >= qHot {
			c = hot
		}
		text(img, float64(x), y-4, fmt.Sprintf("%.1f", e.Mag), 38, c)
		place := strings.ToUpper(e.Place)
		text(img, float64(x+110), y, fit(place, 24, float64(W-pad-x-110-200)), 24, ink)
		text(img, float64(x+110), y+28, fmt.Sprintf("DEPTH %.0f KM", e.Depth), 16, ink3)
		ago := agoText(now.Sub(e.Time))
		text(img, float64(W-pad)-measure(ago, 22), y+2, ago, 22, ink2)
	}
	if len(d.day) == 0 {
		text(img, float64(x), 100, "A QUIET DAY", 30, ink3)
	}
}

func agoText(d time.Duration) string {
	switch {
	case d < time.Minute:
		return "JUST NOW"
	case d < time.Hour:
		return fmt.Sprintf("%d MIN AGO", int(d.Minutes()))
	default:
		return fmt.Sprintf("%dH AGO", int(d.Hours()))
	}
}
