package dashboard

import (
	"image"
	"image/color"
	"math"
	"math/rand"
	"strings"
	"time"
)

// Four more full-resolution clocks: a split-flap board, nixie tubes, a tuning dial and a swarm.

// ================================================================ flip clock

// FlipClock is a split-flap board: hours and minutes on big tiles, seconds on small ones, each
// flap falling over its hinge when the digit changes.
type FlipClock struct {
	out   *image.RGBA
	big   [10]*image.RGBA
	small [10]*image.RGBA
	flaps [6]flap
	date  string
	head  *image.RGBA // the date, rendered when it changes
}

type flap struct {
	from, to int
	at       float64 // when the flap began to fall; <0 when still
}

const (
	fcBigW, fcBigH     = 290, 384
	fcSmallW, fcSmallH = 150, 200
	fcFall             = 0.55 // seconds for a flap to fall
)

var fcBigX = [4]int{44, 350, 724, 1030}

func NewFlipClock() *FlipClock {
	c := &FlipClock{out: image.NewRGBA(image.Rect(0, 0, W, H))}
	for i := range c.flaps {
		c.flaps[i] = flap{from: -1, to: -1, at: -1}
	}
	return c
}

// tile draws one card with its digit: a rounded card, a slightly lighter top half, the hinge.
func fcTile(d, w, h int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	r := float64(w) / 18
	top, bot := color.RGBA{44, 45, 41, 255}, color.RGBA{34, 35, 32, 255}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			a := roundRectCover(float64(x)+0.5, float64(y)+0.5, float64(w), float64(h), r)
			if a <= 0 {
				img.SetRGBA(x, y, bg)
				continue
			}
			c := top
			if y >= h/2 {
				c = bot
			}
			// a soft light from above on each half
			k := float64(y%(h/2)) / float64(h/2)
			c = mix(c, bg, k*0.25)
			img.SetRGBA(x, y, mix(bg, c, a))
		}
	}
	s := string(rune('0' + d))
	px := fitPx("0", 400, w*78/100, h*74/100)
	b := inkBox(s, px)
	bw := inkBox("0", px)
	x := float64(w-bw.Dx())/2 - float64(bw.Min.X) + float64(bw.Dx()-b.Dx())/2 + float64(bw.Min.X-b.Min.X)
	y := (h-bw.Dy())/2 - bw.Min.Y
	text(img, x, y, s, px, txt)
	// the hinge: a dark split with a pin at each side
	m := h / 2
	rect(img, 0, m-2, w-1, m+1, color.RGBA{6, 6, 5, 255})
	rect(img, 0, m-7, 5, m+6, color.RGBA{6, 6, 5, 255})
	rect(img, w-6, m-7, w-1, m+6, color.RGBA{6, 6, 5, 255})
	return img
}

// roundRectCover is how much of the pixel at (x, y) lies inside a w x h rectangle with corners of
// radius r, antialiased over one pixel.
func roundRectCover(x, y, w, h, r float64) float64 {
	qx := math.Abs(x-w/2) - (w/2 - r)
	qy := math.Abs(y-h/2) - (h/2 - r)
	d := math.Hypot(math.Max(qx, 0), math.Max(qy, 0)) + math.Min(math.Max(qx, qy), 0) - r
	return clamp01(0.5 - d)
}

func (c *FlipClock) Frame(t, dt float64, now time.Time) *image.RGBA {
	if c.big[0] == nil {
		for d := 0; d < 10; d++ {
			c.big[d] = fcTile(d, fcBigW, fcBigH)
			c.small[d] = fcTile(d, fcSmallW, fcSmallH)
		}
	}
	digits := now.Format("150405")
	for i := range c.flaps {
		d := int(digits[i] - '0')
		f := &c.flaps[i]
		switch {
		case f.to < 0:
			f.from, f.to = d, d
		case d != f.to && (f.at < 0 || t-f.at >= fcFall):
			f.from, f.to, f.at = f.to, d, t
		}
		if f.at >= 0 && t-f.at >= fcFall {
			f.from, f.at = f.to, -1
		}
	}
	fill(c.out, bg)
	for i := 0; i < 4; i++ {
		c.drawFlap(c.big[:], &c.flaps[i], fcBigX[i], (H-fcBigH)/2, t)
	}
	// the colon between hours and minutes
	cx := (fcBigX[1] + fcBigW + fcBigX[2]) / 2
	for _, y := range []int{H/2 - 60, H/2 + 44} {
		rect(c.out, cx-8, y, cx+8, y+16, txt3)
	}
	// right: the date over the seconds
	x0 := 1390
	if d := now.Format("Mon 2 Jan"); d != c.date || c.head == nil {
		c.date = d
		c.head = image.NewRGBA(image.Rect(0, 0, W-x0, 200))
		fill(c.head, bg)
		text(c.head, 0, 10, strings.ToUpper(now.Format("Monday")), 40, cyan)
		text(c.head, 0, 62, strings.ToUpper(now.Format("2 January")), 64, txt)
	}
	copyInto(c.out, c.head, x0, (H-fcBigH)/2-6)
	for i := 4; i < 6; i++ {
		c.drawFlap(c.small[:], &c.flaps[i], x0+(i-4)*(fcSmallW+12), (H+fcBigH)/2-fcSmallH, t)
	}
	text(c.out, float64(x0+2*(fcSmallW+12)+8), (H+fcBigH)/2-36, "SEC", 26, txt3)
	return c.out
}

