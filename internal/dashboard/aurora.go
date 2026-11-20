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

// AuroraView is the chance of seeing the northern lights from the UK: AuroraWatch UK's alert
// level large, the last 24 hours of geomagnetic disturbance against its thresholds, and NOAA's
// Kp index for the past days and the days ahead. A shimmering green curtain rises along the
// top when the alert is amber or red.
type AuroraView struct {
	fetch func() (*feeds.Aurora, error)
	poll  poller[*feeds.Aurora]
	cache frameCache
}

func NewAurora() *AuroraView {
	return &AuroraView{fetch: feeds.FetchAurora, poll: poller[*feeds.Aurora]{every: 10 * time.Minute}}
}

var auroraLevels = map[string]struct {
	colour color.RGBA
	say    string
}{
	"green":  {green, "No significant activity"},
	"yellow": {color.RGBA{240, 230, 70, 255}, "Minor activity: maybe visible in Scotland by camera"},
	"amber":  {amber, "Possible aurora: likely visible from Scotland, northern England and N. Ireland"},
	"red":    {hot, "Aurora likely: may be visible from anywhere in the UK"},
}

func (a *AuroraView) Frame(t, dt float64, now time.Time) *image.RGBA {
	d, got, ok, err := a.poll.get("", func(string) (*feeds.Aurora, error) { return a.fetch() })
	if !ok {
		return waitMessage(&a.cache, "AURORA", err)
	}
	img := a.cache.get(fmt.Sprint(got.UnixNano(), now.Format("15:04")), func(img *image.RGBA) { a.render(img, d, now) })
	if d.Status == "amber" || d.Status == "red" {
		curtain(img, t)
	}
	return img
}

// curtain draws soft green and violet rays waving along the top edge.
func curtain(img *image.RGBA, t float64) {
	for x := 0; x < W; x += 3 {
		k := 0.5 + 0.5*math.Sin(float64(x)*0.011+t*0.7)*math.Sin(float64(x)*0.003-t*0.3)
		h := int(10 + 40*k)
		c := mix(green, violet, 0.5+0.5*math.Sin(float64(x)*0.002+t*0.2))
		for y := 0; y < h; y++ {
			a := uint32(110 * (1 - float64(y)/float64(h)) * k)
			for dx := 0; dx < 3 && x+dx < W; dx++ {
				o := img.PixOffset(x+dx, y)
				img.Pix[o] = blend(a, img.Pix[o], c.R)
				img.Pix[o+1] = blend(a, img.Pix[o+1], c.G)
				img.Pix[o+2] = blend(a, img.Pix[o+2], c.B)
			}
		}
	}
}

const (
	auX1 = 600
	auX2 = 1260
)

func (a *AuroraView) render(img *image.RGBA, d *feeds.Aurora, now time.Time) {
	head(img, pad, pad-4, "aurora  uk", auX1-pad-40)
	lv, ok := auroraLevels[d.Status]
	if !ok {
		lv = auroraLevels["green"]
	}
	text(img, pad, 74, strings.ToUpper(d.Status), 110, lv.colour)
	for i, l := range wrap(strings.ToUpper(lv.say), 22, float64(auX1-pad-60), 3) {
		text(img, pad, 214+i*32, l, 22, txt)
	}
	if n := len(d.Activity); n > 0 {
		last := d.Activity[n-1]
		text(img, pad, 330, fmt.Sprintf("DISTURBANCE  %.0f nT", last.Value), 24, txt2)
	}
	if k, ok := kpNow(d.Kp, now); ok {
		text(img, pad, 368, fmt.Sprintf("KP NOW  %.1f", k), 24, kpColour(k))
	}
	text(img, pad, 410, "AURORAWATCH UK, LANCASTER UNIVERSITY", 16, txt3)
	vrule(img, auX1-30)

	// the last 24 hours against the alert thresholds (50 / 100 / 200 nT)
	x := auX1 + 10
	head(img, x, pad-4, "last 24 hours  nT", auX2-x-40)
	cy0, cy1 := 80, H-pad-26
	scale := 220.0
	for _, th := range []float64{50, 100, 200} {
		y := cy1 - int(th/scale*float64(cy1-cy0))
		rect(img, x, y, auX2-80, y, line)
		text(img, float64(auX2-74), y-9, fmt.Sprint(th), 16, txt3)
	}
	if n := len(d.Activity); n > 0 {
		step := float64(auX2-90-x) / float64(n)
		for i, h := range d.Activity {
			bh := int(math.Min(1, h.Value/scale) * float64(cy1-cy0))
			bx := x + int(float64(i)*step)
			c := auroraLevels[h.Status].colour
			if c == (color.RGBA{}) {
				c = green
			}
			rect(img, bx, cy1-max(bh, 1), bx+int(step)-3, cy1, c)
			if lt := h.At.In(now.Location()); lt.Hour()%6 == 0 {
				text(img, float64(bx), cy1+6, lt.Format("15"), 16, txt3)
			}
		}
	}
	vrule(img, auX2-10)

	// Kp, observed then predicted, every three hours
	x = auX2 + 20
	head(img, x, pad-4, "kp index  noaa", W-pad-x)
	text(img, float64(W-pad)-measure("FORECAST", 16), pad, "FORECAST", 16, violet)
	ky0, ky1 := 80, H-pad-26
	if n := len(d.Kp); n > 0 {
		step := float64(W-pad-x) / float64(n)
		for i, k := range d.Kp {
			bh := int(math.Min(1, k.Kp/9) * float64(ky1-ky0))
			bx := x + int(float64(i)*step)
			c := kpColour(k.Kp)
			if k.Predicted {
				c = mix(bg, c, 0.55)
			}
			rect(img, bx, ky1-max(bh, 1), bx+max(int(step)-2, 1), ky1, c)
			if lt := k.At.In(now.Location()); lt.Hour() < 3 {
				text(img, float64(bx), ky1+6, strings.ToUpper(lt.Format("Mon")), 16, txt3)
			}
		}
		// Kp 5 is a geomagnetic storm
		y := ky1 - int(5.0/9*float64(ky1-ky0))
		rect(img, x, y, W-pad, y, line)
		text(img, float64(x), y-20, "STORM", 14, txt3)
	}
}

func kpColour(k float64) color.RGBA { return ramp((k - 2) / 5) }

// kpNow is the Kp for the three-hour slot now falls in.
func kpNow(ks []feeds.KpPoint, now time.Time) (float64, bool) {
	for i := len(ks) - 1; i >= 0; i-- {
		if !ks[i].At.After(now.UTC()) {
			return ks[i].Kp, true
		}
	}
	return 0, false
}
