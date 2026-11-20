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

// DayNight is the world now: a map with the part in daylight lit, the night side dark and the
// twilight bands between them, the points where the sun and the moon are overhead, where you
// are, and the world-clock cities with their local times. Worked out on the Mac.
type DayNight struct {
	home  dnPlace
	zones func() []Zone
	cache frameCache
}

func NewDayNight(place func() string, zones func() []Zone) *DayNight {
	return &DayNight{home: dnPlace{place: place, locate: feeds.Geocode}, zones: zones}
}

// ---------------------------------------------------------------- shared map helpers (ISS too)

// dnGeo is an equirectangular world map placed on the panel.
type dnGeo struct {
	x, y, w, h       int
	lonL, latT, latB float64
}

var dnWorld = dnGeo{x: pad, y: 30, w: 1300, h: 418, lonL: -180, latT: 78, latB: -58}

func (m dnGeo) xy(lon, lat float64) (float64, float64) {
	lon = math.Mod(lon-m.lonL+720, 360)
	return float64(m.x) + lon/360*float64(m.w), float64(m.y) + (m.latT-lat)/(m.latT-m.latB)*float64(m.h)
}

// lonLat is the inverse of xy for a pixel inside the map.
func (m dnGeo) lonLat(px, py int) (float64, float64) {
	return m.lonL + (float64(px-m.x)+0.5)/float64(m.w)*360, m.latT - (float64(py-m.y)+0.5)/float64(m.h)*(m.latT-m.latB)
}

// path draws a line of (lon, lat) points, breaking it where it wraps round the map's edge.
func (m dnGeo) path(img *image.RGBA, lonlat [][2]float64, c color.RGBA, width int) {
	var pts [][2]float64
	prev := 0.0
	for _, p := range lonlat {
		if p[1] > m.latT || p[1] < m.latB { // off the map: break the line
			polyline(img, pts, c, width)
			pts = pts[:0]
			continue
		}
		x, y := m.xy(p[0], p[1])
		if len(pts) > 0 && math.Abs(x-prev) > float64(m.w)/2 {
			polyline(img, pts, c, width)
			pts = pts[:0]
		}
		pts = append(pts, [2]float64{x, y})
		prev = x
	}
	polyline(img, pts, c, width)
}

// land draws the graticule and the continent outlines.
func (m dnGeo) land(img *image.RGBA, coast, grat color.RGBA) {
	for lon := -150.0; lon < 180; lon += 30 {
		x, _ := m.xy(lon, 0)
		for y := m.y; y < m.y+m.h; y += 6 {
			rect(img, int(x), y, int(x), y+1, grat)
		}
	}
	for lat := 60.0; lat >= m.latB; lat -= 30 {
		_, y := m.xy(0, lat)
		for x := m.x; x < m.x+m.w; x += 6 {
			rect(img, x, int(y), x+1, int(y), grat)
		}
	}
	for _, poly := range landOutlines {
		pts := make([][2]float64, 0, len(poly)/2)
		for i := 0; i+1 < len(poly); i += 2 {
			pts = append(pts, [2]float64{poly[i], poly[i+1]})
		}
		m.path(img, pts, coast, 2)
	}
}

var (
	dnDay   = color.RGBA{84, 72, 30, 255}
	dnCivil = color.RGBA{62, 50, 84, 255}
	dnNaut  = color.RGBA{44, 38, 76, 255}
	dnAstro = color.RGBA{30, 28, 60, 255}
	dnNight = color.RGBA{13, 13, 32, 255}
	dnCoast = color.RGBA{150, 206, 186, 255}
	dnGrat  = color.RGBA{44, 60, 70, 255}
)

// dnShade is the ground colour for a sun elevation: day, civil, nautical and astronomical
// twilight in steps, then night.
func dnShade(elev float64) color.RGBA {
	switch {
	case elev > 0:
		return dnDay
	case elev > -6:
		return dnCivil
	case elev > -12:
		return dnNaut
	case elev > -18:
		return dnAstro
	}
	return dnNight
}