// drawFlap draws a tile at (x0, y0). While a flap falls, the old top half folds down over the
// hinge, then the new bottom half unfolds below it; each darkens as it turns away from the light.
func (c *FlipClock) drawFlap(tiles []*image.RGBA, f *flap, x0, y0 int, t float64) {
	cur := tiles[f.to]
	if f.at < 0 {
		copyInto(c.out, cur, x0, y0)
		return
	}
	old := tiles[f.from]
	w, h := cur.Rect.Dx(), cur.Rect.Dy()
	m := h / 2
	p := clamp01((t - f.at) / fcFall)
	p = p * p * (1.6 - 0.6*p)             // falls under gravity, settles at the end
	copyRows(c.out, cur, x0, y0, 0, m, 1) // new top behind
	copyRows(c.out, old, x0, y0, m, h, 1) // old bottom until covered
	if p < 0.5 {
		s := math.Cos(p * math.Pi) // 1 → 0: the old top folds towards the hinge
		top := m - int(float64(m)*s)
		for y := top; y < m; y++ {
			src := m - int(float64(m-y)/math.Max(s, 1e-3))
			copyRow(c.out, old, x0, y0+y, src, 0.55+0.45*s)
		}
	} else {
		s := -math.Cos(p * math.Pi) // 0 → 1: the new bottom unfolds
		bot := m + int(float64(h-m)*s)
		for y := m; y < bot; y++ {
			src := m + int(float64(y-m)/math.Max(s, 1e-3))
			copyRow(c.out, cur, x0, y0+y, src, 0.55+0.45*s)
		}
	}
	_ = w
}

func copyInto(dst, src *image.RGBA, x0, y0 int) {
	copyRows(dst, src, x0, y0, 0, src.Rect.Dy(), 1)
}

func copyRows(dst, src *image.RGBA, x0, y0, from, to int, k float64) {
	for y := from; y < to; y++ {
		copyRow(dst, src, x0, y0+y, y, k)
	}
}

// copyRow copies source row sy to destination row dy at x0, scaled in brightness by k.
func copyRow(dst, src *image.RGBA, x0, dy, sy int, k float64) {
	if dy < 0 || dy >= dst.Rect.Dy() || sy < 0 || sy >= src.Rect.Dy() {
		return
	}
	w := src.Rect.Dx()
	if x0+w > dst.Rect.Dx() {
		w = dst.Rect.Dx() - x0
	}
	d := dst.Pix[dy*dst.Stride+x0*4 : dy*dst.Stride+(x0+w)*4]
	s := src.Pix[sy*src.Stride : sy*src.Stride+w*4]
	if k >= 1 {
		copy(d, s)
		return
	}
	for i := 0; i < len(d); i += 4 {
		d[i] = uint8(float64(s[i]) * k)
		d[i+1] = uint8(float64(s[i+1]) * k)
		d[i+2] = uint8(float64(s[i+2]) * k)
		d[i+3] = 255
	}
}

// ================================================================ nixie clock

// NixieClock is six nixie tubes: the lit numeral glowing orange in front of the dark stack of
// the other nine, a mesh over the front, and neon dots between the pairs blinking the seconds.
type NixieClock struct {
	out   *image.RGBA
	glass *image.RGBA // the empty tubes, drawn once
	tubes [10]*nxGlow // each numeral's glow, drawn once
}

// nxGlow is one numeral's light over a field nxMg wider than the tube on every side, so the halo
// can spill out past the glass: the hot thread, a tight glow round it and a wide halo.
type nxGlow struct {
	hot, core, glow, halo []float32
}

