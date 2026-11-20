package dashboard

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"sync"
	"time"

	"github.com/justynroberts/marquee/internal/feeds"
)

// Sun is today's daylight where you are: the sun's height through the day as a curve over the
// horizon, where it is now, and dawn, sunrise, noon, sunset and dusk, with how much longer or
// shorter the day is than yesterday. Worked out on the Mac; only the place name is looked up.
type Sun struct {
	place  func() string
	locate func(string) (feeds.Place, error)

	mu    sync.Mutex
	asked string
	home  *feeds.Place
	busy  bool
	err   error
	cache frameCache
}

func NewSun(place func() string) *Sun { return &Sun{place: place, locate: feeds.Geocode} }

// SunElevation is the sun's height above the horizon in degrees at t, seen from lat, lon
// (NOAA's low-precision formulas: good to a few hundredths of a degree).
func SunElevation(t time.Time, lat, lon float64) float64 {
	n := float64(t.UnixNano())/86400e9 + 2440587.5 - 2451545.0
	rad := math.Pi / 180
	L := math.Mod(280.460+0.9856474*n, 360)
	g := math.Mod(357.528+0.9856003*n, 360) * rad
	lambda := (L + 1.915*math.Sin(g) + 0.020*math.Sin(2*g)) * rad
	eps := (23.439 - 0.0000004*n) * rad
	ra := math.Atan2(math.Cos(eps)*math.Sin(lambda), math.Cos(lambda))
	dec := math.Asin(math.Sin(eps) * math.Sin(lambda))
	gmst := math.Mod(280.46061837+360.98564736629*n, 360)
	h := (gmst+lon)*rad - ra
	la := lat * rad
	return math.Asin(math.Sin(la)*math.Sin(dec)+math.Cos(la)*math.Cos(dec)*math.Cos(h)) / rad
}

// SunDay holds a day's sun events (zero when it does not happen, as in polar summer).
type SunDay struct {
	Dawn, Rise, Noon, Set, Dusk time.Time
	NoonElev                    float64
	Elev                        [24 * 60]float64 // each minute from local midnight
}

// Daylight is sunrise to sunset.
func (d SunDay) Daylight() time.Duration {
	if d.Rise.IsZero() || d.Set.IsZero() {
		return 0
	}
	return d.Set.Sub(d.Rise)
}

// SunTimes samples the day minute by minute and finds where the sun crosses -0.833° (rise and
// set, allowing for refraction and the sun's size) and -6° (civil dawn and dusk).
func SunTimes(day time.Time, lat, lon float64) SunDay {
	var d SunDay
	mid := time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, day.Location())
	d.NoonElev = -90
	for i := range d.Elev {
		t := mid.Add(time.Duration(i) * time.Minute)
		e := SunElevation(t, lat, lon)
		d.Elev[i] = e
		if e > d.NoonElev {
			d.NoonElev, d.Noon = e, t
		}
		if i == 0 {
			continue
		}
		p := d.Elev[i-1]
		cross := func(level float64) bool { return (p < level) != (e < level) }
		at := t.Add(-30 * time.Second)
		if cross(-0.833) {
			if e > p {
				d.Rise = at
			} else {
				d.Set = at
			}
		}
		if cross(-6) {
			if e > p {
				d.Dawn = at
			} else {
				d.Dusk = at
			}
		}
	}
	return d
}

func (s *Sun) where() (*feeds.Place, error) {
	name := s.place()
	s.mu.Lock()
	defer s.mu.Unlock()
	if name != s.asked && !s.busy {
		s.asked, s.busy, s.home, s.err = name, true, nil, nil
		go func() {
			p, err := s.locate(name)
			s.mu.Lock()
			defer s.mu.Unlock()
			s.busy, s.err = false, err
			if err == nil {
				s.home = &p
			} else {
				s.asked = "" // try again
			}
		}()
	}
	return s.home, s.err
}

func (s *Sun) Frame(t, dt float64, now time.Time) *image.RGBA {
	home, err := s.where()
	if home == nil {
		return waitMessage(&s.cache, "THE SUN", err)
	}
	return s.cache.get(fmt.Sprint(home.Name, now.Format("2006-01-02 15:04")), func(img *image.RGBA) { s.render(img, *home, now) })
}

const (
	snX0 = 640 // chart
	snY0 = 70
	snY1 = H - pad - 28
)

var (
	skyNight = color.RGBA{24, 20, 46, 255}
	skyDay   = color.RGBA{70, 52, 16, 255}
)