// shade fills the map with day, twilight and night for the sun overhead at (sLat, sLon).
func (m dnGeo) shade(img *image.RGBA, sLat, sLon float64) {
	rad := math.Pi / 180
	sd, cd := math.Sin(sLat*rad), math.Cos(sLat*rad)
	cosH := make([]float64, m.w)
	for i := range cosH {
		lon, _ := m.lonLat(m.x+i, m.y)
		cosH[i] = math.Cos((lon - sLon) * rad)
	}
	for j := 0; j < m.h; j++ {
		_, lat := m.lonLat(m.x, m.y+j)
		sl, cl := math.Sin(lat*rad), math.Cos(lat*rad)
		o := img.PixOffset(m.x, m.y+j)
		for i := 0; i < m.w; i++ {
			c := dnShade(math.Asin(sl*sd+cl*cd*cosH[i]) / rad)
			img.Pix[o], img.Pix[o+1], img.Pix[o+2], img.Pix[o+3] = c.R, c.G, c.B, 255
			o += 4
		}
	}
}

// SubSolar is the point on Earth where the sun is overhead at t (latitude, longitude).
func SubSolar(t time.Time) (float64, float64) {
	n := float64(t.UnixNano())/86400e9 + 2440587.5 - 2451545.0
	rad := math.Pi / 180
	L := math.Mod(280.460+0.9856474*n, 360)
	g := math.Mod(357.528+0.9856003*n, 360) * rad
	lambda := (L + 1.915*math.Sin(g) + 0.020*math.Sin(2*g)) * rad
	eps := (23.439 - 0.0000004*n) * rad
	ra := math.Atan2(math.Cos(eps)*math.Sin(lambda), math.Cos(lambda))
	dec := math.Asin(math.Sin(eps) * math.Sin(lambda))
	return dec / rad, dnLon(ra/rad - dnGMST(n))
}

// SubLunar is the point where the moon is overhead at t, from the Astronomical Almanac's
// low-precision lunar formulas (good to a degree or so: plenty for a map).
func SubLunar(t time.Time) (float64, float64) {
	n := float64(t.UnixNano())/86400e9 + 2440587.5 - 2451545.0
	rad := math.Pi / 180
	L := 218.316 + 13.176396*n
	M := (134.963 + 13.064993*n) * rad
	F := (93.272 + 13.229350*n) * rad
	lambda := (L + 6.289*math.Sin(M)) * rad
	beta := 5.128 * math.Sin(F) * rad
	eps := (23.439 - 0.0000004*n) * rad
	ra := math.Atan2(math.Sin(lambda)*math.Cos(eps)-math.Tan(beta)*math.Sin(eps), math.Cos(lambda))
	dec := math.Asin(math.Sin(beta)*math.Cos(eps) + math.Cos(beta)*math.Sin(eps)*math.Sin(lambda))
	return dec / rad, dnLon(ra/rad - dnGMST(n))
}

func dnGMST(n float64) float64 { return math.Mod(280.46061837+360.98564736629*n, 360) }

// dnLon wraps a longitude into -180..180.
func dnLon(l float64) float64 { return math.Mod(math.Mod(l+180, 360)+360, 360) - 180 }

// dnLatLon writes a latitude and longitude as "52.9°N  1.3°W".
func dnLatLon(lat, lon float64) string {
	ns, ew := "N", "E"
	if lat < 0 {
		ns, lat = "S", -lat
	}
	if lon < 0 {
		ew, lon = "W", -lon
	}
	return fmt.Sprintf("%.1f°%s  %.1f°%s", lat, ns, lon, ew)
}

// dnPlace looks the configured place up in the background, as Sun does.
type dnPlace struct {
	place  func() string
	locate func(string) (feeds.Place, error)

	mu    sync.Mutex
	asked string
	home  *feeds.Place
	busy  bool
	err   error
}

func (p *dnPlace) where() (*feeds.Place, error) {
	name := p.place()
	p.mu.Lock()
	defer p.mu.Unlock()
	if name != p.asked && !p.busy {
		p.asked, p.busy, p.home, p.err = name, true, nil, nil
		go func() {
			pl, err := p.locate(name)
			p.mu.Lock()
			defer p.mu.Unlock()
			p.busy, p.err = false, err
			if err == nil {
				p.home = &pl
			} else {
				p.asked = "" // try again
			}
		}()
	}
	return p.home, p.err
}

