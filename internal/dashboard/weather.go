package dashboard

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/justynroberts/marquee/internal/feeds"
)

// Weather is a full-resolution forecast: now on the left (place, temperature, conditions,
// feels-like, wind, humidity, sun), the next 24 hours in the middle as a temperature curve over
// rain-chance bars, and the coming days on the right. Temperatures are coloured from violet (freezing) to vermilion (30°C and up).
type Weather struct {
	place func() string
	fetch func(string) (*feeds.Forecast, error)

	mu    sync.Mutex
	fc    *feeds.Forecast
	err   error
	asked string
	at    time.Time
	busy  bool

	cache frameCache
}

const weatherEvery = 15 * time.Minute

// NewWeather shows the forecast for place(), refetched every 15 minutes or when it changes.
func NewWeather(place func() string) *Weather {
	return &Weather{place: place, fetch: feeds.FetchForecast}
}

func (w *Weather) refresh() {
	p := w.place()
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.busy || (p == w.asked && time.Since(w.at) < weatherEvery && (w.fc != nil || time.Since(w.at) < time.Minute)) {
		return
	}
	w.busy, w.asked = true, p
	go func() {
		fc, err := w.fetch(p)
		w.mu.Lock()
		defer w.mu.Unlock()
		w.busy, w.at = false, time.Now()
		if err == nil {
			w.fc, w.err = fc, nil
		} else {
			w.err = err
		}
	}()
}

func (w *Weather) Frame(t, dt float64, now time.Time) *image.RGBA {
	w.refresh()
	w.mu.Lock()
	fc, err := w.fc, w.err
	w.mu.Unlock()
	if fc == nil {
		msg := "FETCHING WEATHER…"
		if err != nil {
			msg = strings.ToUpper(err.Error())
		}
		return w.cache.get("msg"+msg, func(img *image.RGBA) {
			msg = fit(msg, 28, W-2*pad)
			text(img, (W-measure(msg, 28))/2, H/2-18, msg, 28, txt3)
		})
	}
	return w.cache.get(fc.Fetched.String()+now.Format("15:04"), func(img *image.RGBA) { w.render(img, fc, now) })
}

// tempColour runs violet below freezing, cyan when cold, green when mild, amber when warm and
// vermilion from 30°C.
func tempColour(c float64) color.RGBA {
	switch {
	case c >= 30:
		return hot
	case c >= 20:
		return amber
	case c >= 10:
		return green
	case c > 0:
		return cyan
	}
	return violet
}

const (
	wx1 = 600  // end of "now"
	wx2 = 1420 // end of the hourly chart
)

func (w *Weather) render(img *image.RGBA, fc *feeds.Forecast, now time.Time) {
	// now
	text(img, pad, pad, fit(strings.ToUpper(fc.Place), 30, wx1-pad-40), 30, txt2)
	big := fmt.Sprintf("%.0f°", fc.Temp)
	b := inkBox(big, 190)
	text(img, pad, 92-b.Min.Y, big, 190, tempColour(fc.Temp))
	y := 92 + b.Dy() + 28
	text(img, pad, y, fit(feeds.Describe(fc.Code), 34, wx1-pad-40), 34, txt)
	text(img, pad, y+50, fmt.Sprintf("FEELS %.0f°  WIND %.0f KM/H  HUM %.0f%%", fc.Feels, fc.Wind, fc.Humidity), 22, txt2)
	text(img, pad, y+84, fmt.Sprintf("SUN %s – %s   UPDATED %s", fc.Sunrise, fc.Sunset, fc.Fetched.Format("15:04")), 22, txt3)
	vrule(img, wx1-20)

	// next 24 hours
	x0, x1 := wx1+20, wx2-40
	head(img, x0, pad, "next 24 hours", x1-x0)
	hs := fc.Hours
	if len(hs) >= 2 {
		lo, hi := math.Inf(1), math.Inf(-1)
		for _, h := range hs {
			lo, hi = math.Min(lo, h.Temp), math.Max(hi, h.Temp)
		}
		if hi-lo < 4 {
			mid := (hi + lo) / 2
			lo, hi = mid-2, mid+2
		}
		cy0, cy1 := 120, 290 // curve band
		ry0, ry1 := 330, 390 // rain bars
		step := float64(x1-x0) / float64(len(hs)-1)
		pts := make([][2]float64, len(hs))
		for i, h := range hs {
			px := float64(x0) + float64(i)*step
			pts[i] = [2]float64{px, float64(cy1) - (h.Temp-lo)/(hi-lo)*float64(cy1-cy0)}
			// rain chance
			bw := int(step) - 6
			bh := int(float64(ry1-ry0) * float64(h.Rain) / 100)
			bx := int(px) - bw/2
			rect(img, bx, ry1, bx+bw, ry1, line)
			if bh > 0 {
				rect(img, bx, ry1-bh, bx+bw, ry1, cyan)
			}
			if i%3 == 0 {
				lbl := fmt.Sprintf("%02d", h.Hour)
				text(img, px-measure(lbl, 20)/2, ry1+14, lbl, 20, txt3)
				tl := fmt.Sprintf("%.0f°", h.Temp)
				text(img, px-measure(tl, 22)/2, int(pts[i][1])-38, tl, 22, txt2)
			}
		}
		polyline(img, pts, amber, 4)
		for i := 0; i < len(pts); i += 3 {
			rect(img, int(pts[i][0])-4, int(pts[i][1])-4, int(pts[i][0])+4, int(pts[i][1])+4, txt)
		}
		text(img, float64(x1)-measure("RAIN %", 18), ry0-30, "RAIN %", 18, txt3)
	}
	vrule(img, wx2-20)

	// coming days
	x := wx2 + 20
	head(img, x, pad, "days", W-pad-x)
	rowH := 66
	for i, d := range fc.Days {
		if i >= 5 {
			break
		}
		ry := pad + 46 + i*rowH
		name := d.Day
		if i == 0 {
			name = "TODAY"
		}
		text(img, float64(x), ry, name, 26, cyan)
		text(img, float64(x), ry+32, fit(feeds.Describe(d.Code), 18, 220), 18, txt3)
		hi := fmt.Sprintf("%.0f°", d.Hi)
		lo := fmt.Sprintf("%.0f°", d.Lo)
		rx := float64(W - pad)
		text(img, rx-measure(lo, 26), ry, lo, 26, tempColour(d.Lo))
		text(img, rx-measure(lo, 26)-24-measure(hi, 26), ry, hi, 26, tempColour(d.Hi))
		if d.Rain > 0 {
			r := fmt.Sprintf("%d%%", d.Rain)
			text(img, rx-measure(r, 18), ry+32, r, 18, txt3)
		}
	}
}
