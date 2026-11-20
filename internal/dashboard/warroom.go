package dashboard

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"math/rand/v2"
	"time"
)

// WarRoom is a cold-war strategic simulation on a vector map: the alert climbs through the
// DEFCON levels, two blocs trade waves of missiles in growing numbers, impacts bloom, and every
// run ends the same way, with no winner. Then the next scenario begins.
type WarRoom struct {
	base   *image.RGBA // the map, drawn once
	out    *image.RGBA
	sim    int
	start  float64 // t the current simulation started
	shots  []shot
	rng    *rand.Rand
	casual float64 // millions
}

type shot struct {
	at, dur  float64 // launch time (seconds into the run) and flight time
	x0, y0   float64
	x1, y1   float64
	east     bool
	label    string
	impacted bool
}

const (
	wrMapX = pad
	wrMapY = 34
	wrMapW = 1290
	wrMapH = 410
	wrLonL = -170.0 // the map's left edge
	wrLatT = 76.0
	wrLatB = -46.0
	wrSide = wrMapX + wrMapW + 34 // the status column

	wrDefcon = 9.0  // seconds of rising alert
	wrWar    = 22.0 // seconds of launches
	wrRun    = 46.0 // whole simulation
)

var (
	vector  = color.RGBA{70, 200, 160, 255}
	vectorD = color.RGBA{30, 84, 68, 255}
	west    = color.RGBA{96, 200, 255, 255}
	east    = color.RGBA{255, 176, 64, 255}
)

func wrXY(lon, lat float64) (float64, float64) {
	if lon < wrLonL {
		lon += 360
	}
	return wrMapX + (lon-wrLonL)/360*wrMapW, wrMapY + (wrLatT-lat)/(wrLatT-wrLatB)*wrMapH
}

func NewWarRoom() *WarRoom {
	w := &WarRoom{base: image.NewRGBA(image.Rect(0, 0, W, H)), out: image.NewRGBA(image.Rect(0, 0, W, H)), start: -1}
	fill(w.base, bg)
	// graticule
	for lon := -150.0; lon <= 180; lon += 30 {
		x, _ := wrXY(lon, 0)
		for y := wrMapY; y < wrMapY+wrMapH; y += 6 {
			rect(w.base, int(x), y, int(x), y+1, vectorD)
		}
	}
	for lat := 60.0; lat >= -30; lat -= 30 {
		_, y := wrXY(0, lat)
		for x := wrMapX; x < wrMapX+wrMapW; x += 6 {
			rect(w.base, x, int(y), x+1, int(y), vectorD)
		}
	}
	for _, poly := range landOutlines {
		var pts [][2]float64
		prev := 0.0
		for i := 0; i+1 < len(poly); i += 2 {
			x, y := wrXY(poly[i], poly[i+1])
			if len(pts) > 0 && math.Abs(x-prev) > wrMapW/2 { // wrapped round the edge
				polyline(w.base, pts, vector, 2)
				pts = pts[:0]
			}
			pts = append(pts, [2]float64{x, y})
			prev = x
		}
		polyline(w.base, pts, vector, 2)
	}
	return w
}

// sites are launch areas (lon, lat), loosely placed; targets are drawn near the other side's.
var (
	westSites = [][2]float64{{-101, 47}, {-105, 41}, {-112, 46}, {-97, 38}, {-150, 38}, {-40, 42}, {-25, 60}, {2, 50}, {-2, 54}}
	eastSites = [][2]float64{{60, 56}, {85, 54}, {104, 52}, {40, 58}, {130, 46}, {110, 37}, {160, 50}, {45, 66}, {75, 42}}
)