// dnCities places common time zones on the map (roughly, at the city they are named after).
var dnCities = map[string][2]float64{ // lat, lon
	"Europe/London": {51.5, -0.1}, "Europe/Dublin": {53.3, -6.3}, "Europe/Lisbon": {38.7, -9.1},
	"Europe/Paris": {48.9, 2.4}, "Europe/Madrid": {40.4, -3.7}, "Europe/Berlin": {52.5, 13.4},
	"Europe/Amsterdam": {52.4, 4.9}, "Europe/Brussels": {50.8, 4.4}, "Europe/Zurich": {47.4, 8.5},
	"Europe/Rome": {41.9, 12.5}, "Europe/Stockholm": {59.3, 18.1}, "Europe/Oslo": {59.9, 10.8},
	"Europe/Copenhagen": {55.7, 12.6}, "Europe/Helsinki": {60.2, 24.9}, "Europe/Warsaw": {52.2, 21.0},
	"Europe/Prague": {50.1, 14.4}, "Europe/Vienna": {48.2, 16.4}, "Europe/Athens": {38.0, 23.7},
	"Europe/Istanbul": {41.0, 29.0}, "Europe/Moscow": {55.8, 37.6}, "Europe/Kyiv": {50.5, 30.5},
	"America/New_York": {40.7, -74.0}, "America/Toronto": {43.7, -79.4}, "America/Chicago": {41.9, -87.6},
	"America/Denver": {39.7, -105.0}, "America/Phoenix": {33.4, -112.1}, "America/Los_Angeles": {34.1, -118.2},
	"America/Vancouver": {49.3, -123.1}, "America/Anchorage": {61.2, -149.9}, "Pacific/Honolulu": {21.3, -157.9},
	"America/Mexico_City": {19.4, -99.1}, "America/Bogota": {4.7, -74.1}, "America/Lima": {-12.0, -77.0},
	"America/Sao_Paulo": {-23.6, -46.6}, "America/Argentina/Buenos_Aires": {-34.6, -58.4},
	"America/Santiago": {-33.4, -70.7}, "America/Halifax": {44.6, -63.6}, "America/St_Johns": {47.6, -52.7},
	"Atlantic/Reykjavik": {64.1, -21.9}, "Africa/Cairo": {30.0, 31.2}, "Africa/Lagos": {6.5, 3.4},
	"Africa/Nairobi": {-1.3, 36.8}, "Africa/Johannesburg": {-26.2, 28.0}, "Africa/Casablanca": {33.6, -7.6},
	"Asia/Dubai": {25.2, 55.3}, "Asia/Tehran": {35.7, 51.4}, "Asia/Karachi": {24.9, 67.0},
	"Asia/Kolkata": {22.6, 88.4}, "Asia/Calcutta": {22.6, 88.4}, "Asia/Dhaka": {23.8, 90.4},
	"Asia/Bangkok": {13.8, 100.5}, "Asia/Jakarta": {-6.2, 106.8}, "Asia/Singapore": {1.3, 103.8},
	"Asia/Kuala_Lumpur": {3.1, 101.7}, "Asia/Hong_Kong": {22.3, 114.2}, "Asia/Shanghai": {31.2, 121.5},
	"Asia/Taipei": {25.0, 121.6}, "Asia/Manila": {14.6, 121.0}, "Asia/Seoul": {37.6, 127.0},
	"Asia/Tokyo": {35.7, 139.7}, "Asia/Jerusalem": {31.8, 35.2}, "Asia/Riyadh": {24.7, 46.7},
	"Australia/Perth": {-31.9, 115.9}, "Australia/Adelaide": {-34.9, 138.6}, "Australia/Brisbane": {-27.5, 153.0},
	"Australia/Sydney": {-33.9, 151.2}, "Australia/Melbourne": {-37.8, 145.0}, "Pacific/Auckland": {-36.8, 174.8},
	"UTC": {51.5, 0},
}