const nxMg = 70

const (
	nxW, nxH        = 236, 420 // a tube
	nxDigW          = 132      // the numeral box
	nxDigH          = 236
	nxDigY          = 70
	nxInner, nxPair = 14, 76
)

var (
	nxOrange = color.RGBA{255, 122, 26, 255}
	nxCore   = color.RGBA{255, 226, 178, 255}
	nxGhost  = color.RGBA{58, 46, 38, 255}
)

func NewNixieClock() *NixieClock {
	return &NixieClock{out: image.NewRGBA(image.Rect(0, 0, W, H))}
}

func nxTubeX(i int) int {
	total := 6*nxW + 3*nxInner + 2*nxPair
	x := (W - total) / 2
	return x + i*nxW + (i/2)*nxPair + (i-i/2)*nxInner
}

// nxStrokes are the numerals as pen strokes in a unit box, drawn the way nixie cathodes are bent.
func nxStrokes(d int) [][][2]float64 {
	arc := func(cx, cy, rx, ry, a0, a1 float64) [][2]float64 {
		n := int(math.Abs(a1-a0)*24) + 2
		pts := make([][2]float64, n)
		for i := range pts {
			a := a0 + (a1-a0)*float64(i)/float64(n-1)
			pts[i] = [2]float64{cx + rx*math.Cos(a), cy + ry*math.Sin(a)}
		}
		return pts
	}
	bez := func(p0, p1, p2 [2]float64) [][2]float64 {
		pts := make([][2]float64, 24)
		for i := range pts {
			t := float64(i) / 23
			u := 1 - t
			pts[i] = [2]float64{u*u*p0[0] + 2*u*t*p1[0] + t*t*p2[0], u*u*p0[1] + 2*u*t*p1[1] + t*t*p2[1]}
		}
		return pts
	}
	ln := func(p ...[2]float64) [][2]float64 { return p }
	switch d {
	case 0:
		return [][][2]float64{arc(0.5, 0.5, 0.5, 0.5, 0, 2*math.Pi)}
	case 1:
		return [][][2]float64{ln([2]float64{0.5, 0}, [2]float64{0.5, 1}), ln([2]float64{0.5, 0}, [2]float64{0.26, 0.17})}
	case 2:
		a := arc(0.5, 0.27, 0.46, 0.27, math.Pi, 2*math.Pi+0.75)
		e := a[len(a)-1]
		return [][][2]float64{a, ln(e, [2]float64{0.02, 1}, [2]float64{1, 1})}
	case 3:
		return [][][2]float64{arc(0.5, 0.25, 0.42, 0.25, math.Pi*1.12, math.Pi*2.5), arc(0.5, 0.73, 0.48, 0.27, -math.Pi/2, math.Pi*0.88)}
	case 4:
		return [][][2]float64{ln([2]float64{0.74, 1}, [2]float64{0.74, 0}, [2]float64{0, 0.68}, [2]float64{1, 0.68})}
	case 5:
		return [][][2]float64{ln([2]float64{0.92, 0}, [2]float64{0.14, 0}, [2]float64{0.07, 0.44}), arc(0.5, 0.66, 0.47, 0.34, -2.45, 2.55)}
	case 6:
		return [][][2]float64{arc(0.5, 0.68, 0.47, 0.32, 0, 2*math.Pi), bez([2]float64{0.84, 0}, [2]float64{0.08, 0.1}, [2]float64{0.04, 0.68})}
	case 7:
		return [][][2]float64{ln([2]float64{0, 0}, [2]float64{1, 0}, [2]float64{0.34, 1})}
	case 8:
		return [][][2]float64{arc(0.5, 0.235, 0.39, 0.235, 0, 2*math.Pi), arc(0.5, 0.72, 0.48, 0.28, 0, 2*math.Pi)}
	case 9:
		var out [][][2]float64
		for _, s := range nxStrokes(6) {
			r := make([][2]float64, len(s))
			for i, p := range s {
				r[i] = [2]float64{1 - p[0], 1 - p[1]}
			}
			out = append(out, r)
		}
		return out
	}
	return nil
}

