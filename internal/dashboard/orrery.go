package dashboard

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"sort"
	"time"
)

// Orrery is the solar system now, seen from above and a little to one side: the planets on
// their orbits (distances from the sun compressed so Neptune and Mercury both fit), each with a
// short trail of where it has been, slowly turning; and in the column how far each planet is
// from Earth and whether it is in the evening or morning sky.
type Orrery struct {
	cache  frameCache
	orbits [][][3]float64 // per planet, points round its orbit (AU, ecliptic)
	at     string         // minute the positions are for
	pos    [][3]float64
	trails [][][3]float64
}

func NewOrrery() *Orrery {
	o := &Orrery{}
	for _, p := range orrPlanets {
		var pts [][3]float64
		for i := 0; i <= 160; i++ {
			pts = append(pts, p.at(2000, float64(i)/160*360))
		}
		o.orbits = append(o.orbits, pts)
	}
	return o
}

// orrPlanet holds JPL's approximate Keplerian elements (valid 1800-2050) at J2000 and their
// rates per Julian century: a (AU), e, I, L, long. perihelion, long. node (degrees).
type orrPlanet struct {
	name     string
	c        color.RGBA
	size     float64
	el, rate [6]float64
}

var orrPlanets = []orrPlanet{
	{"MERCURY", color.RGBA{200, 190, 176, 255}, 5, [6]float64{0.38709927, 0.20563593, 7.00497902, 252.25032350, 77.45779628, 48.33076593}, [6]float64{0.00000037, 0.00001906, -0.00594749, 149472.67411175, 0.16047689, -0.12534081}},
	{"VENUS", color.RGBA{255, 222, 150, 255}, 7, [6]float64{0.72333566, 0.00677672, 3.39467605, 181.97909950, 131.60246718, 76.67984255}, [6]float64{0.00000390, -0.00004107, -0.00078890, 58517.81538729, 0.00268329, -0.27769418}},
	{"EARTH", color.RGBA{86, 170, 255, 255}, 7.5, [6]float64{1.00000261, 0.01671123, -0.00001531, 100.46457166, 102.93768193, 0.0}, [6]float64{0.00000562, -0.00004392, -0.01294668, 35999.37244981, 0.32327364, 0.0}},
	{"MARS", color.RGBA{255, 112, 72, 255}, 6, [6]float64{1.52371034, 0.09339410, 1.84969142, -4.55343205, -23.94362959, 49.55953891}, [6]float64{0.00001847, 0.00007882, -0.00813131, 19140.30268499, 0.44441088, -0.29257343}},
	{"JUPITER", color.RGBA{236, 186, 140, 255}, 13, [6]float64{5.20288700, 0.04838624, 1.30439695, 34.39644051, 14.72847983, 100.47390909}, [6]float64{-0.00011607, -0.00013253, -0.00183714, 3034.74612775, 0.21252668, 0.20469106}},
	{"SATURN", color.RGBA{240, 214, 150, 255}, 11, [6]float64{9.53667594, 0.05386179, 2.48599187, 49.95424423, 92.59887831, 113.66242448}, [6]float64{-0.00125060, -0.00050991, 0.00193609, 1222.49362201, -0.41897216, -0.28867794}},
	{"URANUS", color.RGBA{150, 230, 236, 255}, 9, [6]float64{19.18916464, 0.04725744, 0.77263783, 313.23810451, 170.95427630, 74.01692503}, [6]float64{-0.00196176, -0.00004397, -0.00242939, 428.48202785, 0.40805281, 0.04240589}},
	{"NEPTUNE", color.RGBA{110, 140, 255, 255}, 9, [6]float64{30.06992276, 0.00859048, 1.77004347, -55.12002969, 44.96476227, 131.78422574}, [6]float64{0.00026291, 0.00005105, 0.00035372, 218.45945325, -0.32241464, -0.01262724}},
}

const orrEarth = 2

// orrCenturies is Julian centuries since J2000 (TT taken as UTC: near enough here).
func orrCenturies(t time.Time) float64 {
	return (float64(t.UnixNano())/86400e9 + 2440587.5 - 2451545.0) / 36525
}

// Position is the planet's heliocentric ecliptic position (AU) at t.
func (p orrPlanet) Position(t time.Time) [3]float64 {
	T := orrCenturies(t)
	el := p.el
	for i := range el {
		el[i] += p.rate[i] * T
	}
	L, peri := el[3], el[4]
	return p.at(T*100+2000, L-peri)
}

