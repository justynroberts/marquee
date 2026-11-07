package hud

import (
	"bytes"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func lit(img *image.RGBA) int {
	n := 0
	for i := 0; i < len(img.Pix); i += 4 {
		if img.Pix[i] > 128 || img.Pix[i+1] > 128 {
			n++
		}
	}
	return n
}

func TestRender(t *testing.T) {
	now := time.Date(2026, 11, 7, 15, 42, 9, 0, time.Local)
	cases := map[string]Info{
		"plain":   {Now: now},
		"message": {Now: now, Message: "Recording — do not disturb"},
		"track":   {Now: now, Track: "Everything In Its Right Place — Radiohead"},
		"alert":   {Now: now, Alert: "INCIDENT: checkout API error rate above 5% in eu-west-1 for ten minutes"},
		"lyrics":  {Now: now, Lyrics: true, Lyric: "And I'll be the one who takes the blame when the whole thing falls apart tonight", NextLyric: "I got what you need", Sweep: 0.4},
		"lyric1":  {Now: now, Lyrics: true, Lyric: "I got what you want", NextLyric: "I got what you need", Sweep: 0.6},
		"long":    {Now: now, Message: "A very long message that will certainly not fit on one line at any size we offer here"},
	}
	dir := os.Getenv("HUD_PNG")
	for name, in := range cases {
		img := Render(in)
		if lit(img) < 2000 {
			t.Errorf("%s: almost nothing drawn", name)
		}
		// pure black background: the corners stay black
		if o := img.PixOffset(W-2, H-2); img.Pix[o] != 0 || img.Pix[o+1] != 0 {
			t.Errorf("%s: background not black", name)
		}
		if dir != "" {
			var b bytes.Buffer
			_ = png.Encode(&b, Mirrored(img, "horizontal"))
			_ = os.WriteFile(filepath.Join(dir, "hud-"+name+"-mirror.png"), b.Bytes(), 0o644)
			b.Reset()
			_ = png.Encode(&b, img)
			_ = os.WriteFile(filepath.Join(dir, "hud-"+name+"-plain.png"), b.Bytes(), 0o644)
		}
	}
}

func TestMirrored(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 3, 2))
	img.Pix[0] = 9 // (0,0)
	h := Mirrored(img, "horizontal")
	if h.Pix[h.PixOffset(2, 0)] != 9 || img.Pix[0] != 9 {
		t.Fatal("horizontal")
	}
	v := Mirrored(img, "vertical")
	if v.Pix[v.PixOffset(0, 1)] != 9 {
		t.Fatal("vertical")
	}
	if Mirrored(img, "") != img {
		t.Fatal("off should return the same image")
	}
}