// stamp draws strokes into a w x h field as discs of radius r along each segment.
func stamp(field []float32, w, h int, strokes [][][2]float64, ox, oy, sx, sy, r float64, v float32) {
	for _, s := range strokes {
		for i := 1; i < len(s); i++ {
			x0, y0 := ox+s[i-1][0]*sx, oy+s[i-1][1]*sy
			x1, y1 := ox+s[i][0]*sx, oy+s[i][1]*sy
			n := int(math.Hypot(x1-x0, y1-y0)) + 1
			for k := 0; k <= n; k++ {
				t := float64(k) / float64(n)
				cx, cy := x0+(x1-x0)*t, y0+(y1-y0)*t
				for yy := int(cy - r - 1); yy <= int(cy+r+1); yy++ {
					for xx := int(cx - r - 1); xx <= int(cx+r+1); xx++ {
						if xx < 0 || yy < 0 || xx >= w || yy >= h {
							continue
						}
						a := float32(clamp01(r + 0.5 - math.Hypot(float64(xx)+0.5-cx, float64(yy)+0.5-cy)))
						if a*v > field[yy*w+xx] {
							field[yy*w+xx] = a * v
						}
					}
				}
			}
		}
	}
}

// boxBlur blurs a field in place, three passes of radius r each way (close to a Gaussian).
func boxBlur(f []float32, w, h, r int) {
	tmp := make([]float32, len(f))
	for pass := 0; pass < 3; pass++ {
		for y := 0; y < h; y++ {
			var s float32
			row := f[y*w : y*w+w]
			for x := -r; x <= r; x++ {
				s += row[min(max(x, 0), w-1)]
			}
			for x := 0; x < w; x++ {
				tmp[y*w+x] = s / float32(2*r+1)
				s += row[min(x+r+1, w-1)] - row[max(x-r, 0)]
			}
		}
		for x := 0; x < w; x++ {
			var s float32
			for y := -r; y <= r; y++ {
				s += tmp[min(max(y, 0), h-1)*w+x]
			}
			for y := 0; y < h; y++ {
				f[y*w+x] = s / float32(2*r+1)
				s += tmp[min(y+r+1, h-1)*w+x] - tmp[max(y-r, 0)*w+x]
			}
		}
	}
}

func (c *NixieClock) build() {
	c.glass = image.NewRGBA(image.Rect(0, 0, W, H))
	fill(c.glass, bg)
	ghost := make([]float32, nxW*nxH)
	for d := 0; d < 10; d++ {
		stamp(ghost, nxW, nxH, nxStrokes(d), (nxW-nxDigW)/2, nxDigY, nxDigW, nxDigH, 1.3, 1)
	}
	for i := 0; i < 6; i++ {
		x0 := nxTubeX(i)
		for y := 0; y < nxH; y++ {
			for x := 0; x < nxW; x++ {
				// the envelope: a tall capsule with a domed top, warm-tinted glass
				a := roundRectCover(float64(x)+0.5, float64(y)+0.5, nxW, nxH-58, nxW/2-6)
				edge := a * (1 - roundRectCover(float64(x)+0.5-2, float64(y)+0.5-2, nxW-4, nxH-62, nxW/2-8))
				col := mix(bg, color.RGBA{24, 21, 18, 255}, a)
				col = mix(col, color.RGBA{96, 90, 80, 255}, edge*0.9)
				// a highlight down the left of the glass
				if hx := float64(x) - 30; hx > 0 && hx < 10 && y > 60 && y < nxH-110 {
					col = mix(col, txt, 0.10*a*math.Sin(hx/10*math.Pi))
				}
				// the dark stack of numerals and the mesh
				if g := ghost[y*nxW+x]; g > 0 && a > 0 {
					col = mix(col, nxGhost, float64(g))
				}
				if a > 0.5 && y > 36 && y < nxH-70 {
					if (x+y)%14 == 0 || (x-y+700)%14 == 0 {
						col = mix(col, color.RGBA{58, 54, 48, 255}, 0.55)
					}
				}
				c.glass.SetRGBA(x0+x, y+4, col)
			}
		}
		// the socket and its pins
		rect(c.glass, x0+38, nxH-50, x0+nxW-38, nxH-8, color.RGBA{30, 30, 28, 255})
		rect(c.glass, x0+38, nxH-50, x0+nxW-38, nxH-48, color.RGBA{70, 68, 62, 255})
		for p := 0; p < 7; p++ {
			px := x0 + 58 + p*20
			rect(c.glass, px, nxH-8, px+3, H-1, color.RGBA{88, 84, 76, 255})
		}
	}
	fw, fh := nxW+2*nxMg, nxH+2*nxMg
	for d := 0; d < 10; d++ {
		g := &nxGlow{hot: make([]float32, fw*fh), core: make([]float32, fw*fh), glow: make([]float32, fw*fh), halo: make([]float32, fw*fh)}
		ox, oy := float64(nxMg+(nxW-nxDigW)/2), float64(nxMg+nxDigY)
		stamp(g.hot, fw, fh, nxStrokes(d), ox, oy, nxDigW, nxDigH, 1.6, 1)
		stamp(g.core, fw, fh, nxStrokes(d), ox, oy, nxDigW, nxDigH, 4.2, 1)
		copy(g.glow, g.core)
		boxBlur(g.glow, fw, fh, 7)
		copy(g.halo, g.core)
		boxBlur(g.halo, fw, fh, 30)
		c.tubes[d] = g
	}
}

