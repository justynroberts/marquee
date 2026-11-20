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

// ISS is the International Space Station live: where it is over a map with day and night
// shaded, the orbit it has just flown and the one it is about to, the ground that can see it,
// and in the column its height, speed, how far it is from you and whether it is in sunlight.
type ISS struct {
	home       dnPlace
	fetchNow   func() (feeds.ISSPoint, error)
	fetchTrack func(from time.Time, step time.Duration, n int) ([]feeds.ISSPoint, error)
	pos        poller[feeds.ISSPoint]
	track      poller[[]feeds.ISSPoint]
	cache      frameCache
}

const (
	issBack   = 45 * time.Minute // track drawn behind and ahead of now
	issStep   = 3 * time.Minute
	issN      = 2*int(issBack/issStep) + 1
	issEarthR = 6371.0
)

func NewISS(place func() string) *ISS {
	return &ISS{
		home:       dnPlace{place: place, locate: feeds.Geocode},
		fetchNow:   feeds.FetchISS,
		fetchTrack: feeds.FetchISSTrack,
		pos:        poller[feeds.ISSPoint]{every: 10 * time.Second},
		track:      poller[[]feeds.ISSPoint]{every: 5 * time.Minute},
	}
}

func issVec(lat, lon float64) [3]float64 {
	rad := math.Pi / 180
	return [3]float64{math.Cos(lat*rad) * math.Cos(lon*rad), math.Cos(lat*rad) * math.Sin(lon*rad), math.Sin(lat * rad)}
}

func issLatLon(v [3]float64) (float64, float64) {
	return math.Atan2(v[2], math.Hypot(v[0], v[1])) * 180 / math.Pi, math.Atan2(v[1], v[0]) * 180 / math.Pi
}

// issSlerp moves k of the way along the great circle from a to b.
func issSlerp(a, b [3]float64, k float64) [3]float64 {
	d := math.Acos(math.Max(-1, math.Min(1, a[0]*b[0]+a[1]*b[1]+a[2]*b[2])))
	if d < 1e-9 {
		return a
	}
	s := math.Sin(d)
	wa, wb := math.Sin((1-k)*d)/s, math.Sin(k*d)/s
	return [3]float64{wa*a[0] + wb*b[0], wa*a[1] + wb*b[1], wa*a[2] + wb*b[2]}
}

// issAt is where the track puts the station at t, between its fetched points.
func issAt(track []feeds.ISSPoint, t time.Time) (float64, float64, bool) {
	for i := 0; i+1 < len(track); i++ {
		a, b := track[i], track[i+1]
		if t.Before(a.At) || !t.Before(b.At) {
			continue
		}
		k := float64(t.Sub(a.At)) / float64(b.At.Sub(a.At))
		lat, lon := issLatLon(issSlerp(issVec(a.Lat, a.Lon), issVec(b.Lat, b.Lon), k))
		return lat, lon, true
	}
	return 0, 0, false
}

// issArc is the angle in radians between two points on the ground.
func issArc(lat1, lon1, lat2, lon2 float64) float64 {
	a, b := issVec(lat1, lon1), issVec(lat2, lon2)
	return math.Acos(math.Max(-1, math.Min(1, a[0]*b[0]+a[1]*b[1]+a[2]*b[2])))
}

// issElevation is how high above your horizon a satellite alt km up, arc radians away, appears.
func issElevation(arc, alt float64) float64 {
	r := issEarthR + alt
	return math.Atan2(math.Cos(arc)-issEarthR/r, math.Sin(arc)) * 180 / math.Pi
}

// issCircle is the ring on the ground radius radians round (lat, lon).
func issCircle(lat, lon, radius float64) [][2]float64 {
	c := issVec(lat, lon)
	// two unit vectors at right angles to c
	u := [3]float64{-c[1], c[0], 0}
	if n := math.Hypot(u[0], u[1]); n < 1e-6 {
		u = [3]float64{1, 0, 0}
	} else {
		u[0], u[1] = u[0]/n, u[1]/n
	}
	v := [3]float64{c[1]*u[2] - c[2]*u[1], c[2]*u[0] - c[0]*u[2], c[0]*u[1] - c[1]*u[0]}
	var out [][2]float64
	for i := 0; i <= 72; i++ {
		a := float64(i) / 72 * 2 * math.Pi
		cr, sr := math.Cos(radius), math.Sin(radius)
		p := [3]float64{}
		for k := range p {
			p[k] = cr*c[k] + sr*(math.Cos(a)*u[k]+math.Sin(a)*v[k])
		}
		la, lo := issLatLon(p)
		out = append(out, [2]float64{lo, la})
	}
	return out
}

func (s *ISS) Frame(t, dt float64, now time.Time) *image.RGBA {
	p, pgot, pok, perr := s.pos.get("", func(string) (feeds.ISSPoint, error) { return s.fetchNow() })
	tr, tgot, tok, _ := s.track.get("", func(string) ([]feeds.ISSPoint, error) {
		return s.fetchTrack(now.Add(-issBack).Truncate(time.Minute), issStep, issN)
	})
	if !pok && !tok {
		return waitMessage(&s.cache, "THE ISS", perr)
	}
	home, _ := s.home.where()
	key := fmt.Sprint(pgot.UnixNano(), tgot.UnixNano(), now.Format("15:04"))
	if home != nil {
		key += home.Name
	}
	img := s.cache.get(key, func(img *image.RGBA) { s.render(img, home, p, pok, tr, now) })

	// the station, moving every frame
	lat, lon, ok := issAt(tr, now)
	if !ok {
		if !pok {
			return img
		}
		lat, lon = p.Lat, p.Lon
	}
	m := dnWorld
	if pok && p.Footprint > 0 {
		m.path(img, issCircle(lat, lon, p.Footprint/2/issEarthR), mix(bg, cyan, 0.7), 2)
	}
	x, y := m.xy(lon, lat)
	pulse := math.Mod(t, 2) / 2
	ring(img, int(x), int(y), 10+22*pulse, mix(bg, amber, 1-pulse))
	disc(img, x, y, 11, mix(bg, amber, 0.45))
	disc(img, x, y, 7, color.RGBA{255, 236, 170, 255})
	return img
}

