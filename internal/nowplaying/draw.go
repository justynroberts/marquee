package nowplaying

// Drawing helpers copied from internal/dashboard (unexported there) so the Now Playing screen
// shares its palette, typeface chain and Pillow-exact text rendering.

import (
	"image"
	"image/color"
	"image/draw"
	"math"
	"strings"

	"github.com/justynroberts/marquee/internal/ft"
)

const (
	W, H = 1920, 462
	pad  = 36
)

var (
	bg   = color.RGBA{12, 13, 11, 255}
	ink  = color.RGBA{230, 226, 214, 255}
	ink2 = color.RGBA{161, 157, 144, 255}
	ink3 = color.RGBA{96, 93, 85, 255}
	sung = color.RGBA{120, 116, 106, 255} // karaoke: the part of the line not yet sung
	rule = color.RGBA{44, 44, 38, 255}
	hot  = color.RGBA{255, 91, 56, 255}
)

// SF Mono, falling back to Menlo and then DejaVu Sans Mono, exactly like the dashboard.
var monoPaths = []struct {
	path  string
	index int
}{{"/System/Library/Fonts/SFNSMono.ttf", 0}, {"/System/Library/Fonts/Menlo.ttc", 0}, {"/usr/share/fonts/truetype/dejavu/DejaVuSansMono.ttf", 0}}

func face(px int) *ft.Face {
	for _, p := range monoPaths {
		if f, err := ft.Open(p.path, p.index, px); err == nil {
			return f
		}
	}
	return nil
}

// text draws s with its "la" anchor at (x, y), clipped to dst's bounds, and returns the advance.
func text(dst *image.RGBA, x float64, y int, s string, px int, c color.RGBA) float64 {
	return textFace(dst, face(px), x, y, s, c)
}

// Bold monospace for lyrics: Menlo Bold, then DejaVu Sans Mono Bold, then the regular face.
var boldPaths = []struct {
	path  string
	index int
}{{"/System/Library/Fonts/Menlo.ttc", 1}, {"/usr/share/fonts/truetype/dejavu/DejaVuSansMono-Bold.ttf", 0}}

func boldFace(px int) *ft.Face {
	for _, p := range boldPaths {
		if f, err := ft.Open(p.path, p.index, px); err == nil {
			return f
		}
	}
	return face(px)
}

func measureBold(s string, px int) float64 {
	if f := boldFace(px); f != nil {
		return f.Length(s)
	}
	return 0
}

func textBold(dst *image.RGBA, x float64, y int, s string, px int, c color.RGBA) float64 {
	return textFace(dst, boldFace(px), x, y, s, c)
}

// textBoldClip draws only the part of the text left of maxX (the karaoke sweep).
func textBoldClip(dst *image.RGBA, x float64, y int, s string, px int, c color.RGBA, maxX float64) {
	r := dst.Bounds()
	r.Max.X = min(r.Max.X, int(math.Ceil(maxX)))
	if r.Max.X <= r.Min.X {
		return
	}
	textFace(dst.SubImage(r).(*image.RGBA), boldFace(px), x, y, s, c)
}

func textFace(dst *image.RGBA, f *ft.Face, x float64, y int, s string, c color.RGBA) float64 {
	if f == nil || s == "" {
		return 0
	}
	ix := int(math.Floor(x))
	m, ox, oy := f.Mask(s, x-float64(ix))
	b := dst.Bounds()
	for j := 0; j < m.Rect.Dy(); j++ {
		for i := 0; i < m.Rect.Dx(); i++ {
			a := uint32(m.Pix[j*m.Stride+i])
			if a == 0 {
				continue
			}
			X, Y := ix+ox+i, y+oy+j
			if X < b.Min.X || X >= b.Max.X || Y < b.Min.Y || Y >= b.Max.Y {
				continue
			}
			o := dst.PixOffset(X, Y)
			dst.Pix[o] = blend(a, dst.Pix[o], c.R)
			dst.Pix[o+1] = blend(a, dst.Pix[o+1], c.G)
			dst.Pix[o+2] = blend(a, dst.Pix[o+2], c.B)
		}
	}
	return f.Length(s)
}

func blend(m uint32, in1, in2 uint8) uint8 {
	t := uint32(in1)*(255-m) + uint32(in2)*m + 128
	return uint8(((t >> 8) + t) >> 8)
}

func measure(s string, px int) float64 {
	if f := face(px); f != nil {
		return f.Length(s)
	}
	return 0
}

func fit(s string, px int, width float64) string {
	if measure(s, px) <= width {
		return s
	}
	r := []rune(s)
	for len(r) > 0 && measure(string(r)+"…", px) > width {
		r = r[:len(r)-1]
	}
	return string(r) + "…"
}

func caps(dst *image.RGBA, x, y int, s string, width int, c color.RGBA) float64 {
	return text(dst, float64(x), y, fit(strings.ToUpper(s), 22, float64(width)), 22, c)
}

// rect fills like Pillow's ImageDraw.rectangle: both corners inclusive.
func rect(dst *image.RGBA, x0, y0, x1, y1 int, c color.Color) {
	draw.Draw(dst, image.Rect(x0, y0, x1+1, y1+1), image.NewUniform(c), image.Point{}, draw.Src)
}

func fill(dst *image.RGBA, c color.Color) {
	draw.Draw(dst, dst.Bounds(), image.NewUniform(c), image.Point{}, draw.Src)
}

func bar(dst *image.RGBA, x0, y0, x1, y1 int, frac float64, c color.Color) {
	rect(dst, x0, y0, x1, y1, rule)
	w := int(float64(x1-x0) * math.Max(0, math.Min(frac, 1)))
	if w > 0 {
		rect(dst, x0, y0, x0+w, y1, c)
	}
}