// at is the position with mean anomaly M (degrees) in year (for the slowly changing elements).
func (p orrPlanet) at(year, M float64) [3]float64 {
	T := (year - 2000) / 100
	el := p.el
	for i := range el {
		el[i] += p.rate[i] * T
	}
	a, e, I, _, peri, node := el[0], el[1], el[2], el[3], el[4], el[5]
	rad := math.Pi / 180
	w := (peri - node) * rad
	m := math.Mod(M+180, 360)
	if m < 0 {
		m += 360
	}
	m = (m - 180) * rad
	E := m + e*math.Sin(m)
	for range 8 {
		E -= (E - e*math.Sin(E) - m) / (1 - e*math.Cos(E))
	}
	x, y := a*(math.Cos(E)-e), a*math.Sqrt(1-e*e)*math.Sin(E)
	cw, sw := math.Cos(w), math.Sin(w)
	cO, sO := math.Cos(node*rad), math.Sin(node*rad)
	cI, sI := math.Cos(I*rad), math.Sin(I*rad)
	return [3]float64{
		(cw*cO-sw*sO*cI)*x + (-sw*cO-cw*sO*cI)*y,
		(cw*sO+sw*cO*cI)*x + (-sw*sO+cw*cO*cI)*y,
		(sw*sI)*x + (cw*sI)*y,
	}
}

func orrDist(a, b [3]float64) float64 {
	return math.Sqrt((a[0]-b[0])*(a[0]-b[0]) + (a[1]-b[1])*(a[1]-b[1]) + (a[2]-b[2])*(a[2]-b[2]))
}

// Elongation is the angle between the sun and the planet seen from Earth, in degrees: positive
// east of the sun (the evening sky), negative west (the morning sky).
func orrElongation(earth, planet [3]float64) float64 {
	sun := math.Atan2(-earth[1], -earth[0])
	pl := math.Atan2(planet[1]-earth[1], planet[0]-earth[0])
	d := math.Mod((pl-sun)*180/math.Pi+540, 360) - 180
	return d
}

const (
	orrCX  = 650.0 // the view's centre
	orrCY  = 236.0
	orrRX  = 590.0 // Neptune's radius across
	orrRY  = 190.0 // and front to back
	orrMax = 30.5  // AU at the edge
	orrX2  = 1340  // the column
)

// orrXY projects a heliocentric point, turning the view by rot radians.
func orrXY(p [3]float64, rot float64) (float64, float64, float64) {
	r := math.Hypot(p[0], p[1])
	a := math.Atan2(p[1], p[0]) + rot
	s := math.Sqrt(r/orrMax) * 0.97
	if r > 0 {
		s += 0.03 // keep Mercury off the sun
	}
	return orrCX + s*orrRX*math.Cos(a), orrCY + s*orrRY*math.Sin(a), math.Sin(a)
}

func (o *Orrery) update(now time.Time) {
	key := now.Format("2006-01-02 15:04")
	if key == o.at {
		return
	}
	o.at = key
	o.pos, o.trails = nil, nil
	for _, p := range orrPlanets {
		o.pos = append(o.pos, p.Position(now))
		// a trail of the last twentieth of an orbit
		period := 365.25 * math.Pow(p.el[0], 1.5)
		var tr [][3]float64
		for k := 24; k >= 1; k-- {
			tr = append(tr, p.Position(now.Add(-time.Duration(float64(k)/24*period/20*86400)*time.Second)))
		}
		o.trails = append(o.trails, tr)
	}
}

