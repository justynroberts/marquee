package engine

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/gif"
	"math"
	"os"

	"golang.org/x/image/webp"

	"github.com/justynroberts/marquee/internal/led"
	"github.com/justynroberts/marquee/internal/pil"
)

// FitCover scales (Lanczos) and crops src to fill the panel, centred, like the Python fit_cover.
func FitCover(src image.Image) *image.RGBA {
	b := src.Bounds()
	scale := math.Max(float64(led.W)/float64(b.Dx()), float64(led.H)/float64(b.Dy()))
	w := max(1, int(math.Round(float64(b.Dx())*scale)))
	h := max(1, int(math.Round(float64(b.Dy())*scale)))
	// Pillow's convert("RGB") discards alpha and keeps the stored colour: do the same.
	flat := image.NewRGBA(b)
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			c := color.NRGBAModel.Convert(src.At(x, y)).(color.NRGBA)
			o := flat.PixOffset(x, y)
			flat.Pix[o], flat.Pix[o+1], flat.Pix[o+2], flat.Pix[o+3] = c.R, c.G, c.B, 255
		}
	}
	// img.resize(..., Image.LANCZOS) on the RGB image, exactly as Pillow computes it.
	r := pil.Resize(pil.Image{W: b.Dx(), H: b.Dy(), Pix: flat.Pix}, w, h, pil.Lanczos)
	tmp := &image.RGBA{Pix: r.Pix, Stride: w * 4, Rect: image.Rect(0, 0, w, h)}
	out := image.NewRGBA(image.Rect(0, 0, led.W, led.H))
	draw.Draw(out, out.Bounds(), tmp, image.Pt((w-led.W)/2, (h-led.H)/2), draw.Src)
	return out
}

// frameDuration applies Pillow/panel.py's rule: missing or zero = 100 ms, floor 50 ms.
func frameDuration(ms int) float64 {
	if ms <= 0 {
		ms = 100
	}
	return math.Max(0.05, float64(ms)/1000)
}

const maxFrames = 600 // cap memory for very long animations, as panel.py does

func decodeMedia(name string, data []byte) (*mediaProgram, error) {
	m := &mediaProgram{name: "media:" + baseName(name)}
	add := func(img image.Image, ms int) {
		m.frames = append(m.frames, FitCover(img))
		d := frameDuration(ms)
		m.delays = append(m.delays, d)
		m.total += d
	}
	if bytes.HasPrefix(data, []byte("GIF8")) {
		g, err := gif.DecodeAll(bytes.NewReader(data))
		if err != nil {
			return nil, fmt.Errorf("bad GIF: %w", err)
		}
		for _, f := range composeGIF(g) {
			add(f.img, f.ms)
			if len(m.frames) >= maxFrames {
				break
			}
		}
	} else if isAnimatedWebP(data) {
		frames, err := decodeAnimatedWebP(data)
		if err != nil {
			return nil, fmt.Errorf("bad WebP: %w", err)
		}
		for _, f := range frames {
			add(f.img, f.ms)
			if len(m.frames) >= maxFrames {
				break
			}
		}
	} else {
		img, _, err := image.Decode(bytes.NewReader(data))
		if err != nil {
			return nil, fmt.Errorf("not an image (PNG, JPEG, GIF, WebP): %w", err)
		}
		add(img, 0)
	}
	if len(m.frames) == 0 {
		return nil, errors.New("image has no frames")
	}
	return m, nil
}

func loadMedia(path string) (*mediaProgram, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return decodeMedia(path, data)
}

type timedFrame struct {
	img image.Image
	ms  int
}

// composeGIF renders each GIF frame onto the logical screen, honouring disposal methods.
func composeGIF(g *gif.GIF) []timedFrame {
	w, h := g.Config.Width, g.Config.Height
	if w == 0 || h == 0 {
		for _, f := range g.Image {
			w, h = max(w, f.Bounds().Max.X), max(h, f.Bounds().Max.Y)
		}
	}
	canvas := image.NewRGBA(image.Rect(0, 0, w, h))
	var out []timedFrame
	for i, fr := range g.Image {
		disposal := byte(0)
		if i < len(g.Disposal) {
			disposal = g.Disposal[i]
		}
		var prev *image.RGBA
		if disposal == gif.DisposalPrevious {
			prev = image.NewRGBA(canvas.Bounds())
			copy(prev.Pix, canvas.Pix)
		}
		draw.Draw(canvas, fr.Bounds(), fr, fr.Bounds().Min, draw.Over)
		snap := image.NewRGBA(canvas.Bounds())
		copy(snap.Pix, canvas.Pix)
		ms := 0
		if i < len(g.Delay) {
			ms = g.Delay[i] * 10
		}
		out = append(out, timedFrame{snap, ms})
		switch disposal {
		case gif.DisposalBackground:
			draw.Draw(canvas, fr.Bounds(), image.Transparent, image.Point{}, draw.Src)
		case gif.DisposalPrevious:
			canvas = prev
		}
	}
	return out
}

