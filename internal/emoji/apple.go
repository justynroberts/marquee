package emoji

import (
	"bytes"
	"encoding/binary"
	"errors"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"sync"

	"github.com/justynroberts/marquee/internal/ft"
	"github.com/justynroberts/marquee/internal/pil"
)

// Apple Color Emoji, rendered exactly as board.py's emoji_cells does with Pillow: the 160 px sbix
// strike, shaped with HarfBuzz (ZWJ sequences, skin tones, flags), drawn with embedded colour onto
// a transparent canvas, cropped to its ink, Lanczos-resized, then brightened. The font is read
// from the Mac it runs on; nothing of Apple's is bundled.

const appleEmojiPath = "/System/Library/Fonts/Apple Color Emoji.ttc"

type sbixFont struct {
	face   *ft.Face
	file   *os.File // read on demand: the font is ~180 MB
	strike int      // byte offset of the 160 ppem strike within the file
}

func (f *sbixFont) read(off, n int) []byte {
	b := make([]byte, n)
	if _, err := f.file.ReadAt(b, int64(off)); err != nil {
		return nil
	}
	return b
}

var (
	appleOnce sync.Once
	apple     *sbixFont
)

func loadApple() *sbixFont {
	appleOnce.Do(func() {
		file, err := os.Open(appleEmojiPath)
		if err != nil {
			return
		}
		face, err := ft.Open(appleEmojiPath, 0, 160)
		if err != nil {
			file.Close()
			return
		}
		f := &sbixFont{face: face, file: file}
		strike, err := f.findStrike(160)
		if err != nil {
			file.Close()
			return
		}
		f.strike = strike
		apple = f
	})
	return apple
}

func be16(b []byte) int  { return int(binary.BigEndian.Uint16(b)) }
func be32(b []byte) int  { return int(binary.BigEndian.Uint32(b)) }
func bes16(b []byte) int { return int(int16(binary.BigEndian.Uint16(b))) }

// tableOffset finds a table in font 0 of a TTC (or a plain sfnt).
func (f *sbixFont) tableOffset(tag string) (int, error) {
	head := f.read(0, 16)
	if head == nil {
		return 0, errors.New("unreadable font")
	}
	off := 0
	if string(head[:4]) == "ttcf" {
		off = be32(head[12:])
	}
	dir := f.read(off, 12)
	if dir == nil {
		return 0, errors.New("unreadable font")
	}
	n := be16(dir[4:])
	recs := f.read(off+12, 16*n)
	for i := 0; i < n && recs != nil; i++ {
		rec := recs[16*i:]
		if string(rec[:4]) == tag {
			return be32(rec[8:]), nil
		}
	}
	return 0, errors.New("no " + tag + " table")
}

var numGlyphs int

func (f *sbixFont) findStrike(ppem int) (int, error) {
	maxp, err := f.tableOffset("maxp")
	if err != nil {
		return 0, err
	}
	numGlyphs = be16(f.read(maxp+4, 2))
	sbix, err := f.tableOffset("sbix")
	if err != nil {
		return 0, err
	}
	n := be32(f.read(sbix+4, 4))
	offs := f.read(sbix+8, 4*n)
	for i := 0; i < n; i++ {
		s := sbix + be32(offs[4*i:])
		if be16(f.read(s, 2)) == ppem {
			return s, nil
		}
	}
	return 0, errors.New("no 160 ppem strike")
}

// sbixGlyph returns FreeType's premultiplied BGRA-equivalent (as straight NRGBA decoded, then
// premultiplied the way FreeType's PNG shim does), plus the bitmap origin.
func (f *sbixFont) glyph(id uint32) (img *image.NRGBA, originX, originY int, ok bool) {
	if int(id) >= numGlyphs {
		return nil, 0, 0, false
	}
	offs := f.read(f.strike+4+4*int(id), 8)
	if offs == nil {
		return nil, 0, 0, false
	}
	start, end := f.strike+be32(offs), f.strike+be32(offs[4:])
	if end-start < 8 || end-start > 4<<20 {
		return nil, 0, 0, false
	}
	rec := f.read(start, end-start)
	if rec == nil {
		return nil, 0, 0, false
	}
	if string(rec[4:8]) != "png " {
		return nil, 0, 0, false
	}
	src, err := png.Decode(bytes.NewReader(rec[8:]))
	if err != nil {
		return nil, 0, 0, false
	}
	b := src.Bounds()
	n := image.NewNRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	for y := 0; y < b.Dy(); y++ {
		for x := 0; x < b.Dx(); x++ {
			c := color.NRGBAModel.Convert(src.At(b.Min.X+x, b.Min.Y+y)).(color.NRGBA)
			n.SetNRGBA(x, y, c)
		}
	}
	return n, bes16(rec[0:]), bes16(rec[2:]), true
}

