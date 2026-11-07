// Package hud draws the heads-up screen: pure black (which vanishes in a reflection), bold
// white type, amber only for alerts. It is meant to be read mirrored in a monitor's glass, so
// it is deliberately plain: a huge clock and one or two lines beside it.
package hud

import (
	"image"
	"image/color"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/justynroberts/marquee/internal/ft"
)

const W, H = 1920, 462

var (
	black = color.RGBA{0, 0, 0, 255}
	white = color.RGBA{255, 255, 255, 255}
	soft  = color.RGBA{200, 200, 200, 255}
	amber = color.RGBA{255, 176, 0, 255}
)

// Info is what the HUD shows. Priority for the main line: Alert, Message, Track, the day.
type Info struct {
	Now     time.Time
	Alert   string
	Message string
	Track   string // "Title — Artist" when something is playing
	Seconds bool   // show seconds on the clock

	// Lyrics mode: when Lyrics is true, the HUD shows Lyric (sweeping from grey to white by
	// Sweep, 0..1) with NextLyric beneath, instead of the clock. Alerts still take over.
	Lyrics    bool
	Lyric     string
	NextLyric string
	Sweep     float64
}

type fontRef struct {
	path  string
	index int
}

var (
	boldFonts = []fontRef{{"/System/Library/Fonts/Helvetica.ttc", 1}, {"/System/Library/Fonts/HelveticaNeue.ttc", 1}, {"/usr/share/fonts/truetype/dejavu/DejaVuSans-Bold.ttf", 0}, {"/System/Library/Fonts/SFNSMono.ttf", 0}}
	textFonts = []fontRef{{"/System/Library/Fonts/Helvetica.ttc", 0}, {"/System/Library/Fonts/HelveticaNeue.ttc", 0}, {"/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf", 0}, {"/System/Library/Fonts/SFNSMono.ttf", 0}}
	faceMu    sync.Mutex
	faces     = map[[2]int]*ft.Face{}
)

func face(bold bool, px int) *ft.Face {
	faceMu.Lock()
	defer faceMu.Unlock()
	b := 0
	list := textFonts
	if bold {
		b, list = 1, boldFonts
	}
	k := [2]int{b, px}
	if f, ok := faces[k]; ok {
		return f
	}
	for _, r := range list {
		if f, err := ft.Open(r.path, r.index, px); err == nil {
			faces[k] = f
			return f
		}
	}
	faces[k] = nil
	return nil
}

// draw puts s with its top-left ink at (x, y) and returns the advance.
func draw(dst *image.RGBA, x, y int, s string, bold bool, px int, c color.RGBA) int {
	f := face(bold, px)
	if f == nil || s == "" {
		return 0
	}
	m, ox, oy := f.Mask(s, 0)
	bb := f.BBox(s)
	b := dst.Bounds()
	for j := 0; j < m.Rect.Dy(); j++ {
		for i := 0; i < m.Rect.Dx(); i++ {
			a := uint32(m.Pix[j*m.Stride+i])
			if a == 0 {
				continue
			}
			X, Y := x+ox+i, y+oy+j-bb.Min.Y
			if X < b.Min.X || X >= b.Max.X || Y < b.Min.Y || Y >= b.Max.Y {
				continue
			}
			o := dst.PixOffset(X, Y)
			dst.Pix[o] = uint8((uint32(dst.Pix[o])*(255-a) + uint32(c.R)*a) / 255)
			dst.Pix[o+1] = uint8((uint32(dst.Pix[o+1])*(255-a) + uint32(c.G)*a) / 255)
			dst.Pix[o+2] = uint8((uint32(dst.Pix[o+2])*(255-a) + uint32(c.B)*a) / 255)
		}
	}
	return int(math.Ceil(f.Length(s)))
}

func width(s string, bold bool, px int) int {
	f := face(bold, px)
	if f == nil {
		return 0
	}
	return int(math.Ceil(f.Length(s)))
}

// fitLine picks the largest size from sizes at which s fits w, ellipsizing at the smallest.
func fitLine(s string, bold bool, sizes []int, w int) (string, int) {
	for _, px := range sizes {
		if width(s, bold, px) <= w {
			return s, px
		}
	}
	px := sizes[len(sizes)-1]
	r := []rune(s)
	for len(r) > 1 && width(string(r)+"…", bold, px) > w {
		r = r[:len(r)-1]
	}
	return strings.TrimSpace(string(r)) + "…", px
}

const (
	margin  = 48
	clockPx = 300
	gap     = 72
)

