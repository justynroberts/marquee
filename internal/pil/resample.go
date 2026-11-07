// Package pil reproduces the parts of Pillow's image arithmetic that Marquee's output depends on
// (Lanczos resampling and RGBA premultiplication), bit for bit, so frames match the reference
// implementation. Ported from Pillow 13 libImaging/Resample.c and Convert.c (HPND licence).
package pil

import "math"

const precisionBits = 32 - 8 - 2

func sinc(x float64) float64 {
	if x == 0 {
		return 1
	}
	x *= math.Pi
	return math.Sin(x) / x
}

func lanczos(x float64) float64 {
	if -3 <= x && x < 3 {
		return sinc(x) * sinc(x/3)
	}
	return 0
}

func clip8(v int32) uint8 {
	v >>= precisionBits
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return uint8(v)
}

func bilinear(x float64) float64 {
	if x < 0 {
		x = -x
	}
	if x < 1 {
		return 1 - x
	}
	return 0
}

// Filter is a Pillow resampling filter.
type Filter struct {
	fn      func(float64) float64
	support float64
}

// Pillow's LANCZOS and BILINEAR filters.
var (
	Lanczos  = Filter{lanczos, 3}
	Bilinear = Filter{bilinear, 1}
)

// precompute is precompute_coeffs + normalize_coeffs_8bpc.
func precompute(flt Filter, inSize int, in0, in1 float32, outSize int) (ksize int, bounds []int, kk []int32) {
	scale := float64(in1-in0) / float64(outSize)
	filterscale := math.Max(scale, 1)
	support := flt.support * filterscale
	ksize = int(math.Ceil(support))*2 + 1
	pre := make([]float64, outSize*ksize)
	bounds = make([]int, outSize*2)
	inv := 1.0 / filterscale
	for xx := 0; xx < outSize; xx++ {
		center := float64(in0) + (float64(xx)+0.5)*scale
		ww := 0.0
		xmin := int(center - support + 0.5)
		if xmin < 0 {
			xmin = 0
		}
		xmax := int(center + support + 0.5)
		if xmax > inSize {
			xmax = inSize
		}
		xmax -= xmin
		k := pre[xx*ksize : (xx+1)*ksize]
		for x := 0; x < xmax; x++ {
			w := flt.fn((float64(x+xmin) - center + 0.5) * inv)
			k[x] = w
			ww += w
		}
		if ww != 0 {
			for x := 0; x < xmax; x++ {
				k[x] /= ww
			}
		}
		bounds[xx*2], bounds[xx*2+1] = xmin, xmax
	}
	kk = make([]int32, len(pre))
	for i, v := range pre {
		if v < 0 {
			kk[i] = int32(-0.5 + v*(1<<precisionBits))
		} else {
			kk[i] = int32(0.5 + v*(1<<precisionBits))
		}
	}
	return ksize, bounds, kk
}

// Image is a 4-channel 8-bit image (any channel meaning; Pillow resamples each independently).
type Image struct {
	W, H int
	Pix  []uint8 // W*H*4
}

// ResizeLanczos is Image.resize((w, h), Image.LANCZOS) on a 4-channel image with the full box.
func ResizeLanczos(in Image, w, h int) Image { return Resize(in, w, h, Lanczos) }

