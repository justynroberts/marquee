package scenes

// More camera scenes: a false-colour heat camera and a kaleidoscope.

import (
	"math"

	"github.com/justynroberts/marquee/internal/camera"
	"github.com/justynroberts/marquee/internal/led"
)

func init() {
	Register("thermal", "camera", "A heat camera: you glow white-hot against a cold room, in an iron false-colour palette", func() Scene { return &thermal{} })
	Register("kaleido", "camera", "The webcam through a slowly turning kaleidoscope", func() Scene { return &kaleido{} })
}

// ---------------------------------------------------------------------------
// thermal: brightness and the person mask mapped to an iron palette. With Vision's mask the
// body is hot and the room cold; without it, brightness alone sets the heat.

type thermal struct {
	px   []led.RGB
	heat []float64
	tmp  []float64
}

// ironLUT runs black, indigo, magenta, red, orange, yellow, white.
var ironLUT = func() (lut [256]led.RGB) {
	stops := []struct {
		p       float64
		r, g, b float64
	}{{0, 0, 0, 0}, {0.18, 30, 0, 90}, {0.38, 150, 0, 140}, {0.55, 230, 30, 40}, {0.72, 255, 120, 0}, {0.88, 255, 220, 40}, {1, 255, 255, 235}}
	for i := range lut {
		v := float64(i) / 255
		for j := 1; j < len(stops); j++ {
			a, b := stops[j-1], stops[j]
			if v <= b.p {
				k := (v - a.p) / (b.p - a.p)
				lut[i] = led.RGB{R: u8(a.r + (b.r-a.r)*k), G: u8(a.g + (b.g-a.g)*k), B: u8(a.b + (b.b-a.b)*k)}
				break
			}
		}
	}
	return
}()

func (s *thermal) Draw(g *led.Grid, t, dt float64) {
	f := CameraSource(camera.Mask)
	if cameraProblem(g, f, t) {
		return
	}
	if s.px == nil {
		s.px = make([]led.RGB, rows*cols)
		s.heat = make([]float64, rows*cols)
		s.tmp = make([]float64, rows*cols)
	}
	v := fitView(f, 1.6)
	lo, hi := 255.0, 0.0
	for y := range rows {
		for x := range cols {
			fx, fy := v.frame(x, y)
			l := -1.0
			if fx >= 0 && fx < f.W && fy >= 0 && fy < f.H {
				r, gg, b := f.At(fx, fy)
				l = luma(r, gg, b)
				lo, hi = math.Min(lo, l), math.Max(hi, l)
			}
			s.tmp[y*cols+x] = l
		}
	}
	span := math.Max(hi-lo, 24)
	for y := range rows {
		for x := range cols {
			i := y*cols + x
			l := s.tmp[i]
			h := 0.0
			if l >= 0 {
				n := (l - lo) / span
				h = 0.08 + n*0.42
				if f.Mask != nil {
					fx, fy := v.frame(x, y)
					if f.MaskAt(fx, fy) {
						h = 0.62 + n*0.38 // the body runs hot, its brighter parts hottest
					} else {
						h = 0.04 + n*0.3
					}
				}
			}
			s.heat[i] = h
		}
	}
	// a little blur so edges bloom like a real sensor
	for y := range rows {
		for x := range cols {
			sum, n := 0.0, 0.0
			for dy := -1; dy <= 1; dy++ {
				for dx := -1; dx <= 1; dx++ {
					xx, yy := x+dx, y+dy
					if xx >= 0 && xx < cols && yy >= 0 && yy < rows {
						sum += s.heat[yy*cols+xx]
						n++
					}
				}
			}
			c := ironLUT[int(math.Min(1, sum/n)*255)]
			if c.Luma() >= 10 {
				g.Set(x, y, c)
			}
		}
	}
	// scale down the right-hand edge
	for y := 4; y < rows-4; y++ {
		c := ironLUT[int(float64(rows-5-y)/float64(rows-9)*255)]
		g.Fill(cols-4, y, cols-1, y+1, c)
	}
}

// ---------------------------------------------------------------------------
// kaleido: each dot is folded into one mirrored wedge around the centre, which turns slowly,
// and sampled from the middle of the frame.

type kaleido struct {
	lo, hi float64
}

const kaleidoWedges = 8

func (s *kaleido) Draw(g *led.Grid, t, dt float64) {
	f := CameraSource(0)
	if cameraProblem(g, f, t) {
		return
	}
	cx, cy := float64(cols)/2, float64(rows)/2
	wedge := 2 * math.Pi / kaleidoWedges
	rot := t * 0.15
	// sample radius: the panel's half-width covers a disc of 45% of the frame height
	scale := float64(f.H) * 0.45 / (float64(cols) / 2)
	zoom := 1 + 0.25*math.Sin(t*0.3)
	fcx, fcy := float64(f.W)/2, float64(f.H)/2
	lo, hi := 255.0, 0.0
	type px struct {
		x, y    int
		r, g, b uint8
	}
	var buf [rows * cols]px
	n := 0
	for y := range rows {
		for x := range cols {
			dx, dy := float64(x)+0.5-cx, float64(y)+0.5-cy
			r := math.Hypot(dx, dy)
			a := math.Atan2(dy, dx) + rot
			a = math.Mod(a, wedge)
			if a < 0 {
				a += wedge
			}
			if a > wedge/2 {
				a = wedge - a // mirror alternate halves
			}
			rr := math.Mod(r*scale/zoom, float64(f.H)*0.48)
			sx := int(fcx + rr*math.Cos(a-rot*0.5))
			sy := int(fcy + rr*math.Sin(a-rot*0.5))
			if sx < 0 || sy < 0 || sx >= f.W || sy >= f.H {
				continue
			}
			cr, cg, cb := f.At(sx, sy)
			l := luma(cr, cg, cb)
			lo, hi = math.Min(lo, l), math.Max(hi, l)
			buf[n] = px{x, y, cr, cg, cb}
			n++
		}
	}
	if hi-lo < 24 {
		hi = lo + 24
	}
	if s.hi == 0 {
		s.lo, s.hi = lo, hi
	}
	k := math.Min(1, dt*3)
	s.lo += (lo - s.lo) * k
	s.hi += (hi - s.hi) * k
	gain := 255 / (s.hi - s.lo)
	for _, p := range buf[:n] {
		// stretch levels and push saturation so the pattern reads as stained glass
		r, gg, b := (float64(p.r)-s.lo)*gain, (float64(p.g)-s.lo)*gain, (float64(p.b)-s.lo)*gain
		m := (r + gg + b) / 3
		c := led.RGB{R: u8(m + (r-m)*1.8), G: u8(m + (gg-m)*1.8), B: u8(m + (b-m)*1.8)}
		if c.Luma() >= 18 {
			g.Set(p.x, p.y, c)
		}
	}
}