// ftPremul is FreeType's pngshim multiply_alpha.
func ftPremul(a, c uint32) uint32 {
	t := a*c + 0x80
	return (t + (t >> 8)) >> 8
}

func div255(a uint32) uint32 {
	t := a + 128
	return ((t >> 8) + t) >> 8
}

// renderApple is emoji_cells for one cluster at h LED rows.
func renderApple(cluster string, h int) *Sprite {
	f := loadApple()
	if f == nil {
		return nil
	}
	glyphs := f.face.Shape(cluster)
	if len(glyphs) == 0 {
		return nil
	}
	const cw, ch = 360, 240
	canvas := make([]uint8, cw*ch*4) // the RGBA image Pillow draws onto
	// Pillow's font_render colour mask, then ImageDraw's paste through its alpha band.
	pen := 0
	drew := false
	asc := 0 // anchor offsets cancel out after the crop; only relative placement matters
	for _, g := range glyphs {
		img, ox, oy, ok := f.glyph(g.ID)
		px := ft.Pixel(pen + g.XOff)
		pen += g.XAdvance
		if !ok {
			continue
		}
		w, hh := img.Bounds().Dx(), img.Bounds().Dy()
		left, top := ox, oy+hh // FreeType sbix metrics: bearingX, bearingY
		x0, y0 := 10+px+left, 10+asc+160-top
		for y := 0; y < hh; y++ {
			for x := 0; x < w; x++ {
				X, Y := x0+x, y0+y
				if X < 0 || X >= cw || Y < 0 || Y >= ch {
					continue
				}
				c := img.NRGBAAt(x, y)
				a := uint32(c.A)
				if a == 0 {
					continue
				}
				// FreeType premultiplies; Pillow's font_render unpremultiplies.
				r := min(255, 255*ftPremul(a, uint32(c.R))/a)
				gg := min(255, 255*ftPremul(a, uint32(c.G))/a)
				bb := min(255, 255*ftPremul(a, uint32(c.B))/a)
				// ImageDraw pastes the colour (alpha forced to 255) through mask = a onto the
				// transparent canvas: BLEND(mask, 0, in) per band.
				o := (Y*cw + X) * 4
				if canvas[o+3] > 0 {
					m := a
					canvas[o] = uint8(div255(uint32(canvas[o])*(255-m) + r*m))
					canvas[o+1] = uint8(div255(uint32(canvas[o+1])*(255-m) + gg*m))
					canvas[o+2] = uint8(div255(uint32(canvas[o+2])*(255-m) + bb*m))
					canvas[o+3] = uint8(div255(uint32(canvas[o+3])*(255-m) + 255*m))
				} else {
					canvas[o] = uint8(div255(r * a))
					canvas[o+1] = uint8(div255(gg * a))
					canvas[o+2] = uint8(div255(bb * a))
					canvas[o+3] = uint8(div255(255 * a))
				}
				drew = true
			}
		}
	}
	if !drew {
		return nil
	}
	// img.getbbox(): bounds of non-zero alpha.
	minX, minY, maxX, maxY := cw, ch, -1, -1
	for y := 0; y < ch; y++ {
		for x := 0; x < cw; x++ {
			if canvas[(y*cw+x)*4+3] != 0 {
				minX, minY = min(minX, x), min(minY, y)
				maxX, maxY = max(maxX, x), max(maxY, y)
			}
		}
	}
	if maxX < 0 {
		return nil
	}
	bw, bh := maxX-minX+1, maxY-minY+1
	crop := pil.Image{W: bw, H: bh, Pix: make([]uint8, bw*bh*4)}
	for y := 0; y < bh; y++ {
		copy(crop.Pix[y*bw*4:(y+1)*bw*4], canvas[((minY+y)*cw+minX)*4:((minY+y)*cw+maxX+1)*4])
	}
	h = max(8, min(h, 66))
	w := max(1, int(math.RoundToEven(float64(bw)*float64(h)/float64(bh))))
	small := pil.ResizeRGBA(crop, w, h)
	s := &Sprite{W: w, H: h, Lit: make([]bool, w*h), Col: make([]RGBA, w*h)}
	for i := 0; i < w*h; i++ {
		p := small.Pix[i*4:]
		alpha := float32(p[3]) / 255
		den := alpha
		if den < float32(1e-3) {
			den = float32(1e-3)
		}
		ch := func(v uint8) uint8 {
			x := float32(v) / den * float32(1.15)
			if x < 0 {
				return 0
			}
			if x > 255 {
				return 255
			}
			return uint8(x)
		}
		s.Lit[i] = alpha > 0.45
		s.Col[i] = RGBA{ch(p[0]), ch(p[1]), ch(p[2]), p[3]}
	}
	return s
}
