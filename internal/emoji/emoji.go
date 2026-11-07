// Package emoji renders colour emoji onto the LED grid, from Twemoji graphics embedded in the
// binary (44 px PNGs, 64-colour palettes).
//
// Twemoji graphics: Copyright 2020 Twitter, Inc and other contributors; 2024 jdecked and other
// contributors. Licensed under CC-BY 4.0 (LICENSE-TWEMOJI). https://github.com/jdecked/twemoji
package emoji

import (
	"archive/zip"
	"bytes"
	_ "embed"
	"fmt"
	"image"

	"image/png"
	"io"
	"strings"
	"sync"

	"golang.org/x/image/draw"
)

//go:embed twemoji.zip
var archive []byte

// Attribution must appear wherever the emoji are credited (about panels, docs).
const Attribution = "Emoji from Apple Color Emoji (this Mac) or Twemoji (CC-BY 4.0). Includes FreeType, HarfBuzz and code ported from Pillow"

var (
	once  sync.Once
	files map[string]*zip.File
)

func index() map[string]*zip.File {
	once.Do(func() {
		files = map[string]*zip.File{}
		r, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
		if err != nil {
			return
		}
		for _, f := range r.File {
			files[strings.TrimSuffix(f.Name, ".png")] = f
		}
	})
	return files
}

// Sprite is an emoji at LED resolution.
type Sprite struct {
	W, H int
	Lit  []bool
	Col  []RGBA // straight (un-premultiplied) colour per LED
}

// RGBA is a colour with coverage.
type RGBA struct{ R, G, B, A uint8 }

// key turns an emoji cluster into a Twemoji file name: lowercase hex code points joined with
// "-", without FE0F unless the bare form is missing.
func keys(cluster string) []string {
	var all, noVS []string
	for _, r := range cluster {
		h := fmt.Sprintf("%x", r)
		all = append(all, h)
		if r != 0xfe0f {
			noVS = append(noVS, h)
		}
	}
	return []string{strings.Join(noVS, "-"), strings.Join(all, "-")}
}

var (
	cacheMu sync.Mutex
	cache   = map[string]*Sprite{}
)

// Render returns the emoji scaled to h LEDs tall, or false if it is not in the set.
func Render(cluster string, h int) (*Sprite, bool) {
	ck := fmt.Sprintf("%s@%d", cluster, h)
	cacheMu.Lock()
	if s, ok := cache[ck]; ok {
		cacheMu.Unlock()
		return s, s != nil
	}
	cacheMu.Unlock()
	s := render(cluster, h)
	cacheMu.Lock()
	cache[ck] = s
	cacheMu.Unlock()
	return s, s != nil
}

func render(cluster string, h int) *Sprite {
	if s := renderApple(cluster, h); s != nil {
		return s
	}
	return renderTwemoji(cluster, h)
}

// renderTwemoji is the fallback where Apple Color Emoji is not installed (Linux).
func renderTwemoji(cluster string, h int) *Sprite {
	idx := index()
	var f *zip.File
	for _, k := range keys(cluster) {
		if v, ok := idx[k]; ok {
			f = v
			break
		}
	}
	if f == nil {
		return nil
	}
	rc, err := f.Open()
	if err != nil {
		return nil
	}
	data, _ := io.ReadAll(rc)
	rc.Close()
	src, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		return nil
	}
	h = max(4, h)
	w := max(1, src.Bounds().Dx()*h/src.Bounds().Dy())
	dst := image.NewNRGBA(image.Rect(0, 0, w, h))
	draw.CatmullRom.Scale(dst, dst.Bounds(), src, src.Bounds(), draw.Src, nil)
	s := &Sprite{W: w, H: h, Lit: make([]bool, w*h), Col: make([]RGBA, w*h)}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			o := y*dst.Stride + x*4
			c := RGBA{dst.Pix[o], dst.Pix[o+1], dst.Pix[o+2], dst.Pix[o+3]}
			// brighten slightly: LEDs read darker than the artwork
			c.R, c.G, c.B = boost(c.R), boost(c.G), boost(c.B)
			s.Col[y*w+x] = c
			s.Lit[y*w+x] = c.A > 115
		}
	}
	return s
}

func boost(v uint8) uint8 {
	f := float64(v) * 1.12
	if f > 255 {
		return 255
	}
	return uint8(f)
}

// IsEmoji reports whether r starts an emoji.
func IsEmoji(r rune) bool {
	switch {
	case r >= 0x1F000 && r <= 0x1FAFF, r >= 0x2600 && r <= 0x27BF, r >= 0x2B00 && r <= 0x2BFF,
		r >= 0x2190 && r <= 0x21FF, r >= 0x2300 && r <= 0x23FF:
		return true
	}
	switch r {
	case 0xA9, 0xAE, 0x203C, 0x2049, 0x2122, 0x2139, 0x3030, 0x303D:
		return true
	}
	return false
}

func isJoiner(r rune) bool {
	return r == 0x200D || r == 0xFE0F || r == 0xFE0E || r == 0x20E3 || (r >= 0x1F3FB && r <= 0x1F3FF) || (r >= 0xE0020 && r <= 0xE007F)
}

// Run is a piece of text: plain or one emoji cluster.
type Run struct {
	Text  string
	Emoji bool
}

// Split breaks text into plain runs and emoji clusters (ZWJ sequences, selectors, skin tones,
// flags as regional-indicator pairs).
func Split(s string) []Run {
	var out []Run
	rs := []rune(s)
	var buf []rune
	flush := func() {
		if len(buf) > 0 {
			out = append(out, Run{Text: string(buf)})
			buf = nil
		}
	}
	for i := 0; i < len(rs); {
		r := rs[i]
		if r >= 0x1F1E6 && r <= 0x1F1FF && i+1 < len(rs) && rs[i+1] >= 0x1F1E6 && rs[i+1] <= 0x1F1FF {
			flush()
			out = append(out, Run{Text: string(rs[i : i+2]), Emoji: true})
			i += 2
			continue
		}
		if IsEmoji(r) {
			flush()
			j := i + 1
			for j < len(rs) && (isJoiner(rs[j]) || (rs[j-1] == 0x200D && IsEmoji(rs[j]))) {
				j++
			}
			out = append(out, Run{Text: string(rs[i:j]), Emoji: true})
			i = j
			continue
		}
		if r == 0xFE0F || r == 0x200D {
			i++
			continue
		}
		buf = append(buf, r)
		i++
	}
	flush()
	return out
}

var _ = draw.Src
