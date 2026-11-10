package dashboard

import (
	"fmt"
	"image"
	"math"
	"strings"
	"time"

	"github.com/justynroberts/marquee/internal/feeds"
)

// AirQuality is the air at the weather place, from Open-Meteo's CAMS forecast: the European
// AQI large with its band, the four main pollutants against WHO guideline levels, the next 48
// hours, and pollen (Europe only). A "poor" AQI or worse, or a pollutant at twice its
// guideline, takes the accent.
type AirQuality struct {
	place  func() string
	locate func(string) (feeds.Place, error)
	fetch  func(lat, lon float64) (*feeds.Air, error)
	poll   poller[airData]
	cache  frameCache
}

type airData struct {
	place feeds.Place
	air   *feeds.Air
}

func NewAirQuality(place func() string) *AirQuality {
	return &AirQuality{place: place, locate: feeds.Geocode, fetch: feeds.FetchAir, poll: poller[airData]{every: 30 * time.Minute}}
}

func (a *AirQuality) Frame(t, dt float64, now time.Time) *image.RGBA {
	d, got, ok, err := a.poll.get(a.place(), func(name string) (airData, error) {
		p, err := a.locate(name)
		if err != nil {
			return airData{}, err
		}
		air, err := a.fetch(p.Latitude, p.Longitude)
		return airData{p, air}, err
	})
	if !ok {
		return waitMessage(&a.cache, "AIR QUALITY", err)
	}
	return a.cache.get(fmt.Sprint(got.UnixNano()), func(img *image.RGBA) { a.render(img, d, got) })
}

const (
	aqX1 = 560
	aqX2 = 1400
)

func (a *AirQuality) render(img *image.RGBA, d airData, got time.Time) {
	air := d.air
	caps(img, pad, pad-4, "air quality  "+d.place.Name, aqX1-pad-40)
	band, bad := feeds.AQIBand(air.AQI)
	col := ink
	if bad {
		col = hot
	}
	v := fmt.Sprintf("%.0f", air.AQI)
	b := inkBox(v, 190)
	text(img, pad, 90-b.Min.Y, v, 190, col)
	text(img, pad, 90+b.Dy()+26, strings.ToUpper(band), 40, col)
	text(img, pad, 90+b.Dy()+84, "EUROPEAN AQI   "+got.Format("15:04"), 20, ink3)
	vline(img, aqX1-30)

	// pollutants against WHO 24-hour (8-hour for ozone) guideline levels
	x := aqX1 + 10
	caps(img, x, pad-4, "pollutants", 300)
	text(img, float64(x)+measure("POLLUTANTS", 22)+24, pad-4, "µg/m³   | WHO guideline", 22, ink3)
	type row struct {
		name  string
		v, gl float64
	}
	rows := []row{{"PM2.5", air.PM25, 15}, {"PM10", air.PM10, 45}, {"NO2", air.NO2, 25}, {"O3", air.O3, 100}}
	bx0, bx1 := x+130, aqX2-150
	for i, r := range rows {
		y := 86 + i*46
		c := ink
		if r.v >= 2*r.gl {
			c = hot
		}
		text(img, float64(x), y, r.name, 26, ink2)
		scale := 3 * r.gl
		rect(img, bx0, y+12, bx1, y+14, rule)
		rect(img, bx0, y+8, bx0+int(float64(bx1-bx0)*math.Min(1, r.v/scale)), y+18, c)
		gx := bx0 + (bx1-bx0)/3
		rect(img, gx, y+2, gx+1, y+24, ink2)
		s := fmt.Sprintf("%.0f", r.v)
		text(img, float64(aqX2-50)-measure(s, 26), y, s, 26, c)
	}

	// the next 48 hours
	cy0, cy1 := 300, H-pad-26
	caps(img, x, cy0-34, "next 48 hours", 400)
	if n := len(air.Hours); n > 0 {
		step := float64(aqX2-50-x) / float64(n)
		for i, h := range air.Hours {
			bh := int(math.Min(1, h/100) * float64(cy1-cy0))
			c := ink3
			if _, bad := feeds.AQIBand(h); bad {
				c = hot
			}
			bx := x + int(float64(i)*step)
			rect(img, bx, cy1-max(bh, 1), bx+int(step)-3, cy1, c)
			if hr := (air.HourStart + i) % 24; hr%6 == 0 {
				l := fmt.Sprintf("%02d", hr)
				text(img, float64(bx), cy1+6, l, 16, ink3)
			}
		}
	}
	vline(img, aqX2-20)

	// pollen
	px := aqX2 + 20
	caps(img, px, pad-4, "pollen", 200)
	text(img, float64(px)+measure("POLLEN", 22)+24, pad-4, "grains/m³", 22, ink3)
	if len(air.Pollen) == 0 {
		text(img, float64(px), 90, "NOT FORECAST HERE", 24, ink3)
		return
	}
	i := 0
	for _, k := range feeds.PollenKinds {
		v, ok := air.Pollen[k]
		if !ok {
			continue
		}
		y := 86 + i*52
		lvl, c := "LOW", ink3
		switch {
		case v >= 50:
			lvl, c = "HIGH", hot
		case v >= 10:
			lvl, c = "MODERATE", ink
		}
		text(img, float64(px), y, strings.ToUpper(k), 26, ink)
		text(img, float64(W-pad)-measure(lvl, 22), y+2, lvl, 22, c)
		text(img, float64(px), y+30, fmt.Sprintf("%.0f", v), 16, ink3)
		i++
	}
}