// ---------------------------------------------------------------- animated WebP

type riffChunk struct {
	id   string
	data []byte
}

func riffChunks(b []byte) []riffChunk {
	var out []riffChunk
	for len(b) >= 8 {
		id := string(b[:4])
		n := int(binary.LittleEndian.Uint32(b[4:8]))
		if 8+n > len(b) {
			break
		}
		out = append(out, riffChunk{id, b[8 : 8+n]})
		b = b[8+n+n%2:]
	}
	return out
}

func isAnimatedWebP(data []byte) bool {
	if len(data) < 30 || string(data[:4]) != "RIFF" || string(data[8:12]) != "WEBP" {
		return false
	}
	for _, c := range riffChunks(data[12:]) {
		if c.id == "ANIM" || c.id == "ANMF" {
			return true
		}
	}
	return false
}

func u24(b []byte) int { return int(b[0]) | int(b[1])<<8 | int(b[2])<<16 }

func put24(b []byte, v int) { b[0], b[1], b[2] = byte(v), byte(v>>8), byte(v>>16) }

func appendChunk(dst []byte, id string, data []byte) []byte {
	dst = append(dst, id...)
	dst = binary.LittleEndian.AppendUint32(dst, uint32(len(data)))
	dst = append(dst, data...)
	if len(data)%2 == 1 {
		dst = append(dst, 0)
	}
	return dst
}

// decodeAnimatedWebP demuxes ANMF frames, decodes each as a still WebP and composites them with
// the frame's blend and dispose flags.
func decodeAnimatedWebP(data []byte) ([]timedFrame, error) {
	var w, h int
	var frames []timedFrame
	var canvas *image.RGBA
	for _, c := range riffChunks(data[12:]) {
		switch c.id {
		case "VP8X":
			if len(c.data) >= 10 {
				w, h = u24(c.data[4:])+1, u24(c.data[7:])+1
				canvas = image.NewRGBA(image.Rect(0, 0, w, h))
			}
		case "ANMF":
			if canvas == nil || len(c.data) < 16 {
				return nil, errors.New("ANMF before VP8X")
			}
			fx, fy := u24(c.data[0:])*2, u24(c.data[3:])*2
			fw, fh := u24(c.data[6:])+1, u24(c.data[9:])+1
			ms := u24(c.data[12:])
			flags := c.data[15]
			blend := flags&2 == 0 // 0 = alpha-blend
			dispose := flags&1 == 1
			// Wrap the frame's bitstream (ALPH + VP8, or VP8L) as a standalone WebP.
			sub := riffChunks(c.data[16:])
			var body []byte
			hasAlpha := false
			for _, sc := range sub {
				if sc.id == "ALPH" {
					hasAlpha = true
				}
			}
			if hasAlpha {
				vp8x := make([]byte, 10)
				vp8x[0] = 0x10 // alpha
				put24(vp8x[4:], fw-1)
				put24(vp8x[7:], fh-1)
				body = appendChunk(body, "VP8X", vp8x)
			}
			for _, sc := range sub {
				body = appendChunk(body, sc.id, sc.data)
			}
			file := append([]byte("RIFF"), binary.LittleEndian.AppendUint32(nil, uint32(4+len(body)))...)
			file = append(append(file, "WEBP"...), body...)
			img, err := webp.Decode(bytes.NewReader(file))
			if err != nil {
				return nil, err
			}
			r := image.Rect(fx, fy, fx+fw, fy+fh)
			op := draw.Over
			if !blend {
				op = draw.Src
			}
			draw.Draw(canvas, r, img, img.Bounds().Min, op)
			snap := image.NewRGBA(canvas.Bounds())
			copy(snap.Pix, canvas.Pix)
			frames = append(frames, timedFrame{snap, ms})
			if dispose {
				draw.Draw(canvas, r, image.Transparent, image.Point{}, draw.Src)
			}
		}
	}
	if len(frames) == 0 {
		return nil, errors.New("no frames")
	}
	return frames, nil
}
