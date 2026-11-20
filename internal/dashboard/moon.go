package dashboard

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"math/rand/v2"
	"strings"
	"time"
)

// Moon is tonight's moon, worked out on the Mac (no network): the disc lit to its phase with
// faint earthshine on the dark side, its name, illumination and age, when the next full and new
// moons fall, and a strip of the coming 30 nights.
type Moon struct {
	tex   []float64 // surface brightness over the unit disc, moonR*2 square
	cache frameCache
}

const (
	moonR     = 176
	moonCX    = pad + moonR + 20
	moonCY    = H / 2
	synodic   = 29.530588853
	moonRight = moonCX + moonR + 80
)

// a known new moon: 6 January 2000 18:14 UTC
var newMoon2000 = time.Date(2000, 1, 6, 18, 14, 0, 0, time.UTC)

// MoonAge is days since the last new moon.
func MoonAge(t time.Time) float64 {
	a := math.Mod(t.Sub(newMoon2000).Hours()/24, synodic)
	if a < 0 {
		a += synodic
	}
	return a
}

// MoonIllumination is the lit fraction of the disc.
func MoonIllumination(age float64) float64 { return (1 - math.Cos(2*math.Pi*age/synodic)) / 2 }

// MoonPhase names the phase for an age in days.
func MoonPhase(age float64) string {
	names := []string{"New moon", "Waxing crescent", "First quarter", "Waxing gibbous", "Full moon", "Waning gibbous", "Last quarter", "Waning crescent"}
	return names[int(math.Floor(age/synodic*8+0.5))%8]
}

func NewMoon() *Moon {
	m := &Moon{tex: make([]float64, 4*moonR*moonR)}
	// a quiet, fixed surface: dark maria and a scatter of small craters over soft noise
	r := rand.New(rand.NewPCG(7, 11))
	type spot struct{ x, y, r, d float64 }
	spots := []spot{{-0.28, -0.32, 0.30, 0.22}, {0.12, -0.18, 0.26, 0.2}, {-0.42, 0.12, 0.28, 0.18}, {0.3, 0.2, 0.2, 0.16}, {-0.05, 0.38, 0.16, 0.12}}
	for range 60 {
		spots = append(spots, spot{r.Float64()*1.8 - 0.9, r.Float64()*1.8 - 0.9, 0.02 + r.Float64()*0.06, 0.08 + r.Float64()*0.1})
	}
	for j := 0; j < 2*moonR; j++ {
		for i := 0; i < 2*moonR; i++ {
			x, y := (float64(i)+0.5)/moonR-1, (float64(j)+0.5)/moonR-1
			v := 0.92 + 0.05*math.Sin(x*13+y*7)*math.Cos(y*11-x*5)
			for _, s := range spots {
				if d := math.Hypot(x-s.x, y-s.y) / s.r; d < 1 {
					v -= s.d * (1 - d*d)
				}
			}
			m.tex[j*2*moonR+i] = v
		}
	}
	return m
}

func (m *Moon) Frame(t, dt float64, now time.Time) *image.RGBA {
	return m.cache.get(now.Format("2006-01-02 15"), func(img *image.RGBA) { m.render(img, now) })
}

func (m *Moon) render(img *image.RGBA, now time.Time) {
	age := MoonAge(now)
	m.draw(img, moonCX, moonCY, moonR, age, true)

	x := float64(moonRight)
	head(img, moonRight, pad-4, "the moon tonight", 600)
	text(img, x, 76, strings.ToUpper(MoonPhase(age)), 64, txt)
	text(img, x, 160, fmt.Sprintf("%.0f%% LIT   DAY %.1f OF %.1f", MoonIllumination(age)*100, age, synodic), 28, amber)

	nextFull := now.Add(time.Duration(math.Mod(synodic/2-age+synodic, synodic) * 24 * float64(time.Hour)))
	nextNew := now.Add(time.Duration((synodic - age) * 24 * float64(time.Hour)))
	row := func(y int, label string, at time.Time) {
		text(img, x, y, label, 22, cyan)
		days := int(math.Round(at.Sub(now).Hours() / 24))
		when := strings.ToUpper(at.Format("Mon 2 Jan"))
		text(img, x+200, y-4, when, 30, txt)
		text(img, x+520, y, fmt.Sprintf("IN %d DAYS", days), 22, amber)
	}
	row(222, "FULL MOON", nextFull)
	row(266, "NEW MOON", nextNew)

	// the next 30 nights
	sx, sy, step := moonRight, 352, (W-pad-moonRight)/30
	r := float64(step)/2 - 4
	for i := 0; i < 30; i++ {
		d := now.AddDate(0, 0, i)
		cx := sx + i*step + step/2
		m.draw(img, cx, sy, r, MoonAge(d), false)
		if i == 0 || (i > 2 && (d.Day() == 1 || d.Weekday() == time.Monday)) {
			lbl := strings.ToUpper(d.Format("2"))
			if i == 0 {
				lbl = "TODAY"
			}
			c := txt3
			if i == 0 {
				c = txt2
			}
			text(img, float64(cx)-measure(lbl, 16)/2, sy+int(r)+14, lbl, 16, c)
		}
	}
}

// draw renders the moon at age with its centre at cx, cy. Northern-hemisphere view: waxing
// lights the right-hand side.
func (m *Moon) draw(img *image.RGBA, cx, cy int, r float64, age float64, textured bool) {
	cosPhi := math.Cos(2 * math.Pi * age / synodic)
	waxing := age < synodic/2
	lit := txt
	dark := color.RGBA{30, 31, 28, 255}
	ri := int(r) + 1
	for j := -ri; j <= ri; j++ {
		for i := -ri; i <= ri; i++ {
			fx, fy := (float64(i)+0.5)/r, (float64(j)+0.5)/r
			d := math.Hypot(fx, fy)
			cov := (1 - d) * r // pixels inside the rim
			if cov <= -0.5 {
				continue
			}
			w := math.Sqrt(math.Max(0, 1-fy*fy))
			// distance past the terminator, in pixels, for a soft edge
			var e float64
			if waxing {
				e = (fx - w*cosPhi) * r
			} else {
				e = (-w*cosPhi - fx) * r
			}
			k := math.Max(0, math.Min(1, e/3+0.5))
			v := 1.0
			if textured {
				tx, ty := int((fx+1)*moonR), int((fy+1)*moonR)
				if tx >= 0 && ty >= 0 && tx < 2*moonR && ty < 2*moonR {
					v = m.tex[ty*2*moonR+tx]
				}
				v *= 1 - 0.25*d*d*d*d // limb darkening
			}
			c := mix(dark, color.RGBA{uint8(float64(lit.R) * v), uint8(float64(lit.G) * v), uint8(float64(lit.B) * v * 0.96), 255}, k)
			a := uint32(math.Max(0, math.Min(1, cov+0.5)) * 255)
			x, y := cx+i, cy+j
			if x < 0 || y < 0 || x >= W || y >= H {
				continue
			}
			o := img.PixOffset(x, y)
			img.Pix[o] = blend(a, img.Pix[o], c.R)
			img.Pix[o+1] = blend(a, img.Pix[o+1], c.G)
			img.Pix[o+2] = blend(a, img.Pix[o+2], c.B)
		}
	}
}