func (o *Orrery) Frame(t, dt float64, now time.Time) *image.RGBA {
	o.update(now)
	img := o.cache.get(o.at, func(img *image.RGBA) { o.column(img, now) })
	rot := t * 2 * math.Pi / 240 // once round every four minutes

	// orbits
	for i, orb := range o.orbits {
		pts := make([][2]float64, len(orb))
		for k, q := range orb {
			x, y, _ := orrXY(q, rot)
			pts[k] = [2]float64{x, y}
		}
		c := mix(bg, orrPlanets[i].c, 0.32)
		polyline(img, pts, c, 2)
	}
	// the sun
	disc(img, orrCX, orrCY, 22, mix(bg, amber, 0.35))
	disc(img, orrCX, orrCY, 14, color.RGBA{255, 214, 100, 255})

	// planets back to front
	idx := make([]int, len(o.pos))
	for i := range idx {
		idx[i] = i
	}
	depth := func(i int) float64 { _, _, d := orrXY(o.pos[i], rot); return d }
	sort.Slice(idx, func(a, b int) bool { return depth(idx[a]) < depth(idx[b]) })
	type box struct{ x0, y0, x1, y1 float64 }
	taken := []box{{orrCX - 24, orrCY - 24, orrCX + 24, orrCY + 24}}
	for _, i := range idx {
		x, y, _ := orrXY(o.pos[i], rot)
		taken = append(taken, box{x - 8, y - 8, x + 8, y + 8})
	}
	label := func(s string, x, y, r float64) {
		w := measure(s, 16)
		for _, c := range [][2]float64{{x + r + 6, y - r - 18}, {x + r + 6, y + r}, {x - r - 6 - w, y - r - 18}, {x - r - 6 - w, y + r}} {
			b := box{c[0], c[1], c[0] + w, c[1] + 18}
			if b.x1 > orrX2-40 || b.x0 < pad {
				continue
			}
			clash := false
			for _, t := range taken {
				if b.x0 < t.x1 && t.x0 < b.x1 && b.y0 < t.y1 && t.y0 < b.y1 {
					clash = true
					break
				}
			}
			if !clash {
				taken = append(taken, b)
				text(img, c[0], int(c[1]), s, 16, txt2)
				return
			}
		}
	}
	for _, i := range idx {
		p := orrPlanets[i]
		var tr [][2]float64
		for _, q := range o.trails[i] {
			x, y, _ := orrXY(q, rot)
			tr = append(tr, [2]float64{x, y})
		}
		x, y, d := orrXY(o.pos[i], rot)
		tr = append(tr, [2]float64{x, y})
		polyline(img, tr, mix(bg, p.c, 0.6), 3)
		r := p.size * (0.85 + 0.15*d)
		if p.name == "SATURN" {
			orrEllipse(img, x, y, r*2.1, r*0.7, mix(bg, p.c, 0.8))
		}
		disc(img, x, y, r, p.c)
		if i == orrEarth {
			ring(img, int(x), int(y), r+6+2*math.Sin(t*3), mix(bg, green, 0.8))
		}
	}
	// labels, outer planets first so the crowded middle gives way
	for i := len(o.pos) - 1; i >= 0; i-- {
		x, y, d := orrXY(o.pos[i], rot)
		label(orrPlanets[i].name, x, y, orrPlanets[i].size*(0.85+0.15*d))
	}
	return img
}

// orrEllipse is an outline ellipse (Saturn's rings).
func orrEllipse(img *image.RGBA, cx, cy, rx, ry float64, c color.RGBA) {
	var pts [][2]float64
	for i := 0; i <= 48; i++ {
		a := float64(i) / 48 * 2 * math.Pi
		pts = append(pts, [2]float64{cx + rx*math.Cos(a), cy + ry*math.Sin(a)})
	}
	polyline(img, pts, c, 2)
}

func (o *Orrery) column(img *image.RGBA, now time.Time) {
	head(img, pad, pad-4, "the solar system  "+now.Format("2 Jan 2006"), 600)
	text(img, pad, H-pad-14, "DISTANCES FROM THE SUN COMPRESSED", 16, txt3)
	x := orrX2
	vrule(img, x-30)
	head(img, x, pad-4, "from earth", 300)
	text(img, float64(x+150), pad, "AU", 16, txt3)
	text(img, float64(x+250), pad, "LIGHT", 16, txt3)
	text(img, float64(x+360), pad, "IN THE SKY", 16, txt3)
	earth := o.pos[orrEarth]
	y := 76
	for i, p := range orrPlanets {
		if i == orrEarth {
			continue
		}
		d := orrDist(earth, o.pos[i])
		el := orrElongation(earth, o.pos[i])
		text(img, float64(x), y, p.name, 22, p.c)
		text(img, float64(x+150), y, fmt.Sprintf("%.2f", d), 22, txt)
		text(img, float64(x+250), y, orrLight(d), 22, txt2)
		sky, sc := "EVENING", amber
		if el < 0 {
			sky, sc = "MORNING", cyan
		}
		if math.Abs(el) < 15 {
			sky, sc = "NEAR SUN", txt3
		} else {
			sky += fmt.Sprintf(" %.0f°", math.Abs(el))
		}
		text(img, float64(x+360), y, fit(sky, 20, float64(W-pad-x-360)), 20, sc)
		y += 52
	}
}

// orrLight is how long light takes to cover d AU.
func orrLight(d float64) string {
	m := d * 149597870.7 / 299792.458 / 60
	if m < 60 {
		return fmt.Sprintf("%.1fM", m)
	}
	return fmt.Sprintf("%dH%02dM", int(m/60), int(m)%60)
}
