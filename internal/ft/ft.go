// Package ft renders text with FreeType 2.14.3 and HarfBuzz 14.5.0 (both vendored, the versions
// Pillow 13 bundles), laid out the way Pillow's ImageDraw.text does (hinted
// glyphs, grayscale anti-aliasing, integer-rounded pen positions, "la" anchor), so text on the
// panel matches the reference implementation pixel for pixel.
package ft

/*
#cgo CFLAGS: -I${SRCDIR}/freetype/include -I${SRCDIR}/harfbuzz/src -O2 -Wno-everything
#cgo CXXFLAGS: -I${SRCDIR}/freetype/include -std=c++17 -O2 -fno-exceptions -fno-rtti -Wno-everything
#include <ft2build.h>
#include FT_FREETYPE_H
#include FT_OUTLINE_H
#include <stdlib.h>
#include <string.h>
#include "hb.h"
#include "hb-ft.h"

static FT_Library lib;

static int mq_init(void) { return FT_Init_FreeType(&lib); }

static int mq_open(const char *path, long index, FT_Face *out) {
	return FT_New_Face(lib, path, index, out);
}

static int mq_size(FT_Face f, int px) { return FT_Set_Pixel_Sizes(f, 0, px); }

typedef struct {
	int left, top, width, rows, pitch;
	long advance; // 26.6
	unsigned char *buffer;
} mq_glyph;

static long mq_ascender(FT_Face f) { return f->size->metrics.ascender; }

// Shape UTF-32 text as Raqm does for Pillow: an hb-ft font with its default load flags
// (unhinted advances), segment properties guessed, default features. Returns the glyph count;
// fills up to cap entries of (index, x_advance, x_offset, y_offset) in 26.6.
static int mq_shape(FT_Face f, const unsigned int *text, int n, int cap,
                    unsigned int *idx, int *xadv, int *xoff, int *yoff) {
	hb_font_t *font = hb_ft_font_create_referenced(f);
	hb_buffer_t *buf = hb_buffer_create();
	hb_buffer_add_utf32(buf, text, n, 0, n);
	hb_buffer_guess_segment_properties(buf);
	hb_shape(font, buf, NULL, 0);
	unsigned int count = 0;
	hb_glyph_info_t *info = hb_buffer_get_glyph_infos(buf, &count);
	hb_glyph_position_t *pos = hb_buffer_get_glyph_positions(buf, NULL);
	int out = (int)count < cap ? (int)count : cap;
	for (int i = 0; i < out; i++) {
		idx[i] = info[i].codepoint; xadv[i] = pos[i].x_advance;
		xoff[i] = pos[i].x_offset; yoff[i] = pos[i].y_offset;
	}
	hb_buffer_destroy(buf);
	hb_font_destroy(font);
	return (int)count;
}

// Load and render one glyph by index (hinted, as Pillow renders); returns the CBox in pixels too.
static int mq_glyph_index(FT_Face f, unsigned int idx, mq_glyph *g, int *cxmin, int *cymin, int *cxmax, int *cymax) {
	int err = FT_Load_Glyph(f, idx, FT_LOAD_DEFAULT);
	if (err) return err;
	FT_BBox b;
	FT_Outline_Get_CBox(&f->glyph->outline, &b);
	if (f->glyph->format == FT_GLYPH_FORMAT_OUTLINE) {
		*cxmin = (int)(b.xMin >> 6) ; *cymin = (int)(b.yMin >> 6);
		*cxmax = (int)((b.xMax + 63) >> 6); *cymax = (int)((b.yMax + 63) >> 6);
	} else {
		*cxmin = *cymin = *cxmax = *cymax = 0;
	}
	err = FT_Render_Glyph(f->glyph, FT_RENDER_MODE_NORMAL);
	if (err) return err;
	FT_GlyphSlot s = f->glyph;
	g->left = s->bitmap_left; g->top = s->bitmap_top;
	g->width = s->bitmap.width; g->rows = s->bitmap.rows; g->pitch = s->bitmap.pitch;
	g->advance = s->advance.x;
	g->buffer = s->bitmap.buffer;
	return 0;
}
*/
import "C"

import (
	"errors"
	"fmt"
	"image"
	"math"
	"sync"
	"unsafe"
)

var (
	initOnce sync.Once
	initErr  error
	mu       sync.Mutex // FreeType faces are not thread-safe
	faces    = map[faceKey]*Face{}
)