func clockHM(t time.Time) string {
	if t.IsZero() {
		return "--:--"
	}
	return t.Format("15:04")
}

func (s *Sun) render(img *image.RGBA, home feeds.Place, now time.Time) {
	today := SunTimes(now, home.Latitude, home.Longitude)
	yest := SunTimes(now.AddDate(0, 0, -1), home.Latitude, home.Longitude)
	head(img, pad, pad-4, "the sun  "+home.Name, snX0-pad-40)

	dl := today.Daylight()
	big := fmt.Sprintf("%dH %02dM", int(dl.Hours()), int(dl.Minutes())%60)
	text(img, pad, 80, big, 84, amber)
	diff := dl - yest.Daylight()
	sign, dc := "LONGER", green
	if diff < 0 {
		sign, dc, diff = "SHORTER", violet, -diff
	}
	text(img, pad, 180, fmt.Sprintf("DAYLIGHT  %dM %02dS %s THAN YESTERDAY", int(diff.Minutes()), int(diff.Seconds())%60, sign), 20, dc)
	rows := []struct {
		label string
		t     time.Time
		c     color.RGBA
	}{{"DAWN", today.Dawn, violet}, {"SUNRISE", today.Rise, amber}, {"NOON", today.Noon, txt}, {"SUNSET", today.Set, amber}, {"DUSK", today.Dusk, violet}}
	for i, r := range rows {
		y := 232 + i*40
		text(img, pad, y, r.label, 22, cyan)
		v := clockHM(r.t)
		text(img, float64(pad+180), y-4, v, 30, r.c)
		if r.label == "NOON" {
			text(img, float64(pad+300), y, fmt.Sprintf("%.0f° HIGH", today.NoonElev), 22, txt2)
		}
	}
	vrule(img, snX0-30)

	// the day's curve: sky shaded day and night, the sun's path, the horizon
	x0, x1 := snX0+10, W-pad
	low := 90.0
	for _, e := range today.Elev {
		low = math.Min(low, e)
	}
	lo, hi := math.Floor(low/10)*10-2, math.Max(30, math.Ceil(today.NoonElev/10)*10+5)
	ey := func(e float64) float64 { return float64(snY1) - (e-lo)/(hi-lo)*float64(snY1-snY0) }
	horizon := int(ey(0))
	w := x1 - x0
	pts := make([][2]float64, 0, w)
	for x := 0; x < w; x++ {
		m := x * len(today.Elev) / w
		e := today.Elev[m]
		c := skyNight
		if e > -6 {
			c = mix(skyNight, skyDay, math.Min(1, (e+6)/12))
		}
		rect(img, x0+x, snY0, x0+x, snY1, mix(bg, c, 0.55))
		pts = append(pts, [2]float64{float64(x0 + x), ey(e)})
	}
	rect(img, x0, horizon, x1, horizon+1, txt3)
	text(img, float64(x0+6), horizon+6, "HORIZON", 16, txt3)
	polyline(img, pts, amber, 3)
	for h := 0; h <= 24; h += 3 {
		x := x0 + w*h/24
		rect(img, x, snY1, x, snY1+6, txt3)
		l := fmt.Sprintf("%02d", h%24)
		text(img, float64(x)-measure(l, 16)/2, snY1+8, l, 16, txt3)
	}
	// rise and set marks
	for _, e := range []time.Time{today.Rise, today.Set} {
		if e.IsZero() {
			continue
		}
		x := x0 + int(float64(w)*minutesOfDay(e)/1440)
		disc(img, float64(x), float64(horizon), 5, amber)
	}
	// now
	m := minutesOfDay(now)
	nx := float64(x0) + float64(w)*m/1440
	ne := SunElevation(now, home.Latitude, home.Longitude)
	ny := ey(ne)
	rect(img, int(nx), snY0, int(nx), snY1, mix(bg, cyan, 0.5))
	if ne > -0.833 {
		disc(img, nx, ny, 14, mix(bg, amber, 0.35))
		disc(img, nx, ny, 9, color.RGBA{255, 226, 120, 255})
	} else {
		disc(img, nx, ny, 7, violet)
	}
	lbl := fmt.Sprintf("NOW %s  %.0f°", now.Format("15:04"), ne)
	lx := math.Min(nx+16, float64(x1)-measure(lbl, 20))
	text(img, lx, snY0+4, lbl, 20, cyan)
}

func minutesOfDay(t time.Time) float64 {
	return float64(t.Hour()*60+t.Minute()) + float64(t.Second())/60
}