func (c *NixieClock) Frame(t, dt float64, now time.Time) *image.RGBA {
	if c.glass == nil {
		c.build()
	}
	img := c.out
	copy(img.Pix, c.glass.Pix)
	digits := now.Format("150405")
	fw, fh := nxW+2*nxMg, nxH+2*nxMg
	for i := 0; i < 6; i++ {
		g := c.tubes[digits[i]-'0']
		// neon never burns quite steady: a slow breath and the odd quick shiver, per tube
		ph := float64(i) * 1.7
		k := float32(0.94 + 0.04*math.Sin(t*2.3+ph) + 0.025*math.Sin(t*17.1+ph*3)*math.Max(0, math.Sin(t*0.7+ph)))
		x0, y0 := nxTubeX(i)-nxMg, 4-nxMg
		for y := 0; y < fh; y++ {
			Y := y0 + y
			if Y < 0 || Y >= H {
				continue
			}
			for x := 0; x < fw; x++ {
				j := y*fw + x
				halo, glow := g.halo[j]*k, g.glow[j]*k
				if halo < 0.002 && glow < 0.002 {
					continue
				}
				X := x0 + x
				if X < 0 || X >= W {
					continue
				}
				o := img.PixOffset(X, Y)
				r := float32(img.Pix[o]) + halo*8.5*nxDeep[0] + glow*3.6*nxOrangeF[0]
				gg := float32(img.Pix[o+1]) + halo*8.5*nxDeep[1] + glow*3.6*nxOrangeF[1]
				b := float32(img.Pix[o+2]) + halo*8.5*nxDeep[2] + glow*3.6*nxOrangeF[2]
				if cv := g.core[j] * k; cv > 0 {
					r, gg, b = r+(255-r)*cv, gg+(196-gg)*cv*0.95+20*cv, b+(120-b)*cv
				}
				if hv := g.hot[j] * k; hv > 0 {
					r, gg, b = r+(255-r)*hv, gg+(246-gg)*hv, b+(222-b)*hv
				}
				img.Pix[o], img.Pix[o+1], img.Pix[o+2] = sat(r), sat(gg), sat(b)
			}
		}
	}
	// neon dots between the pairs, lit for the first half of each second
	if now.Nanosecond() < 5e8 {
		for _, gap := range []int{1, 3} {
			x := nxTubeX(gap) + nxW + nxPair/2
			for _, y := range []int{nxDigY + nxDigH*30/100, nxDigY + nxDigH*75/100} {
				neonDot(img, x, y+4)
			}
		}
	}
	return img
}

var (
	nxOrangeF = [3]float32{255, 118, 22}
	nxDeep    = [3]float32{200, 58, 8}
)

func sat(v float32) uint8 {
	if v >= 255 {
		return 255
	}
	if v <= 0 {
		return 0
	}
	return uint8(v)
}

func neonDot(img *image.RGBA, cx, cy int) {
	for y := cy - 40; y <= cy+40; y++ {
		for x := cx - 40; x <= cx+40; x++ {
			d := math.Hypot(float64(x-cx), float64(y-cy))
			o := img.PixOffset(x, y)
			p := color.RGBA{img.Pix[o], img.Pix[o+1], img.Pix[o+2], 255}
			p = addRGB(p, nxOrange, 1.1*math.Exp(-d*d/90)+0.25*math.Exp(-d*d/600))
			if d < 4.5 {
				p = mix(p, nxCore, clamp01(4.5-d))
			}
			img.Pix[o], img.Pix[o+1], img.Pix[o+2] = p.R, p.G, p.B
		}
	}
}

// addRGB adds c scaled by k, clipping at white.
func addRGB(p, c color.RGBA, k float64) color.RGBA {
	add := func(a, b uint8) uint8 { return uint8(math.Min(255, float64(a)+float64(b)*k)) }
	return color.RGBA{add(p.R, c.R), add(p.G, c.G), add(p.B, c.B), 255}
}

// ================================================================ dial clock

