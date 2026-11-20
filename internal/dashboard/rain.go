package dashboard

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"time"

	"github.com/justynroberts/marquee/internal/feeds"
)

// Rain answers "will it rain?": a verdict for the next two hours in large type, the rain
// expected every 15 minutes as bars, and the next twelve hours as a strip of chances.
type Rain struct {
	place func() string
	fetch func(string) (*feeds.RainCast, error)
	poll  poller[*feeds.RainCast]
	cache frameCache
}

func NewRain(place func() string) *Rain {
	return &Rain{place: place, fetch: feeds.FetchRain, poll: poller[*feeds.RainCast]{every: 10 * time.Minute}}
}

// rainWet is the least rain in a slot worth calling rain, in millimetres.
const rainWet = 0.1

func (r *Rain) Frame(t, dt float64, now time.Time) *image.RGBA {
	d, got, ok, err := r.poll.get(r.place(), r.fetch)
	if !ok {
		return waitMessage(&r.cache, "THE RAIN", err)
	}
	return r.cache.get(fmt.Sprint(got.UnixNano(), now.Format("15:04")), func(img *image.RGBA) { rainRender(img, d, now) })
}

// rainVerdict sums the next two hours up in a few words, with its colour.
func rainVerdict(d *feeds.RainCast) (string, color.RGBA) {
	q := d.Quarters
	first := -1
	for i, s := range q {
		if s.MM >= rainWet {
			first = i
			break
		}
	}
	switch {
	case len(q) == 0:
	case first == 0:
		for _, s := range q {
			if s.MM < rainWet {
				return "RAIN UNTIL " + s.At.Format("15:04"), cyan
			}
		}
		return "RAIN FOR 2 HOURS", cyan
	case first > 0:
		return "RAIN FROM " + q[first].At.Format("15:04"), amber
	}
	// dry for the next two hours: when does it start after that?
	var end time.Time
	if len(q) > 0 {
		end = q[len(q)-1].At.Add(15 * time.Minute)
	}
	for _, h := range d.Hours {
		if h.At.Add(time.Hour).After(end) && h.MM >= rainWet {
			if len(q) == 0 && h.At.Before(time.Now()) {
				return "RAIN NOW", cyan
			}
			return "DRY UNTIL " + h.At.Format("15:00"), green
		}
	}
	if len(d.Hours) > 0 {
		return fmt.Sprintf("DRY FOR %d HOURS", len(d.Hours)), green
	}
	return "DRY FOR 2 HOURS", green
}

const (
	rainX1 = 660  // 15-minute chart
	rainX2 = 1310 // hourly strip
)

var rainBlue = color.RGBA{70, 150, 255, 255}

func rainRender(img *image.RGBA, d *feeds.RainCast, now time.Time) {
	head(img, pad, pad-4, "rain  "+d.Place, rainX1-pad-40)
	say, c := rainVerdict(d)
	px := fitPx(say, 76, rainX1-pad-60, 90)
	text(img, pad, 84, say, px, c)
	total, peak, chance := 0.0, 0.0, 0
	for _, s := range d.Quarters {
		total += s.MM
		peak = math.Max(peak, s.MM)
		chance = max(chance, s.Chance)
	}
	text(img, pad, 196, fmt.Sprintf("%.1f MM IN THE NEXT 2 HOURS", total), 24, txt)
	text(img, pad, 236, fmt.Sprintf("CHANCE OF RAIN UP TO %d%%", chance), 24, txt2)
	if peak > 0 {
		text(img, pad, 276, "HEAVIEST "+rainIntensity(peak*4), 24, txt2)
	}
	day := 0.0
	for _, h := range d.Hours {
		day += h.MM
	}
	text(img, pad, 340, fmt.Sprintf("NEXT 12 HOURS  %.1f MM", day), 22, violet)
	text(img, pad, 410, "OPEN-METEO", 16, txt3)
	vrule(img, rainX1-30)

	// every 15 minutes: chance shaded behind, millimetres as bars
	x := rainX1 + 10
	head(img, x, pad-4, "next 2 hours  mm / 15 min", rainX2-x-60)
	y0, y1 := 90, H-pad-30
	scale := math.Max(1, math.Ceil(peak*2)/2)
	if n := len(d.Quarters); n > 0 {
		step := float64(rainX2-70-x) / float64(n)
		for i, s := range d.Quarters {
			bx := x + int(float64(i)*step)
			bw := int(step) - 10
			ch := int(float64(s.Chance) / 100 * float64(y1-y0))
			rect(img, bx, y1-ch, bx+bw, y1, mix(bg, violet, 0.22))
			if s.MM > 0 {
				bh := max(int(math.Min(1, s.MM/scale)*float64(y1-y0)), 3)
				rect(img, bx+8, y1-bh, bx+bw-8, y1, rainBlue)
				l := fmt.Sprintf("%.1f", s.MM)
				text(img, float64(bx+bw/2)-measure(l, 18)/2, y1-bh-24, l, 18, txt)
			}
			text(img, float64(bx), y1+6, s.At.Format("15:04"), 16, txt3)
		}
	}
	rect(img, x, y1, rainX2-60, y1, line)
	vrule(img, rainX2-30)

	// the next twelve hours
	x = rainX2 + 10
	head(img, x, pad-4, "next 12 hours  chance", W-pad-x)
	if n := len(d.Hours); n > 0 {
		step := float64(W-pad-x) / float64(n)
		for i, h := range d.Hours {
			bx := x + int(float64(i)*step)
			bw := int(step) - 6
			ch := max(int(float64(h.Chance)/100*float64(y1-y0)), 2)
			c := mix(bg, violet, 0.35+0.65*float64(h.Chance)/100)
			if h.MM >= rainWet {
				c = rainBlue
			}
			rect(img, bx, y1-ch, bx+bw, y1, c)
			if h.Chance > 0 {
				l := fmt.Sprint(h.Chance)
				text(img, float64(bx+bw/2)-measure(l, 16)/2, y1-ch-22, l, 16, txt2)
			}
			if i%2 == 0 {
				text(img, float64(bx), y1+6, h.At.Format("15"), 16, txt3)
			}
		}
	}
}

// rainIntensity names a rain rate in mm per hour as the Met Office does.
func rainIntensity(mmh float64) string {
	switch {
	case mmh < 0.5:
		return "DRIZZLE"
	case mmh < 4:
		return "LIGHT RAIN"
	case mmh < 8:
		return "MODERATE RAIN"
	case mmh < 50:
		return "HEAVY RAIN"
	}
	return "VIOLENT RAIN"
}