// Render draws one frame.
func Render(in Info) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, W, H))
	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i+3] = 255 // black
	}
	if in.Lyrics && in.Alert == "" {
		renderLyrics(img, in)
		return img
	}
	layout := "15:04"
	if in.Seconds {
		layout = "15:04:05"
	}
	clock := in.Now.Format(layout)
	// Centre the clock's cap height vertically; digits are all cap height in Helvetica.
	capH := int(float64(clockPx) * 0.72)
	cw := draw(img, margin, (H-capH)/2, clock, true, clockPx, white)

	x := margin + cw + gap
	w := W - margin - x
	main, mainCol := in.Now.Format("Monday"), white
	sub := in.Now.Format("2 January")
	switch {
	case in.Alert != "":
		main, mainCol = in.Alert, amber
		sub = ""
	case in.Message != "":
		main = in.Message
		if in.Track != "" {
			sub = in.Track
		} else {
			sub = in.Now.Format("Monday 2 January")
		}
	case in.Track != "":
		parts := strings.SplitN(in.Track, " — ", 2)
		main = parts[0]
		sub = ""
		if len(parts) == 2 {
			sub = parts[1]
		}
	}
	if in.Alert != "" {
		// An alert takes the whole screen (no clock), wrapped to at most three lines.
		img = Render(Info{Now: in.Now})
		for i := 0; i < len(img.Pix); i += 4 {
			img.Pix[i], img.Pix[i+1], img.Pix[i+2] = 0, 0, 0
		}
		ww := W - 2*margin
		var lines wrapped
		for _, z := range []int{150, 124, 104, 88, 76, 64, 56} {
			lines = wrap(main, ww, []int{z}, 3)
			if (len(lines.lines)-1)*int(float64(z)*1.12)+int(float64(z)*0.72) <= H-2*margin-24 {
				break
			}
		}
		px := lines.px
		capH := int(float64(px) * 0.72)
		lh := int(float64(px) * 1.12)
		block := (len(lines.lines)-1)*lh + capH
		top := (H - block) / 2
		for i, l := range lines.lines {
			draw(img, margin, top+i*lh, l, true, px, mainCol)
		}
		return img
	}
	m, mpx := fitLine(main, true, []int{110, 96, 84, 72, 60}, w)
	mainCap := int(float64(mpx) * 0.72)
	if sub == "" {
		draw(img, x, (H-mainCap)/2, m, true, mpx, mainCol)
		return img
	}
	s, spx := fitLine(sub, false, []int{60, 52, 44}, w)
	subCap := int(float64(spx) * 0.72)
	space := int(float64(mpx) * 0.42)
	top := (H - (mainCap + space + subCap)) / 2
	draw(img, x, top, m, true, mpx, mainCol)
	draw(img, x, top+mainCap+space, s, false, spx, soft)
	return img
}

var (
	unsung   = color.RGBA{128, 128, 128, 255} // current line, not yet sung
	upcoming = color.RGBA{84, 84, 84, 255}    // the next line
)

// renderLyrics: the current line huge and bold across the full width (two lines if needed),
// lit white as it is sung, the next line beneath in grey.
func renderLyrics(img *image.RGBA, in Info) {
	ww := W - 2*margin
	var cur wrapped
	for _, z := range []int{132, 116, 104, 92, 80, 70, 60} {
		cur = wrap(in.Lyric, ww, []int{z}, 2)
		if len(cur.lines) <= 2 && !strings.HasSuffix(cur.lines[len(cur.lines)-1], "…") {
			break
		}
	}
	px := cur.px
	capH := int(float64(px) * 0.72)
	lh := int(float64(px) * 1.14)
	block := (len(cur.lines)-1)*lh + capH
	npx := 54
	nextGap := int(float64(px) * 0.55)
	next := ""
	if in.NextLyric != "" {
		next, npx = fitLine(in.NextLyric, false, []int{npx, 46}, ww)
		block += nextGap + int(float64(npx)*0.72)
	}
	top := max(margin/2, (H-block)/2)
	total := 0
	for _, l := range cur.lines {
		total += width(l, true, px)
	}
	lit := in.Sweep * float64(total)
	y := top
	for _, l := range cur.lines {
		w := width(l, true, px)
		draw(img, margin, y, l, true, px, unsung)
		if lit > 0 {
			clip := img.SubImage(image.Rect(0, 0, margin+int(math.Min(lit, float64(w))), H)).(*image.RGBA)
			draw(clip, margin, y, l, true, px, white)
		}
		lit -= float64(w)
		y += lh
	}
	if next != "" {
		draw(img, margin, y-lh+capH+nextGap, next, false, npx, upcoming)
	}
}

type wrapped struct {
	lines []string
	px    int
}

// wrap fits s into at most max lines of width w, trying sizes from largest.
func wrap(s string, w int, sizes []int, max int) wrapped {
	words := strings.Fields(s)
	for _, px := range sizes {
		var lines []string
		cur := ""
		ok := true
		for _, wd := range words {
			try := strings.TrimSpace(cur + " " + wd)
			if width(try, true, px) <= w {
				cur = try
				continue
			}
			if cur == "" { // a single word wider than the line
				ok = false
				break
			}
			lines = append(lines, cur)
			cur = wd
			if width(cur, true, px) > w {
				ok = false
				break
			}
		}
		if cur != "" {
			lines = append(lines, cur)
		}
		if ok && len(lines) <= max {
			return wrapped{lines, px}
		}
	}
	px := sizes[len(sizes)-1]
	l, _ := fitLine(s, true, []int{px}, w)
	return wrapped{[]string{l}, px}
}

// Mirrored returns a copy of img flipped horizontally ("horizontal") or vertically
// ("vertical"); anything else returns img itself. Programs may hand out cached frames, so
// this never writes to img.
func Mirrored(img *image.RGBA, how string) *image.RGBA {
	if how != "horizontal" && how != "vertical" {
		return img
	}
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	out := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		src := img.Pix[y*img.Stride : y*img.Stride+w*4]
		if how == "vertical" {
			copy(out.Pix[(h-1-y)*out.Stride:], src)
			continue
		}
		dst := out.Pix[y*out.Stride : y*out.Stride+w*4]
		for x := 0; x < w; x++ {
			copy(dst[(w-1-x)*4:(w-x)*4], src[x*4:x*4+4])
		}
	}
	return out
}