// DialClock is the day as an old radio's tuning dial: a backlit scale of hours slides past a
// fixed red needle, with a fine-tuning scale of seconds beneath it running twenty times faster.
type DialClock struct {
	base *image.RGBA
	out  *image.RGBA
}

const (
	dcHourPx  = 640.0  // pixels per hour on the main scale
	dcMinPx   = 1100.0 // pixels per minute on the fine scale
	dcMainY   = 236    // the main scale's baseline
	dcFineY   = 372    // the fine scale's baseline
	dcNeedleX = W / 2
)

var dcLit = color.RGBA{255, 204, 128, 255}

func NewDialClock() *DialClock {
	return &DialClock{out: image.NewRGBA(image.Rect(0, 0, W, H))}
}

func (c *DialClock) build() {
	c.base = image.NewRGBA(image.Rect(0, 0, W, H))
	for y := 0; y < H; y++ {
		for x := 0; x < W; x++ {
			// glass lit from behind at the centre, falling away to the sides
			dx := float64(x-dcNeedleX) / 760
			dy := float64(y-H/2) / 300
			k := math.Exp(-dx*dx) * math.Exp(-dy*dy*1.5) * 0.17
			c.base.SetRGBA(x, y, mix(bg, color.RGBA{120, 76, 30, 255}, k))
		}
	}
	// the bezel's rails and the divider between the scales
	for _, y := range []int{18, H - 20} {
		rect(c.base, 0, y, W-1, y+2, color.RGBA{86, 82, 72, 255})
	}
	rect(c.base, 0, 276, W-1, 277, color.RGBA{60, 56, 50, 255})
}

// dcLight is how brightly the backlight catches something at x: full at the needle.
func dcLight(x float64) float64 {
	d := (x - dcNeedleX) / 900
	return 0.28 + 0.72*math.Exp(-d*d*2.2)
}

func (c *DialClock) Frame(t, dt float64, now time.Time) *image.RGBA {
	if c.base == nil {
		c.build()
	}
	copy(c.out.Pix, c.base.Pix)
	mid := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	sec := now.Sub(mid).Seconds()

	// the times of day as coloured bands along the top of the main scale
	bands := []struct {
		from, to float64
		name     string
		c        color.RGBA
	}{{-2, 6, "NIGHT", violet}, {6, 12, "MORNING", green}, {12, 18, "AFTERNOON", amber}, {18, 22, "EVENING", hot}, {22, 30, "NIGHT", violet}}
	for _, b := range bands {
		x0 := float64(dcNeedleX) + (b.from*3600-sec)*dcHourPx/3600
		x1 := float64(dcNeedleX) + (b.to*3600-sec)*dcHourPx/3600
		if x1 < 0 || x0 > W {
			continue
		}
		lx0, lx1 := int(math.Max(0, x0)), int(math.Min(W-1, x1))
		for x := lx0; x <= lx1; x++ {
			rect(c.out, x, 40, x, 44, mix(bg, b.c, 0.85*dcLight(float64(x))))
		}
		// the name sits at the band's left edge, or at the screen's when the band began off it
		tx := math.Max(x0+12, 16)
		if nx := float64(dcNeedleX); tx < nx+14 && tx+measure(b.name, 22) > nx-14 {
			tx = nx + 18 // keep clear of the needle
		}
		if tx+measure(b.name, 22) < x1-8 {
			text(c.out, tx, 52, b.name, 22, mix(bg, b.c, dcLight(tx+40)))
		}
	}

	// main scale: a tick a minute, longer every five and fifteen, hours numbered
	first := math.Floor((sec-float64(W)/2*3600/dcHourPx)/60) * 60
	for m := first; ; m += 60 {
		x := float64(dcNeedleX) + (m-sec)*dcHourPx/3600
		if x > W+40 {
			break
		}
		mm := int(math.Mod(m/60, 1440)+1440) % 1440
		l := 14
		switch {
		case mm%60 == 0:
			l = 72
		case mm%15 == 0:
			l = 44
		case mm%5 == 0:
			l = 28
		}
		w := 2
		if l >= 44 {
			w = 3
		}
		col := mix(bg, dcLit, dcLight(x))
		ix := int(math.Round(x))
		rect(c.out, ix-w/2, dcMainY-l, ix-w/2+w-1, dcMainY, col)
		switch {
		case mm%60 == 0:
			s := fmtTwo(mm / 60)
			text(c.out, x-measure(s, 76)/2, dcMainY-176, s, 76, col)
		case mm%15 == 0:
			s := ":" + fmtTwo(mm%60)
			text(c.out, x-measure(s, 26)/2, dcMainY-84, s, 26, mix(bg, txt3, dcLight(x)))
		}
	}
	rect(c.out, 0, dcMainY, W-1, dcMainY+1, mix(bg, dcLit, 0.5))

	// fine scale: seconds, numbered every five
	s0 := math.Floor(sec - float64(W)/2*60/dcMinPx)
	for s := s0; ; s++ {
		x := float64(dcNeedleX) + (s-sec)*dcMinPx/60
		if x > W+20 {
			break
		}
		ss := int(math.Mod(s, 60)+60) % 60
		l := 12
		if ss%5 == 0 {
			l = 26
		}
		col := mix(bg, txt2, dcLight(x)*0.9)
		ix := int(math.Round(x))
		rect(c.out, ix-1, dcFineY-l, ix, dcFineY, col)
		if ss%5 == 0 {
			lab := fmtTwo(ss)
			if ss == 0 {
				lab = now.Add(time.Duration((s - sec) * 1e9)).Format("15:04")
			}
			text(c.out, x-measure(lab, 26)/2, dcFineY-66, lab, 26, col)
		}
	}
	rect(c.out, 0, dcFineY, W-1, dcFineY, mix(bg, txt3, 0.5))

	// the needle, with a soft red glow
	for dx := -10; dx <= 10; dx++ {
		k := math.Exp(-float64(dx*dx) / 18)
		for y := 26; y < H-26; y++ {
			o := c.out.PixOffset(dcNeedleX+dx, y)
			p := color.RGBA{c.out.Pix[o], c.out.Pix[o+1], c.out.Pix[o+2], 255}
			p = addRGB(p, hot, 0.35*k)
			if dx >= -1 && dx <= 1 {
				p = hot
			}
			c.out.Pix[o], c.out.Pix[o+1], c.out.Pix[o+2] = p.R, p.G, p.B
		}
	}
	// the day, small in the corner
	d := strings.ToUpper(now.Format("Mon 2 Jan"))
	text(c.out, W-pad-measure(d, 22), H-52, d, 22, txt3)
	return c.out
}

