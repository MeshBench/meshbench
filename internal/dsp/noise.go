package dsp

import (
	"math"
	"math/bits"
)

// Philox4x32-style counter-based RNG. Counter-based rather than a stateful
// stream because ADR-0004 requires a seed to reproduce the exact noise
// realisation regardless of goroutine or GPU lane — a shared stateful stream
// cannot promise that, and the GPU could not implement it anyway.
type Philox struct {
	Seed uint64
}

func (p Philox) uint64At(counter uint64) uint64 {
	// A small, well-mixed counter hash (splitmix64). Cheap, reproducible, and
	// trivially portable to WGSL, which matters more here than cryptographic
	// quality: this is simulation noise, not key material.
	z := counter + p.Seed + 0x9E3779B97F4A7C15
	z = (z ^ (z >> 30)) * 0xBF58476D1CE4E5B9
	z = (z ^ (z >> 27)) * 0x94D049BB133111EB
	return z ^ (z >> 31)
}

// normalPair returns two independent standard normals via Box-Muller.
func (p Philox) normalPair(counter uint64) (float64, float64) {
	a := p.uint64At(counter * 2)
	b := p.uint64At(counter*2 + 1)
	// Convert to (0,1); avoid exactly zero, which would make Log infinite.
	u1 := (float64(bits.RotateLeft64(a, 17)>>11) + 0.5) / float64(uint64(1)<<53)
	u2 := (float64(b>>11) + 0.5) / float64(uint64(1)<<53)
	r := math.Sqrt(-2 * math.Log(u1))
	return r * math.Cos(2*math.Pi*u2), r * math.Sin(2*math.Pi*u2)
}

// AddAWGN adds complex additive white Gaussian noise of the given power (in
// linear units, same scale as the signal) to x, in place.
//
// Deterministic in (seed, offset): the same arguments always produce the same
// realisation, which is what makes a run reproducible.
func (p Philox) AddAWGN(x []complex128, noisePower float64, offset uint64) {
	// Split noise power equally between the I and Q components.
	sigma := math.Sqrt(noisePower / 2)
	for i := range x {
		re, im := p.normalPair(offset + uint64(i))
		x[i] += complex(re*sigma, im*sigma)
	}
}

// SignalPower returns the mean power of a waveform.
func SignalPower(x []complex128) float64 {
	if len(x) == 0 {
		return 0
	}
	var sum float64
	for _, v := range x {
		sum += real(v)*real(v) + imag(v)*imag(v)
	}
	return sum / float64(len(x))
}

// NoisePowerForSNR returns the noise power giving the requested SNR in dB
// against a signal of the given power.
func NoisePowerForSNR(signalPower, snrDB float64) float64 {
	return signalPower / math.Pow(10, snrDB/10)
}
