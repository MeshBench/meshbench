package dsp

import (
	"math"
	"math/cmplx"
	"sync"
)

// FFT is the CPU reference transform. Radix-2 Cooley-Tukey, in place, on a
// power-of-two input — which every LoRa symbol is, since N = 2^SF.
//
// Deliberately a plain textbook implementation: per ADR-0004 this is the oracle
// the GPU kernel is tested against, so it is optimised for being obviously
// correct rather than fast. A wrong FFT does not crash; it produces a plausible
// waterfall and slightly wrong sensitivity that nobody notices for months.
// fftPlan is the precomputed half of a transform: the bit-reversal swaps
// and the twiddles for every stage, indexed as one flat table. Recomputing
// the rotation chain per call was a quarter of a multi-transmitter waveform
// run; the sizes in play are a handful of powers of two, so the plans are
// tiny and live forever.
type fftPlan struct {
	rev      []int32      // bit-reversal target for each index
	twiddles []complex128 // stage twiddles, stages concatenated
}

var fftPlans sync.Map // map[int]*fftPlan

func planFor(n int) *fftPlan {
	if p, ok := fftPlans.Load(n); ok {
		// fftPlans is package-private and stores *fftPlan and nothing else,
		// here and in the LoadOrStore below.
		return p.(*fftPlan) //nolint:forcetypeassert // private sync.Map, one value type
	}
	p := &fftPlan{rev: make([]int32, n)}
	for i, j := 1, 0; i < n; i++ {
		bit := n >> 1
		for ; j&bit != 0; bit >>= 1 {
			j ^= bit
		}
		j ^= bit
		p.rev[i] = int32(j)
	}
	for length := 2; length <= n; length <<= 1 {
		ang := -2 * math.Pi / float64(length)
		for j := 0; j < length/2; j++ {
			s, c := math.Sincos(ang * float64(j))
			p.twiddles = append(p.twiddles, complex(c, s))
		}
	}
	actual, _ := fftPlans.LoadOrStore(n, p)
	return actual.(*fftPlan) //nolint:forcetypeassert // private sync.Map, one value type
}

func FFT(x []complex128) {
	n := len(x)
	if n <= 1 {
		return
	}
	if n&(n-1) != 0 {
		panic("dsp: FFT length must be a power of two")
	}
	p := planFor(n)
	for i := 1; i < n; i++ {
		if j := int(p.rev[i]); i < j {
			x[i], x[j] = x[j], x[i]
		}
	}
	tw := 0
	for length := 2; length <= n; length <<= 1 {
		half := length / 2
		stage := p.twiddles[tw : tw+half]
		tw += half
		for i := 0; i < n; i += length {
			a, b := x[i:i+half], x[i+half:i+length]
			for j := 0; j < half; j++ {
				u := a[j]
				v := b[j] * stage[j]
				a[j] = u + v
				b[j] = u - v
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