const dnSide = pad + 1300 + 40 // the info column

func (d *DayNight) Frame(t, dt float64, now time.Time) *image.RGBA {
	home, _ := d.home.where()
	key := now.Format("2006-01-02 15:04")
	if home != nil {
		key += home.Name
	}
	return d.cache.get(key, func(img *image.RGBA) { d.render(img, home, now) })
}

func (d *DayNight) render(img *image.RGBA, home *feeds.Place, now time.Time) {
	m := dnWorld
	sLat, sLon := SubSolar(now)
	mLat, mLon := SubLunar(now)
	m.shade(img, sLat, sLon)
	m.land(img, dnCoast, dnGrat)

	// the sun and moon overhead
	x, y := m.xy(sLon, sLat)
	disc(img, x, y, 16, mix(dnDay, amber, 0.4))
	disc(img, x, y, 10, color.RGBA{255, 222, 110, 255})
	x, y = m.xy(mLon, mLat)
	disc(img, x, y, 8, txt2)

	// the clocks
	var zones []Zone
	if d.zones != nil {
		zones = d.zones()
	}
	for _, z := range zones {
		ll, ok := dnCities[z.Loc.String()]
		if !ok {
			continue
		}
		x, y := m.xy(ll[1], ll[0])
		lt := now.In(z.Loc)
		e := SunElevation(now, ll[0], ll[1])
		c := cyan
		if e > 0 {
			c = amber
		}
		disc(img, x, y, 5, c)
		lbl := dnFitLabel(fmt.Sprintf("%s %s", z.Label, lt.Format("15:04")), 18)
		lx := x + 9
		if lx+measure(lbl, 18) > float64(m.x+m.w) {
			lx = x - 9 - measure(lbl, 18)
		}
		text(img, lx, int(y)-11, lbl, 18, txt)
	}
	if home != nil {
		x, y := m.xy(home.Longitude, home.Latitude)
		ring(img, int(x), int(y), 9, green)
		disc(img, x, y, 4, green)
	}

	// the column
	sx := dnSide
	w := W - pad - sx
	head(img, sx, pad-4, "day and night", w)
	text(img, float64(sx), 70, now.Format("15:04"), 72, amber)
	text(img, float64(sx)+measure(now.Format("15:04"), 72)+16, 108, now.Format("MST"), 22, txt3)
	y0 := 172
	text(img, float64(sx), y0, "SUN OVERHEAD", 18, cyan)
	text(img, float64(sx), y0+24, dnLatLon(sLat, sLon), 24, txt)
	text(img, float64(sx), y0+64, "MOON OVERHEAD", 18, cyan)
	text(img, float64(sx), y0+88, dnLatLon(mLat, mLon), 24, txt2)
	if home != nil {
		e := SunElevation(now, home.Latitude, home.Longitude)
		state, c := "DAYLIGHT", amber
		switch {
		case e <= -18:
			state, c = "NIGHT", violet
		case e <= -12:
			state, c = "ASTRONOMICAL TWILIGHT", violet
		case e <= -6:
			state, c = "NAUTICAL TWILIGHT", violet
		case e <= 0:
			state, c = "CIVIL TWILIGHT", cyan
		}
		text(img, float64(sx), y0+128, fit(strings.ToUpper(home.Name), 18, float64(w)), 18, green)
		text(img, float64(sx), y0+152, fit(state, 24, float64(w)), 24, c)
		text(img, float64(sx), y0+184, fmt.Sprintf("SUN %+.0f°", e), 20, txt2)
	}
	// legend
	ly := H - pad - 16
	lx := float64(sx)
	for _, l := range []struct {
		c color.RGBA
		s string
	}{{dnDay, "DAY"}, {dnCivil, "CIVIL"}, {dnNaut, "NAUT"}, {dnNight, "NIGHT"}} {
		rect(img, int(lx), ly, int(lx)+16, ly+14, mix(l.c, txt, 0.3))
		text(img, lx+22, ly-2, l.s, 16, txt3)
		lx += 30 + measure(l.s, 16) + 10
	}
}

func dnFitLabel(s string, px int) string { return fit(s, px, 260) }
