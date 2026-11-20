package dashboard

import (
	"image"
	"math"
	"strings"
	"time"

	"github.com/justynroberts/marquee/internal/camera"
)

// CameraSource supplies webcam frames; tests replace it.
var CameraSource = camera.LatestWith

// ASCII is the webcam drawn as text at full resolution: each cell's brightness picks a
// character from a density ramp and the camera supplies its colour.
type ASCII struct {
	cellW, cellH int
	glyphs       [][]uint8 // per ramp entry, a cellW x cellH alpha tile
	out          *image.RGBA
	lo, hi       float64
}

const (
	asciiPx   = 16
	asciiRamp = " .'`-:;~=+*xo%#&@"
)

func NewASCII() *ASCII {
	a := &ASCII{out: image.NewRGBA(image.Rect(0, 0, W, H))}
	f := face(asciiPx)
	if f == nil {
		return a
	}
	a.cellW = int(math.Ceil(f.Length("M")))
	a.cellH = asciiPx + 2
	for _, r := range asciiRamp {
		tile := make([]uint8, a.cellW*a.cellH)
		if r != ' ' {
			m, ox, oy := f.Mask(string(r), 0)
			for j := 0; j < m.Rect.Dy(); j++ {
				for i := 0; i < m.Rect.Dx(); i++ {
					x, y := ox+i, oy+j
					if x >= 0 && x < a.cellW && y >= 0 && y < a.cellH {
						tile[y*a.cellW+x] = m.Pix[j*m.Stride+i]
					}
				}
			}
		}
		a.glyphs = append(a.glyphs, tile)
	}
	return a
}

func (a *ASCII) Frame(t, dt float64, now time.Time) *image.RGBA {
	fill(a.out, bg)
	if a.cellW == 0 {
		return a.out
	}
	f := CameraSource(0)
	if msg := cameraMessage(f, t); msg != "" {
		text(a.out, (W-measure(msg, 28))/2, H/2-18, msg, 28, txt2)
		return a.out
	}
	cols, rows := W/a.cellW, H/a.cellH
	x0, y0 := (W-cols*a.cellW)/2, (H-rows*a.cellH)/2
	// fit the frame's height to the panel and stretch it 1.7x wide; the sides stay dark
	sy := float64(f.H) / float64(rows)
	sx := sy / (float64(a.cellH) / float64(a.cellW)) / 1.7
	ox := float64(f.W)/2 - float64(cols)/2*sx
	type cell struct {
		l       float64
		r, g, b uint8
		ok      bool
	}
	cells := make([]cell, cols*rows)
	lo, hi := 255.0, 0.0
	for cy := range rows {
		fy := int((float64(cy) + 0.5) * sy)
		for cx := range cols {
			fx := int(ox + (float64(cx)+0.5)*sx)
			if fx < 0 || fx >= f.W || fy >= f.H {
				continue
			}
			r, g, b := f.At(fx, fy)
			l := 0.299*float64(r) + 0.587*float64(g) + 0.114*float64(b)
			lo, hi = math.Min(lo, l), math.Max(hi, l)
			cells[cy*cols+cx] = cell{l, r, g, b, true}
		}
	}
	if hi-lo < 24 {
		hi = lo + 24
	}
	if a.hi == 0 {
		a.lo, a.hi = lo, hi
	}
	k := math.Min(1, dt*3)
	a.lo += (lo - a.lo) * k
	a.hi += (hi - a.hi) * k
	n := len(a.glyphs)
	for cy := range rows {
		for cx := range cols {
			c := cells[cy*cols+cx]
			if !c.ok {
				continue
			}
			v := math.Max(0, math.Min(1, (c.l-a.lo)/(a.hi-a.lo)))
			gi := int(v * float64(n-1))
			if gi == 0 {
				continue
			}
			// colour: the camera's, levelled and lifted so dim cells still read
			gain := 255 / (a.hi - a.lo)
			cr := clampU8((float64(c.r)-a.lo)*gain*0.8 + 50)
			cg := clampU8((float64(c.g)-a.lo)*gain*0.8 + 50)
			cb := clampU8((float64(c.b)-a.lo)*gain*0.8 + 50)
			tile := a.glyphs[gi]
			px, py := x0+cx*a.cellW, y0+cy*a.cellH
			for j := 0; j < a.cellH; j++ {
				o := a.out.PixOffset(px, py+j)
				for i := 0; i < a.cellW; i++ {
					m := uint32(tile[j*a.cellW+i])
					if m == 0 {
						continue
					}
					p := a.out.Pix[o+i*4 : o+i*4+3]
					p[0] = blend(m, p[0], cr)
					p[1] = blend(m, p[1], cg)
					p[2] = blend(m, p[2], cb)
				}
			}
		}
	}
	return a.out
}

func clampU8(v float64) uint8 { return uint8(math.Max(0, math.Min(255, v))) }

// cameraMessage is the text to show instead of a picture, or "" when there is a frame.
func cameraMessage(f camera.Frame, t float64) string {
	switch {
	case f.Denied:
		return "CAMERA ACCESS NEEDED: SYSTEM SETTINGS > PRIVACY & SECURITY > CAMERA"
	case f.Err == camera.ErrPending:
		return "ALLOW CAMERA ACCESS IN THE DIALOG"
	case f.Err != nil:
		return fit(strings.ToUpper("CAMERA: "+f.Err.Error()), 28, W-2*pad)
	case f.W == 0:
		return "CAMERA STARTING…"
	}
	return ""
}