const issSide = dnSide

func (s *ISS) render(img *image.RGBA, home *feeds.Place, p feeds.ISSPoint, pok bool, tr []feeds.ISSPoint, now time.Time) {
	m := dnWorld
	sLat, sLon := SubSolar(now)
	m.shade(img, sLat, sLon)
	m.land(img, dnCoast, dnGrat)

	// the orbit behind (faint) and ahead (bright)
	var past, next [][2]float64
	for i, q := range tr {
		pt := [2]float64{q.Lon, q.Lat}
		if q.At.After(now) {
			if len(next) == 0 && i > 0 {
				if lat, lon, ok := issAt(tr, now); ok {
					next = append(next, [2]float64{lon, lat})
					past = append(past, [2]float64{lon, lat})
				}
			}
			next = append(next, pt)
		} else {
			past = append(past, pt)
		}
	}
	m.path(img, issDense(past), mix(bg, violet, 0.8), 2)
	m.path(img, issDense(next), amber, 3)
	// ten-minute ticks ahead
	for _, q := range tr {
		if q.At.After(now) && q.At.Sub(now) < issBack && q.At.Minute()%15 == 0 {
			x, y := m.xy(q.Lon, q.Lat)
			disc(img, x, y, 4, amber)
			text(img, x+7, int(y)+4, q.At.In(now.Location()).Format("15:04"), 16, txt2)
		}
	}
	if home != nil {
		x, y := m.xy(home.Longitude, home.Latitude)
		ring(img, int(x), int(y), 9, green)
		disc(img, x, y, 4, green)
	}

	sx := issSide
	w := W - pad - sx
	head(img, sx, pad-4, "space station  iss", w)
	if !pok {
		text(img, float64(sx), 80, "POSITION FROM THE TRACK", 20, txt3)
		return
	}
	text(img, float64(sx), 66, dnLatLon(p.Lat, p.Lon), 40, amber)
	text(img, float64(sx), 124, fmt.Sprintf("%.0f KM UP", p.Alt), 26, txt)
	text(img, float64(sx)+230, 124, fmt.Sprintf("%s KM/H", issThousands(p.Speed)), 26, txt)
	if p.Visibility == "daylight" {
		text(img, float64(sx), 168, "IN SUNLIGHT", 24, amber)
	} else {
		text(img, float64(sx), 168, "IN EARTH'S SHADOW", 24, violet)
	}
	if home != nil {
		arc := issArc(home.Latitude, home.Longitude, p.Lat, p.Lon)
		el := issElevation(arc, p.Alt)
		text(img, float64(sx), 222, fit(strings.ToUpper(home.Name), 18, float64(w)), 18, green)
		text(img, float64(sx), 246, fmt.Sprintf("%s KM AWAY", issThousands(arc*issEarthR)), 30, txt)
		sky := SunElevation(now, home.Latitude, home.Longitude)
		switch {
		case el > 10 && p.Visibility == "daylight" && sky < -6:
			text(img, float64(sx), 290, fmt.Sprintf("LOOK UP  %.0f° HIGH, LIT", el), 24, green)
		case el > 0:
			text(img, float64(sx), 290, fmt.Sprintf("ABOVE YOUR HORIZON  %.0f°", el), 24, cyan)
		default:
			text(img, float64(sx), 290, "BELOW YOUR HORIZON", 24, txt2)
		}
		dark := "YOUR SKY IS LIGHT"
		if sky < -6 {
			dark = "YOUR SKY IS DARK"
		}
		text(img, float64(sx), 326, dark, 20, txt3)
	}
	text(img, float64(sx), 372, "ONE ORBIT EVERY 92 MIN", 20, txt2)
	text(img, float64(sx), 400, "AHEAD", 18, amber)
	text(img, float64(sx)+80, 400, "BEHIND", 18, violet)
	text(img, float64(W-pad)-measure("WHERETHEISS.AT", 16), H-pad-12, "WHERETHEISS.AT", 16, txt3)
}

// issDense adds points along each great-circle step so the track curves smoothly.
func issDense(pts [][2]float64) [][2]float64 {
	if len(pts) < 2 {
		return pts
	}
	var out [][2]float64
	for i := 0; i+1 < len(pts); i++ {
		a, b := issVec(pts[i][1], pts[i][0]), issVec(pts[i+1][1], pts[i+1][0])
		for k := 0; k < 6; k++ {
			lat, lon := issLatLon(issSlerp(a, b, float64(k)/6))
			out = append(out, [2]float64{lon, lat})
		}
	}
	return append(out, pts[len(pts)-1])
}

func issThousands(v float64) string {
	s := fmt.Sprintf("%.0f", v)
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	return s
}