type faceKey struct {
	path  string
	index int
	px    int
}

// Face is a font at one pixel size.
type Face struct {
	f  C.FT_Face
	px int
}

// pixel is Pillow's PIXEL(): 26.6 fixed point rounded to the nearest pixel.
func pixel(v int) int { return (v + 32) &^ 63 >> 6 }

// Open loads (and caches) a face at a pixel size, like ImageFont.truetype(path, px, index).
func Open(path string, index, px int) (*Face, error) {
	initOnce.Do(func() {
		if C.mq_init() != 0 {
			initErr = errors.New("freetype init failed")
		}
	})
	if initErr != nil {
		return nil, initErr
	}
	mu.Lock()
	defer mu.Unlock()
	k := faceKey{path, index, px}
	if f, ok := faces[k]; ok {
		return f, nil
	}
	cpath := C.CString(path)
	defer C.free(unsafe.Pointer(cpath))
	var f C.FT_Face
	if e := C.mq_open(cpath, C.long(index), &f); e != 0 {
		return nil, fmt.Errorf("freetype: open %s#%d: error %d", path, index, int(e))
	}
	if e := C.mq_size(f, C.int(px)); e != 0 {
		return nil, fmt.Errorf("freetype: size %d: error %d", px, int(e))
	}
	face := &Face{f: f, px: px}
	faces[k] = face
	return face, nil
}

type placed struct {
	x, y        int // bitmap origin relative to the "la" anchor (y down)
	w, h, pitch int
	buf         []byte
}

type layoutResult struct {
	glyphs                 []placed
	pen                    int // 26.6
	xMin, xMax, yMin, yMax int // Pillow's text box in pixels, y up, includes the pen line
	asc                    int
}

// layout shapes s with HarfBuzz and places hinted glyph bitmaps exactly as Pillow's
// font_render / text_layout_raqm / bounding_box_and_anchors do. Caller holds mu.
func (f *Face) layout(s string) layoutResult { return f.layoutFrom(s, 0) }

// layoutFrom lays out with the pen starting at shift (26.6). Caller holds mu.
func (f *Face) layoutFrom(s string, shift int) layoutResult {
	res := layoutResult{asc: pixel(int(C.mq_ascender(f.f)))}
	runes := []rune(s)
	if len(runes) == 0 {
		return res
	}
	text := make([]C.uint, len(runes))
	for i, r := range runes {
		text[i] = C.uint(r)
	}
	capN := len(runes)*4 + 8
	idx := make([]C.uint, capN)
	xadv := make([]C.int, capN)
	xoff := make([]C.int, capN)
	yoff := make([]C.int, capN)
	n := int(C.mq_shape(f.f, &text[0], C.int(len(runes)), C.int(capN), &idx[0], &xadv[0], &xoff[0], &yoff[0]))
	n = min(n, capN)
	pen := shift
	for i := 0; i < n; i++ {
		px := pixel(pen + int(xoff[i]))
		py := pixel(int(yoff[i]))
		pen += int(xadv[i])
		if adv := pixel(pen); adv > res.xMax {
			res.xMax = adv
		}
		var g C.mq_glyph
		var cx0, cy0, cx1, cy1 C.int
		if C.mq_glyph_index(f.f, idx[i], &g, &cx0, &cy0, &cx1, &cy1) != 0 {
			continue
		}
		res.xMax = max(res.xMax, int(cx1)+px)
		res.xMin = min(res.xMin, int(cx0)+px)
		res.yMax = max(res.yMax, int(cy1)+py)
		res.yMin = min(res.yMin, int(cy0)+py)
		rows, pitch := int(g.rows), int(g.pitch)
		var buf []byte
		if rows*pitch > 0 && g.buffer != nil {
			buf = C.GoBytes(unsafe.Pointer(g.buffer), C.int(rows*pitch))
		}
		res.glyphs = append(res.glyphs, placed{
			x: px + int(g.left), y: res.asc - (py + int(g.top)),
			w: int(g.width), h: rows, pitch: pitch, buf: buf,
		})
	}
	res.pen = pen - shift
	return res
}

// Length is ImageFont.getlength: the shaped advance width in pixels (fractional).
func (f *Face) Length(s string) float64 {
	mu.Lock()
	defer mu.Unlock()
	return float64(f.layout(s).pen) / 64
}