// begin lays out a new simulation: launches in waves that grow, each side answering the other.
func (w *WarRoom) begin(t float64) {
	w.sim++
	w.start = t
	w.rng = rand.New(rand.NewPCG(uint64(w.sim)*7919, uint64(time.Now().UnixNano())))
	w.shots = w.shots[:0]
	w.casual = 0
	firstEast := w.rng.IntN(2) == 0
	waves := 7
	for wave := 0; wave < waves; wave++ {
		n := 1 + wave*wave/2 + w.rng.IntN(2)
		for side := 0; side < 2; side++ {
			isEast := (side == 0) == firstEast
			from, to := westSites, eastSites
			if isEast {
				from, to = eastSites, westSites
			}
			for range n {
				a := from[w.rng.IntN(len(from))]
				b := to[w.rng.IntN(len(to))]
				x0, y0 := wrXY(a[0]+w.rng.Float64()*6-3, a[1]+w.rng.Float64()*4-2)
				x1, y1 := wrXY(b[0]+w.rng.Float64()*16-8, b[1]+w.rng.Float64()*10-5)
				at := wrDefcon + float64(wave)*wrWar/float64(waves) + float64(side)*1.1 + w.rng.Float64()*1.4
				w.shots = append(w.shots, shot{at: at, dur: 5 + w.rng.Float64()*3, x0: x0, y0: y0, x1: x1, y1: y1, east: isEast,
					label: fmt.Sprintf("%s-%02d", map[bool]string{false: "W", true: "E"}[isEast], len(w.shots)+1)})
			}
		}
	}
}

// arc is a point along a shot's ballistic-looking path at u in [0, 1].
func (s shot) arc(u float64) (float64, float64) {
	mx, my := (s.x0+s.x1)/2, (s.y0+s.y1)/2
	// a quadratic's peak is half its control point's lift: keep the peak on the panel
	lift := math.Min(math.Hypot(s.x1-s.x0, s.y1-s.y0)*0.38, 2*(my-8))
	cx, cy := mx, my-lift
	a, b, c := (1-u)*(1-u), 2*u*(1-u), u*u
	return a*s.x0 + b*cx + c*s.x1, a*s.y0 + b*cy + c*s.y1
}

func (w *WarRoom) Frame(t, dt float64, now time.Time) *image.RGBA {
	if w.start < 0 || t-w.start > wrRun {
		w.begin(t)
	}
	rt := t - w.start
	copy(w.out.Pix, w.base.Pix)

	launched := [2]int{}
	impacts := 0
	var log []string
	for i := range w.shots {
		s := &w.shots[i]
		if rt < s.at {
			continue
		}
		side := 0
		c := west
		if s.east {
			side, c = 1, east
		}
		launched[side]++
		u := (rt - s.at) / s.dur
		if u < 1 {
			// trail, then the warhead
			var pts [][2]float64
			for k := 0; k <= 24; k++ {
				x, y := s.arc(u * float64(k) / 24)
				pts = append(pts, [2]float64{x, y})
			}
			polyline(w.out, pts, mix(bg, c, 0.55), 2)
			hx, hy := s.arc(u)
			disc(w.out, hx, hy, 3.5, c)
			log = append(log, fmt.Sprintf("%s  IN FLIGHT", s.label))
			continue
		}
		impacts++
		if !s.impacted {
			s.impacted = true
			w.casual += 2 + w.rng.Float64()*9
		}
		// the path stays faint; the impact blooms, then glows
		var pts [][2]float64
		for k := 0; k <= 24; k++ {
			x, y := s.arc(float64(k) / 24)
			pts = append(pts, [2]float64{x, y})
		}
		polyline(w.out, pts, mix(bg, c, 0.18), 1)
		age := rt - s.at - s.dur
		if age < 1.4 {
			k := age / 1.4
			disc(w.out, s.x1, s.y1, 3+10*k, mix(bg, color.RGBA{255, 245, 220, 255}, 1-k))
			ring(w.out, int(s.x1), int(s.y1), 6+26*k, mix(bg, hot, 1-k))
			log = append(log, fmt.Sprintf("%s  DETONATION", s.label))
		} else {
			disc(w.out, s.x1, s.y1, 4, mix(bg, hot, 0.6))
		}
	}

	w.status(rt, launched, impacts, log)
	return w.out
}

