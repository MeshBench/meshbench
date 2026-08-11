package dsp

import (
	"math"
	"math/cmplx"
)

// FFT is the CPU reference transform. Radix-2 Cooley-Tukey, in place, on a
// power-of-two input — which every LoRa symbol is, since N = 2^SF.
//
// Deliberately a plain textbook implementation: per ADR-0004 this is the oracle
// the GPU kernel is tested against, so it is optimised for being obviously
// correct rather than fast. A wrong FFT does not crash; it produces a plausible
// waterfall and slightly wrong sensitivity that nobody notices for months.
func FFT(x []complex128) {
	n := len(x)
	if n <= 1 {
		return
	}
	if n&(n-1) != 0 {
		panic("dsp: FFT length must be a power of two")
	}
	// bit-reversal permutation
	for i, j := 1, 0; i < n; i++ {
		bit := n >> 1
		for ; j&bit != 0; bit >>= 1 {
			j ^= bit
		}
		j ^= bit
		if i < j {
			x[i], x[j] = x[j], x[i]
		}
	}
	for length := 2; length <= n; length <<= 1 {
		ang := -2 * math.Pi / float64(length)
		wl := complex(math.Cos(ang), math.Sin(ang))
		for i := 0; i < n; i += length {
			w := complex(1, 0)
			for j := 0; j < length/2; j++ {
				u := x[i+j]
				v := x[i+j+length/2] * w
				x[i+j] = u + v
				x[i+j+length/2] = u - v
				w *= wl
			}
		}
	}
}

// IFFT is the inverse transform, in place.
//
// Conjugate, forward, conjugate, scale — the standard identity rather than a
// second hand-written butterfly loop. One transform to get wrong is enough.
func IFFT(x []complex128) {
	n := len(x)
	if n <= 1 {
		return
	}
	for i := range x {
		x[i] = cmplx.Conj(x[i])
	}
	FFT(x)
	s := complex(1/float64(n), 0)
	for i := range x {
		x[i] = cmplx.Conj(x[i]) * s
	}
}
