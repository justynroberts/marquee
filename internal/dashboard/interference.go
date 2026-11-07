package dashboard

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"time"

	"github.com/justynroberts/marquee/internal/pil"
)

// Interference is the live-stream visual (ported from stream.py): three drifting circular wave
// sources summed and drawn as contour bands, the strongest crests in vermilion, rendered at half
// resolution and upscaled bilinearly, with a clock and fps label.
type Interference struct {
	small *image.RGBA
	fps   float64
}

const iScale = 2

// NewInterference makes the visual.
func NewInterference() *Interference {
	return &Interference{small: image.NewRGBA(image.Rect(0, 0, W/iScale, H/iScale))}
}

// Frame renders time t; dt is used for the fps readout.
func (p *Interference) Frame(t, dt float64, now time.Time) *image.RGBA {
	if dt > 0 {
		inst := 1 / dt
		if p.fps == 0 {
			p.fps = inst
		} else {
			p.fps += (inst - p.fps) * 0.1
		}
	}
	w, h := W/iScale, H/iScale
	type src struct{ sx, sy, k, amp float64 }
	srcs := [3]src{
		{0.5 + 0.42*math.Sin(t*0.21), 0.5 + 0.35*math.Cos(t*0.33), 0.016, 1.0},
		{0.5 + 0.45*math.Cos(t*0.17+1.3), 0.5 + 0.40*math.Sin(t*0.29), 0.013, 0.9},
		{0.5 + 0.30*math.Sin(t*0.11+2.1), 0.5 + 0.45*math.Sin(t*0.23+0.4), 0.021, 0.7},
	}
	bgF := [3]float64{12, 13, 11}
	inkF := [3]float64{230, 226, 214}
	hotF := [3]float64{255, 91, 56}
	pix := p.small.Pix
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			var f float64
			for _, s := range srcs {
				dx := float64(x) - s.sx*float64(w)
				dy := (float64(y) - s.sy*float64(h)) * 1.6
				f += s.amp * math.Sin(math.Sqrt(dx*dx+dy*dy)*s.k*iScale-t*2.4)
			}
			f /= 2.6
			// numpy evaluates this in float32; keep the same precision for the bands.
			f = float64(float32(f))
			v := f * 4
			band := math.Abs((v - math.Floor(v)) - 0.5) // Python's % is floored
			line := math.Pow(math.Max(0, math.Min(1, 1-band*7)), 1.5)
			crest := math.Max(0, math.Min(1, (f-0.62)*4))
			o := (y*w + x) * 4
			for c := 0; c < 3; c++ {
				ink := bgF[c] + (inkF[c]-bgF[c])*(line*0.75)
				out := ink + (hotF[c]-ink)*(crest*line)
				pix[o+c] = uint8(math.Max(0, math.Min(255, out)))
			}
			pix[o+3] = 255
		}
	}
	// Image.resize((W, H), Image.BILINEAR), exactly as Pillow computes it.
	big := pil.Resize(pil.Image{W: w, H: h, Pix: p.small.Pix}, W, H, pil.Bilinear)
	img := &image.RGBA{Pix: big.Pix, Stride: W * 4, Rect: image.Rect(0, 0, W, H)}
	rect(img, 24, H-92, 640, H-24, color.RGBA{12, 13, 11, 255})
	text(img, 40, H-86, now.Format("15:04:05"), 40, ink)
	text(img, 260, H-74, fmt.Sprintf("%4.1f fps  live stream", p.fps), 22, ink2)
	return img
}