func fmtTwo(n int) string { return string([]byte{byte('0' + n/10%10), byte('0' + n%10)}) }

// ================================================================ swarm clock

// SwarmClock draws the time in a few thousand particles. They hang in the shape of the digits,
// breathing; when the minute turns they burst and stream to the new time. Seconds fill a row of
// dots beneath.
type SwarmClock struct {
	out   *image.RGBA
	ps    []swParticle
	shown string
	rng   *rand.Rand
	px    int
	pts   [][2]float64 // the points of the digits now shown
}

type swParticle struct {
	x, y, vx, vy, tx, ty, ph float64
}

const swN = 4200

func NewSwarmClock() *SwarmClock {
	return &SwarmClock{out: image.NewRGBA(image.Rect(0, 0, W, H)), rng: rand.New(rand.NewSource(7))}
}

// targets are points sampled from the ink of s, laid out centred above the seconds row.
func (c *SwarmClock) targets(s string) [][2]float64 {
	if c.px == 0 {
		c.px = fitPx("00:00", 420, W-260, H-100)
	}
	f := faceFor(s, c.px)
	if f == nil {
		return nil
	}
	m, ox, oy := f.Mask(s, 0)
	b := inkBox(s, c.px)
	x0 := float64(W-b.Dx())/2 - float64(b.Min.X)
	y0 := float64(H-60-b.Dy())/2 - float64(b.Min.Y)
	var pts [][2]float64
	for y := 0; y < m.Rect.Dy(); y += 3 {
		for x := 0; x < m.Rect.Dx(); x += 3 {
			if m.Pix[y*m.Stride+x] > 140 {
				pts = append(pts, [2]float64{x0 + float64(ox+x), y0 + float64(oy+y)})
			}
		}
	}
	c.rng.Shuffle(len(pts), func(i, j int) { pts[i], pts[j] = pts[j], pts[i] })
	return pts
}