// BBox is ImageFont.getbbox with the default "la" anchor (y down).
func (f *Face) BBox(s string) image.Rectangle {
	mu.Lock()
	defer mu.Unlock()
	r := f.layout(s)
	top := r.asc - r.yMax
	return image.Rect(r.xMin, top, r.xMax, top+(r.yMax-r.yMin))
}

// Draw renders s into an 8-bit alpha image with the "la" anchor at (x, y), combining overlapping
// glyphs exactly as Pillow's font_render does.
func (f *Face) Draw(dst *image.Alpha, x, y int, s string) {
	mu.Lock()
	gl := f.layout(s).glyphs
	mu.Unlock()
	drawGlyphs(dst, x, y, gl)
}

func drawGlyphs(dst *image.Alpha, x, y int, gl []placed) {
	b := dst.Bounds()
	for _, g := range gl {
		for row := 0; row < g.h; row++ {
			yy := y + g.y + row
			if yy < b.Min.Y || yy >= b.Max.Y {
				continue
			}
			for col := 0; col < g.w; col++ {
				xx := x + g.x + col
				if xx < b.Min.X || xx >= b.Max.X {
					continue
				}
				src := uint(g.buf[row*g.pitch+col])
				if src == 0 {
					continue
				}
				o := dst.PixOffset(xx, yy)
				// Pillow: target = src + MULDIV255(target, 255-src), clipped to 255.
				if t := uint(dst.Pix[o]); t > 0 {
					tmp := t*(255-src) + 128
					v := src + ((tmp>>8)+tmp)>>8
					dst.Pix[o] = uint8(min(v, 255))
				} else {
					dst.Pix[o] = uint8(src)
				}
			}
		}
	}
}

// Glyph is one shaped glyph: font glyph id and HarfBuzz positions in 26.6.
type Glyph struct {
	ID                   uint32
	XAdvance, XOff, YOff int
}

// Shape runs HarfBuzz on s (as Raqm does for Pillow) and returns the glyphs.
func (f *Face) Shape(s string) []Glyph {
	runes := []rune(s)
	if len(runes) == 0 {
		return nil
	}
	mu.Lock()
	defer mu.Unlock()
	text := make([]C.uint, len(runes))
	for i, r := range runes {
		text[i] = C.uint(r)
	}
	capN := len(runes)*4 + 8
	idx := make([]C.uint, capN)
	xadv := make([]C.int, capN)
	xoff := make([]C.int, capN)
	yoff := make([]C.int, capN)
	n := min(int(C.mq_shape(f.f, &text[0], C.int(len(runes)), C.int(capN), &idx[0], &xadv[0], &xoff[0], &yoff[0])), capN)
	out := make([]Glyph, n)
	for i := range out {
		out[i] = Glyph{uint32(idx[i]), int(xadv[i]), int(xoff[i]), int(yoff[i])}
	}
	return out
}

// Pixel is Pillow's PIXEL() rounding of a 26.6 value.
func Pixel(v int) int { return pixel(v) }

// Mask renders s as ImageDraw.text((x, y), s) would for a float position whose fractional part
// is start: the coverage mask and its offset from (int(x), int(y)), anchor "la".
func (f *Face) Mask(s string, start float64) (*image.Alpha, int, int) {
	mu.Lock()
	r := f.layout(s)
	mu.Unlock()
	shift := int(math.Round(start * 64))
	if shift != 0 {
		// Re-place with the sub-pixel pen start (glyph bitmaps are pen-independent).
		mu.Lock()
		r = f.layoutFrom(s, shift)
		mu.Unlock()
	}
	if len(r.glyphs) == 0 {
		return image.NewAlpha(image.Rect(0, 0, 1, 1)), 0, 0
	}
	x0, y0, x1, y1 := 1<<30, 1<<30, -1<<30, -1<<30
	for _, g := range r.glyphs {
		x0, y0 = min(x0, g.x), min(y0, g.y)
		x1, y1 = max(x1, g.x+g.w), max(y1, g.y+g.h)
	}
	if x1 <= x0 || y1 <= y0 {
		return image.NewAlpha(image.Rect(0, 0, 1, 1)), 0, 0
	}
	m := image.NewAlpha(image.Rect(0, 0, x1-x0, y1-y0))
	drawGlyphs(m, -x0, -y0, r.glyphs)
	return m, x0, y0
}