func (w *WarRoom) status(rt float64, launched [2]int, impacts int, log []string) {
	x := float64(wrSide)
	vrule(w.out, wrSide-18)
	head(w.out, wrSide, pad-4, fmt.Sprintf("strategic simulation  %03d", w.sim), W-pad-wrSide)

	defcon := 5 - int(math.Min(4, rt/(wrDefcon/4)))
	dc := []color.RGBA{hot, hot, amber, amber, green}[defcon-1]
	text(w.out, x, 70, "DEFCON", 22, txt2)
	text(w.out, x+120, 52, fmt.Sprint(defcon), 76, dc)
	if rt < wrDefcon && int(rt*2)%2 == 0 {
		text(w.out, x+200, 76, "ALERT RISING", 22, amber)
	}

	y := 150
	row := func(label, v string, c color.RGBA) {
		text(w.out, x, y, label, 20, cyan)
		text(w.out, float64(W-pad)-measure(v, 26), y-4, v, 26, c)
		y += 36
	}
	row("LAUNCHED  WEST", fmt.Sprint(launched[0]), west)
	row("LAUNCHED  EAST", fmt.Sprint(launched[1]), east)
	row("DETONATIONS", fmt.Sprint(impacts), hot)
	row("CASUALTIES EST.", fmt.Sprintf("%.0f M", w.casual), txt)

	if rt > wrDefcon+wrWar+6 { // the verdict, typed out
		lines := []string{"RESULT: NO SURVIVING WINNER", "RECOMMENDATION: STAND DOWN"}
		shown := int((rt - (wrDefcon + wrWar + 6)) * 18)
		yy := 330
		for _, l := range lines {
			n := min(len(l), max(shown, 0))
			text(w.out, x, yy, l[:n], 24, txt)
			shown -= len(l) + 6
			yy += 38
		}
		return
	}
	// event log, newest last
	if len(log) > 4 {
		log = log[len(log)-4:]
	}
	for i, l := range log {
		text(w.out, x, 318+i*28, l, 18, txt3)
	}
}

