package audio

import (
	"math"
	"math/cmplx"
)

// fft is an in-place iterative radix-2 Cooley-Tukey FFT. len(x) must be a power of two.
func fft(x []complex128) {
	n := len(x)
	if n <= 1 {
		return
	}
	// bit-reversal permutation
	j := 0
	for i := 1; i < n; i++ {
		bit := n >> 1
		for ; j&bit != 0; bit >>= 1 {
			j ^= bit
		}
		j ^= bit
		if i < j {
			x[i], x[j] = x[j], x[i]
		}
	}
	for size := 2; size <= n; size <<= 1 {
		half := size >> 1
		step := -2 * math.Pi / float64(size)
		wStep := cmplx.Rect(1, step)
		for start := 0; start < n; start += size {
			w := complex(1, 0)
			for k := 0; k < half; k++ {
				a := x[start+k]
				b := x[start+k+half] * w
				x[start+k] = a + b
				x[start+k+half] = a - b
				w *= wStep
			}
		}
	}
}

// magnitudes returns |FFT| for bins 0..n/2 of real input (windowed by the caller).
func magnitudes(in []float64) []float64 {
	n := len(in)
	buf := make([]complex128, n)
	for i, v := range in {
		buf[i] = complex(v, 0)
	}
	fft(buf)
	out := make([]float64, n/2+1)
	for i := range out {
		out[i] = cmplx.Abs(buf[i])
	}
	return out
}
