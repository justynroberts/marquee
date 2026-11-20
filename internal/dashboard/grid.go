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

// GridPower is the electricity grid where you are (GB only): how much carbon each kWh costs
// now, what is generating it, the next 48 hours, and the cleanest time in the next day to run
// something heavy. Region from this Mac's postcode district, or grid.txt.
type GridPower struct {
	outcode func() string
	fetch   func(string) (*feeds.Grid, error)
	poll    poller[*feeds.Grid]
	cache   frameCache
}

func NewGridPower(outcode func() string) *GridPower {
	return &GridPower{outcode: outcode, fetch: feeds.FetchGrid, poll: poller[*feeds.Grid]{every: 15 * time.Minute}}
}

func (g *GridPower) Frame(t, dt float64, now time.Time) *image.RGBA {
	d, got, ok, err := g.poll.get(g.outcode(), g.fetch)
	if !ok {
		return waitMessage(&g.cache, "THE GRID", err)
	}
	return g.cache.get(fmt.Sprint(got.UnixNano(), now.Format("15:04")), func(img *image.RGBA) { g.render(img, d, now) })
}

// intensityColour runs green (under 100 g) through amber to vermilion (over 300 g).
func intensityColour(v int) color.RGBA { return ramp((float64(v) - 60) / 260) }

var fuelColours = map[string]color.RGBA{
	"wind": cyan, "solar": amber, "nuclear": violet, "hydro": color.RGBA{80, 150, 255, 255},
	"biomass": green, "gas": hot, "coal": color.RGBA{150, 120, 110, 255}, "imports": txt2, "other": txt3,
}

const (
	gdX1 = 560
	gdX2 = 1180
)

func (g *GridPower) render(img *image.RGBA, d *feeds.Grid, now time.Time) {
	head(img, pad, pad-4, "the grid  "+d.Region, gdX1-pad-40)
	v := fmt.Sprint(d.Intensity)
	c := intensityColour(d.Intensity)
	b := inkBox(v, 170)
	adv := text(img, pad, 86-b.Min.Y, v, 170, c)
	text(img, pad+adv+18, 86+b.Dy()-50, "gCO2", 26, txt2)
	text(img, pad+adv+18, 86+b.Dy()-20, "PER KWH", 18, txt3)
	text(img, pad, 86+b.Dy()+26, strings.ToUpper(d.Index), 38, c)

	// cleanest stretch of the next 24 hours: the lowest three-hour average
	if n := min(len(d.Forecast), 48); n >= 6 {
		best, at := math.Inf(1), 0
		for i := 0; i+6 <= n; i++ {
			s := 0
			for _, h := range d.Forecast[i : i+6] {
				s += h.Intensity
			}
			if a := float64(s) / 6; a < best {
				best, at = a, i
			}
		}
		from := d.Forecast[at].From.In(now.Location())
		day := ""
		if from.Day() != now.Day() {
			day = "TOMORROW "
		}
		text(img, pad, 330, "GREENEST 3 HOURS", 20, cyan)
		text(img, pad, 358, fmt.Sprintf("%s%s – %s", day, from.Format("15:04"), from.Add(3*time.Hour).Format("15:04")), 30, green)
		text(img, pad, 398, fmt.Sprintf("ABOUT %.0f G/KWH", best), 18, txt3)
	}
	vrule(img, gdX1-30)

	// what is generating
	x := gdX1 + 10
	head(img, x, pad-4, "generation mix", gdX2-x-40)
	bx0, bx1 := x+170, gdX2-120
	for i, f := range d.Mix {
		if i >= 8 {
			break
		}
		y := 80 + i*42
		fc, ok := fuelColours[f.Name]
		if !ok {
			fc = txt2
		}
		text(img, float64(x), y, strings.ToUpper(f.Name), 24, fc)
		rect(img, bx0, y+10, bx1, y+12, line)
		rect(img, bx0, y+6, bx0+int(float64(bx1-bx0)*f.Perc/100), y+16, fc)
		p := fmt.Sprintf("%.0f%%", f.Perc)
		text(img, float64(gdX2-50)-measure(p, 24), y, p, 24, txt)
	}
	vrule(img, gdX2-20)

	// the next 48 hours, each half hour a bar
	x = gdX2 + 10
	head(img, x, pad-4, "next 48 hours", W-pad-x)
	cy0, cy1 := 80, H-pad-26
	if n := len(d.Forecast); n > 0 {
		step := float64(W-pad-x) / float64(n)
		for i, h := range d.Forecast {
			bh := int(math.Min(1, float64(h.Intensity)/400) * float64(cy1-cy0))
			bx := x + int(float64(i)*step)
			nx := x + int(float64(i+1)*step)
			rect(img, bx, cy1-max(bh, 1), max(nx-2, bx), cy1, intensityColour(h.Intensity))
			if lt := h.From.In(now.Location()); lt.Minute() == 0 && lt.Hour()%12 == 0 {
				l := lt.Format("15")
				if lt.Hour() == 0 {
					l = strings.ToUpper(lt.Format("Mon"))
				}
				text(img, float64(bx), cy1+6, l, 16, txt3)
			}
		}
	}
}