// landOutlines are rough coastlines as lon, lat pairs: enough for a vector war-room map.
var landOutlines = [][]float64{
	// North America
	{-168, 66, -156, 71, -130, 70, -110, 72, -90, 68, -80, 63, -72, 60, -64, 58, -56, 52, -60, 46, -66, 44, -70, 41, -75, 38, -76, 35, -81, 31, -80, 26, -82, 27, -84, 30, -89, 30, -94, 29, -97, 26, -97, 22, -94, 18, -90, 21, -87, 21, -88, 16, -83, 15, -83, 10, -79, 9, -80, 8, -85, 11, -92, 15, -99, 17, -105, 20, -106, 23, -110, 24, -112, 29, -114, 31, -117, 32, -120, 34, -124, 40, -124, 46, -125, 49, -130, 54, -135, 58, -142, 60, -150, 60, -155, 58, -160, 56, -165, 54, -162, 58, -166, 61, -168, 66},
	// Greenland
	{-73, 78, -60, 82, -30, 83, -20, 80, -18, 75, -22, 70, -30, 68, -40, 65, -45, 60, -50, 63, -54, 67, -58, 73, -73, 78},
	// Iceland
	{-24, 65.5, -18, 66.5, -13.5, 65, -18.5, 63.4, -22.5, 63.8, -24, 65.5},
	// Cuba
	{-85, 21.8, -80, 23, -74.1, 20.2, -77.5, 19.9, -84.9, 21.3, -85, 21.8},
	// South America
	{-80, 9, -75, 11, -71, 12, -63, 10, -60, 8, -52, 4, -50, 0, -45, -2, -38, -4, -35, -7, -37, -12, -39, -17, -41, -22, -45, -24, -48, -27, -52, -33, -57, -37, -62, -39, -65, -45, -68, -50, -69, -54, -72, -53, -74, -48, -73, -40, -72, -30, -71, -20, -75, -15, -77, -10, -81, -5, -80, 0, -78, 3, -77, 7, -80, 9},
	// Britain and Ireland
	{-5.7, 50, 1.4, 51.2, 1.7, 52.7, 0, 53.5, -1.6, 55.5, -2, 57.5, -3.5, 58.6, -5, 58.6, -6.2, 57, -5.5, 55.3, -3, 54.9, -3.2, 53.4, -4.6, 53.2, -4.6, 51.7, -5.7, 50},
	{-10, 51.6, -6, 52, -6, 53.9, -7.5, 55.2, -10, 54.2, -10, 51.6},
	// Eurasia
	{-9, 37, -9, 43, -2, 43.5, -1.5, 46, -4.5, 48, -1, 49.5, 2, 51, 5, 53, 8, 54, 8.5, 57, 10.5, 57.5, 10.5, 54.5, 14, 54, 19, 54.5, 21, 57, 24, 57.5, 24, 59.5, 29, 60, 23, 60.5, 21.5, 63, 25, 65.5, 21, 65.5, 17, 62, 18.5, 60, 16, 56.5, 12.5, 56, 11, 59, 5.5, 58.5, 5, 62, 10, 64, 14, 67.5, 18, 70, 26, 71, 31, 70, 40, 67.5, 43, 68.5, 54, 68.8, 60, 69.8, 68, 71, 73, 72.5, 80, 73, 88, 75, 100, 77, 110, 76.5, 113, 73.8, 128, 72.5, 140, 72.5, 150, 71.5, 160, 70, 170, 70, 180, 68.5, 180, 65, 178, 64.5, 172, 64.5, 178, 62, 170, 60, 163, 60, 162, 57, 156, 51, 156, 57.5, 160, 61, 155, 59.5, 143, 59.5, 137, 54, 141, 52, 140, 48, 135, 43.5, 130, 42.5, 129, 40, 129.5, 35, 126.5, 34.5, 126, 37.5, 125, 39.5, 121.5, 39, 122, 37, 119, 37, 121, 34, 122, 30, 119.5, 25.5, 117, 23.5, 111, 21.5, 108.5, 21.5, 106.5, 17, 109, 12, 105, 8.5, 103, 10.5, 101, 13.5, 100, 10, 103, 5, 103.5, 1.5, 101, 3, 98, 8, 98.5, 13, 97.5, 16.5, 94.5, 16, 94, 19, 92, 22, 89, 22, 87, 21.5, 85, 19.5, 80.3, 15.5, 80, 10, 77.5, 8, 76, 10, 74, 15, 72.8, 19, 72.5, 21.5, 70, 22.5, 68.5, 23.5, 66.5, 25.5, 61.5, 25.2, 57, 25.8, 56.3, 27.2, 51.5, 27.8, 48.5, 30, 50, 26.5, 51.5, 24.5, 54.5, 24.3, 56.3, 26.3, 56.5, 24.5, 59.8, 22.5, 57, 18.8, 55, 17, 52, 15.8, 48.8, 14, 45, 12.8, 43.3, 12.7, 42.5, 15.5, 39.2, 21.5, 37, 25.5, 35, 28, 34.5, 29.5, 32.5, 30, 34, 31.2, 35.5, 33.8, 36, 36.5, 32.5, 36.2, 30, 36.3, 27, 37, 26.5, 39.5, 29, 41, 31, 41.2, 36, 41.5, 41.5, 41.5, 39, 44.5, 37.5, 47, 35, 45, 33, 45.5, 31, 46.6, 29.6, 45, 28, 43, 28, 41.5, 26, 40.6, 23, 40, 24, 38, 22.5, 36.5, 21, 38.5, 19.5, 40.5, 19, 42, 13.5, 45.5, 12.3, 45, 14, 42, 16, 41, 18.5, 40, 15.6, 38, 16, 39, 12.5, 41.5, 10.5, 43, 8.5, 44.3, 4, 43.5, 3, 42.5, 3, 41.8, 0, 39.5, -0.5, 37.6, -2, 36.7, -5.5, 36, -9, 37},
	// Africa
	{-17, 21, -16, 24, -13, 28, -9.5, 30, -9.5, 33, -6, 35.8, -2, 35, 3, 36.8, 10, 37.2, 11, 35, 10, 33.5, 15, 32, 20, 30.5, 20, 32.3, 25, 31.7, 32, 31.3, 32.5, 30, 34.5, 28, 33.5, 27, 35.5, 23.5, 37, 21, 38.5, 18, 39.5, 15.5, 42.5, 12.5, 43.3, 11.5, 44.5, 10.4, 51, 11.8, 51, 10, 48, 5, 45.5, 2, 41.5, -1.8, 40, -4, 39, -7, 40.5, -10.5, 40.5, -15, 35, -20, 35.5, -24, 32.8, -26, 32.5, -28.8, 30, -31.3, 27, -33.6, 23, -34, 20, -34.8, 18.4, -34, 17.8, -31, 16.5, -28.6, 15, -26.5, 14.5, -22.8, 12, -18, 11.8, -15.8, 13.5, -12, 13.2, -8.6, 12.2, -6, 9.6, -2.5, 9.4, 1, 9.8, 3.2, 8.5, 4.5, 5.9, 4.3, 4.5, 6.3, 2, 6.2, -1, 5, -4, 5.2, -7.5, 4.4, -9.5, 5.6, -11.5, 6.9, -13, 8.4, -15, 11, -16.8, 13, -17.5, 14.7, -16.5, 16.2, -16, 19, -17, 21},
	// Madagascar
	{44, -25, 43.3, -22, 44.4, -16.2, 47, -15.5, 49.5, -12, 50.5, -15.5, 47, -25, 44, -25},
	// Japan
	{130, 31, 132, 34, 135.5, 34.6, 140, 35.5, 141, 38.5, 141.5, 41.5, 140, 40, 139.5, 38, 137, 37, 135.5, 35.6, 132.5, 35.5, 130.5, 34, 130, 31},
	{140, 42, 141.5, 45.4, 145.5, 43.5, 143.5, 42, 140, 42},
	// Sumatra, Borneo, Java, New Guinea
	{95.3, 5.6, 98, 4, 104, -2, 106, -6, 104.5, -5.9, 101, -2, 98, 1, 95.3, 5.6},
	{109, 1.5, 110, -2, 114, -3.5, 116.5, -2.5, 118.8, 1, 117.8, 4.5, 116, 6.8, 113.5, 4, 109.6, 2, 109, 1.5},
	{105.5, -6.7, 114.5, -7.8, 114.5, -8.6, 106, -7.5, 105.5, -6.7},
	{131, -1.5, 138, -1.6, 144, -3.8, 150.5, -10.6, 147, -10, 141, -9, 138, -8.4, 135, -4.3, 132, -4, 131, -1.5},
	// Australia
	{113.5, -22, 114, -26.5, 115, -30, 115, -33.6, 117.5, -35, 123, -33.8, 129, -31.6, 132, -32, 135, -34.5, 138, -35.6, 140, -38, 144, -38.3, 146.5, -39, 150, -37.5, 151, -34, 153, -31, 153, -25.5, 150.5, -22.4, 146.3, -19, 145.3, -15, 143.5, -14, 142.5, -10.8, 141.5, -13, 141.5, -17, 140, -17.7, 136.5, -15.8, 137, -12.3, 132.6, -11.5, 131, -12.2, 129.5, -15, 128, -15.2, 126, -14, 124, -16.3, 122.2, -18, 121, -19.5, 118, -20.4, 114.6, -21.8, 113.5, -22},
	// New Zealand
	{172.7, -34.4, 178.5, -37.7, 177, -39.5, 175, -41.5, 172.5, -40.5, 174.5, -38.5, 172.7, -34.4},
	{172, -41, 174.3, -41.7, 170.5, -45.9, 166.5, -46, 168, -44, 172, -41},
}