// Resize is Image.resize((w, h), filter) on a 4-channel image with the full box.
func Resize(in Image, w, h int, flt Filter) Image {
	box := [4]float32{0, 0, float32(in.W), float32(in.H)}
	needH := w != in.W || box[0] != 0 || int(box[2]) != w
	needV := h != in.H || box[1] != 0 || int(box[3]) != h
	horizontalFirst := !((in.H-h) > 0 && (in.H-h) > (in.W-w)*2)
	var ksV, ksH int
	var bV, bH []int
	var kV, kH []int32
	if (needH && horizontalFirst) || needV {
		ksV, bV, kV = precompute(flt, in.H, box[1], box[3], h)
	}
	yFirst, yLast := 0, in.H
	if needH {
		if horizontalFirst {
			yFirst = bV[0]
			yLast = bV[h*2-2] + bV[h*2-1]
			if yFirst != 0 {
				for i := 0; i < h; i++ {
					bV[i*2] -= yFirst
				}
			}
		}
		ksH, bH, kH = precompute(flt, in.W, box[0], box[2], w)
	}
	cur := in
	if horizontalFirst {
		if needH {
			cur = horizontal(cur, w, yLast-yFirst, yFirst, ksH, bH, kH)
		}
		if needV {
			cur = vertical(cur, w, h, ksV, bV, kV)
		}
	} else {
		if needV {
			cur = vertical(cur, cur.W, h, ksV, bV, kV)
		}
		if needH {
			cur = horizontal(cur, w, h, 0, ksH, bH, kH)
		}
	}
	if !needH && !needV {
		out := Image{in.W, in.H, append([]uint8(nil), in.Pix...)}
		return out
	}
	return cur
}

func horizontal(in Image, w, h, offset, ksize int, bounds []int, kk []int32) Image {
	out := Image{w, h, make([]uint8, w*h*4)}
	for yy := 0; yy < h; yy++ {
		row := in.Pix[(yy+offset)*in.W*4:]
		for xx := 0; xx < w; xx++ {
			xmin, xmax := bounds[xx*2], bounds[xx*2+1]
			k := kk[xx*ksize:]
			for c := 0; c < 4; c++ {
				ss := int32(1 << (precisionBits - 1))
				for x := 0; x < xmax; x++ {
					ss += int32(row[(x+xmin)*4+c]) * k[x]
				}
				out.Pix[(yy*w+xx)*4+c] = clip8(ss)
			}
		}
	}
	return out
}

func vertical(in Image, w, h, ksize int, bounds []int, kk []int32) Image {
	out := Image{w, h, make([]uint8, w*h*4)}
	for yy := 0; yy < h; yy++ {
		ymin, ymax := bounds[yy*2], bounds[yy*2+1]
		k := kk[yy*ksize:]
		for xx := 0; xx < w; xx++ {
			for c := 0; c < 4; c++ {
				ss := int32(1 << (precisionBits - 1))
				for y := 0; y < ymax; y++ {
					ss += int32(in.Pix[((y+ymin)*in.W+xx)*4+c]) * k[y]
				}
				out.Pix[(yy*w+xx)*4+c] = clip8(ss)
			}
		}
	}
	return out
}

func muldiv255(a, b uint32) uint8 {
	t := a*b + 128
	return uint8(((t >> 8) + t) >> 8)
}

// Premultiply is Pillow's RGBA -> RGBa conversion.
func Premultiply(im Image) Image {
	out := Image{im.W, im.H, make([]uint8, len(im.Pix))}
	for i := 0; i < len(im.Pix); i += 4 {
		a := uint32(im.Pix[i+3])
		out.Pix[i] = muldiv255(uint32(im.Pix[i]), a)
		out.Pix[i+1] = muldiv255(uint32(im.Pix[i+1]), a)
		out.Pix[i+2] = muldiv255(uint32(im.Pix[i+2]), a)
		out.Pix[i+3] = im.Pix[i+3]
	}
	return out
}

// Unpremultiply is Pillow's RGBa -> RGBA conversion.
func Unpremultiply(im Image) Image {
	out := Image{im.W, im.H, make([]uint8, len(im.Pix))}
	for i := 0; i < len(im.Pix); i += 4 {
		a := uint32(im.Pix[i+3])
		if a == 255 || a == 0 {
			copy(out.Pix[i:i+3], im.Pix[i:i+3])
		} else {
			for c := 0; c < 3; c++ {
				out.Pix[i+c] = uint8(min(255, 255*uint32(im.Pix[i+c])/a))
			}
		}
		out.Pix[i+3] = im.Pix[i+3]
	}
	return out
}

// ResizeRGBA is Image.resize on an RGBA image: premultiply, resample, unpremultiply.
func ResizeRGBA(im Image, w, h int) Image {
	return Unpremultiply(ResizeLanczos(Premultiply(im), w, h))
}