func (c *SwarmClock) Frame(t, dt float64, now time.Time) *image.RGBA {
	hm := now.Format("15:04")
	if hm != c.shown {
		pts := c.targets(hm)
		if len(pts) > 0 {
			first := c.ps == nil
			if first {
				c.ps = make([]swParticle, swN)
				for i := range c.ps {
					c.ps[i] = swParticle{x: c.rng.Float64() * W, y: c.rng.Float64() * H, ph: c.rng.Float64() * 6.28}
				}
			}
			for i := range c.ps {
				p := &c.ps[i]
				q := pts[i%len(pts)]
				p.tx, p.ty = q[0]+c.rng.Float64()*3, q[1]+c.rng.Float64()*3
				if !first {
					a := c.rng.Float64() * 2 * math.Pi
					v := 260 + c.rng.Float64()*520
					p.vx += math.Cos(a) * v
					p.vy += math.Sin(a) * v
				}
			}
			c.shown, c.pts = hm, pts
		}
	}
	// fade the last frame for trails
	pix := c.out.Pix
	for i := 0; i < len(pix); i += 4 {
		pix[i] = uint8(int(bg.R) + (int(pix[i])-int(bg.R))*45/100)
		pix[i+1] = uint8(int(bg.G) + (int(pix[i+1])-int(bg.G))*45/100)
		pix[i+2] = uint8(int(bg.B) + (int(pix[i+2])-int(bg.B))*45/100)
		pix[i+3] = 255
	}
	dt = math.Min(dt, 0.1)
	// a trickle of particles always on the move: each now and then leaves for another spot in the
	// digits and streams across to it
	if len(c.pts) > 0 {
		for i := range c.ps {
			if c.rng.Float64() < dt*0.07 {
				q := c.pts[c.rng.Intn(len(c.pts))]
				c.ps[i].tx, c.ps[i].ty = q[0]+c.rng.Float64()*3, q[1]+c.rng.Float64()*3
			}
		}
	}
	// once a second a wave sweeps left to right across the digits, lifting them as it passes
	frac := float64(now.Nanosecond()) / 1e9
	wave := -200 + frac*(W+400)
	for step := 0; step < 2; step++ {
		h := dt / 2
		for i := range c.ps {
			p := &c.ps[i]
			// a flowing current: the swarm ripples in place, never still
			fx := 4.5*math.Sin(p.ty*0.031+t*1.9+p.ph) + 2*math.Sin(t*3.1+p.ph*2.3)
			fy := 4.5*math.Cos(p.tx*0.027-t*1.6+p.ph) + 2*math.Cos(t*2.7+p.ph*1.9)
			wd := (p.tx - wave) / 70
			lift := 16 * math.Exp(-wd*wd)
			tx := p.tx + fx
			ty := p.ty + fy - lift
			ax := (tx-p.x)*22 - p.vx*6.5
			ay := (ty-p.y)*22 - p.vy*6.5
			// a swirl while far from home
			d := math.Hypot(tx-p.x, ty-p.y)
			if d > 8 {
				ax += -(ty - p.y) * 3
				ay += (tx - p.x) * 3
			}
			p.vx += ax * h
			p.vy += ay * h
			p.x += p.vx * h
			p.y += p.vy * h
		}
	}
	for i := range c.ps {
		p := &c.ps[i]
		sp := math.Hypot(p.vx, p.vy)
		col := mix(txt, cyan, clamp01(sp/240))
		if sp > 420 {
			col = mix(cyan, violet, clamp01((sp-420)/400))
		}
		if wd := (p.x - wave) / 90; wd > -3 && wd < 3 {
			col = mix(col, amber, 0.55*math.Exp(-wd*wd))
		}
		swDot(c.out, p.x, p.y, col)
	}
	// seconds: sixty dots, lit as they pass
	s := now.Second()
	x0 := float64(W)/2 - 59*20/2
	for i := 0; i < 60; i++ {
		col := line
		if i <= s {
			col = amber
			if i%15 == 0 {
				col = txt
			}
		}
		x := int(x0 + float64(i)*20)
		rect(c.out, x-3, H-40, x+3, H-34, col)
	}
	return c.out
}

// swDot draws a soft 3x3 particle.
func swDot(img *image.RGBA, x, y float64, c color.RGBA) {
	cx, cy := int(x), int(y)
	for j := -1; j <= 1; j++ {
		for i := -1; i <= 1; i++ {
			X, Y := cx+i, cy+j
			if X < 0 || Y < 0 || X >= W || Y >= H {
				continue
			}
			k := 1.0
			if i != 0 && j != 0 {
				k = 0.35
			} else if i != 0 || j != 0 {
				k = 0.7
			}
			o := img.PixOffset(X, Y)
			p := color.RGBA{img.Pix[o], img.Pix[o+1], img.Pix[o+2], 255}
			p = mix(p, c, k)
			img.Pix[o], img.Pix[o+1], img.Pix[o+2] = p.R, p.G, p.B
		}
	}
}
