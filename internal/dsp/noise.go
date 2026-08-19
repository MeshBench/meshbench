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
	//
	// The seed is mixed in, not added to the counter. Adding it makes two seeds
	// the same stream at different offsets — seed s at counter c gives exactly
	// what seed s' gives at counter c+(s-s') — so two "independent" runs share
	// most of their noise wherever their counter ranges overlap. That is
	// invisible in any single run and quietly destroys the point of running a
	// second seed, which is to get an independent realisation.
	z := counter + 0x9E3779B97F4A7C15
	z ^= p.Seed * 0xD6E8FEB86659FD93
	z = (z ^ (z >> 30)) * 0xBF58476D1CE4E5B9
	z = (z ^ (z >> 27)) * 0x94D049BB133111EB
	return z ^ (z >> 31)
}

// normalPair returns two independent standard normals, one inverse-CDF
// evaluation each (Acklam's rational approximation, |error| < 1.15e-9).
//
// It replaced Box-Muller, whose log, sin, cos and sqrt per pair were 46%
// of a whole multi-transmitter waveform run. The inverse CDF is two
// polynomial ratios and at worst one sqrt+log on the far tails - and its
// tails are right, which matters here: the FEC threshold lives in the
// tails, and a cheaper bounded generator would quietly flatter it.
// Realisations differ from the Box-Muller ones, so seeded results moved
// once when this landed; the determinism contract - same seed, same
// stream, any goroutine or GPU lane - is unchanged.
func (p Philox) normalPair(counter uint64) (float64, float64) {
	a := p.uint64At(counter * 2)
	b := p.uint64At(counter*2 + 1)
	u1 := (float64(bits.RotateLeft64(a, 17)>>11) + 0.5) / float64(uint64(1)<<53)
	u2 := (float64(b>>11) + 0.5) / float64(uint64(1)<<53)
	return quantile(u1), quantile(u2)
}

// quantileTable is the inverse CDF sampled uniformly across the central
// region, linearly interpolated: two loads and a lerp instead of a
// polynomial ratio per sample. 8192 cells keep the interpolation error
// near 1e-7 sigma - far beneath the quantisation any consumer applies -
// and the tails, where the curve bends too hard for a table and the FEC
// threshold actually lives, still go through the exact polynomial.
const (
	quantLow   = 0.02425
	quantHigh  = 1 - quantLow
	quantCells = 8192
)

var quantTable = func() [quantCells + 2]float64 {
	var t [quantCells + 2]float64
	for i := range t {
		u := quantLow + (quantHigh-quantLow)*float64(i)/quantCells
		if u > quantHigh {
			u = quantHigh
		}
		t[i] = invNormalCDF(u)
	}
	return t
}()

func quantile(u float64) float64 {
	if u < quantLow || u > quantHigh {
		return invNormalCDF(u)
	}
	f := (u - quantLow) * (quantCells / (quantHigh - quantLow))
	i := int(f)
	frac := f - float64(i)
	return quantTable[i] + (quantTable[i+1]-quantTable[i])*frac
}

// invNormalCDF is Acklam's rational approximation to the standard normal
// quantile. Central region: one ratio of degree-5 polynomials. Tails
// (|u-0.5| beyond 0.475): the same shape in sqrt(-2 log u), reached by a
// tiny fraction of samples.
func invNormalCDF(u float64) float64 {
	const (
		a1, a2, a3 = -39.69683028665376, 220.9460984245205, -275.9285104469687
		a4, a5, a6 = 138.3577518672690, -30.66479806614716, 2.506628277459239
		b1, b2, b3 = -54.47609879822406, 161.5858368580409, -155.6989798598866
		b4, b5     = 66.80131188771972, -13.28068155288572
		c1, c2, c3 = -0.007784894002430293, -0.3223964580411365, -2.400758277161838
		c4, c5, c6 = -2.549732539343734, 4.374664141464968, 2.938163982698783
		d1, d2, d3 = 0.007784695709041462, 0.3224671290700398, 2.445134137142996
		d4         = 3.754408661907416
		low, high  = 0.02425, 1 - 0.02425
	)
	switch {
	case u < low:
		q := math.Sqrt(-2 * math.Log(u))
		return (((((c1*q+c2)*q+c3)*q+c4)*q+c5)*q + c6) /
			((((d1*q+d2)*q+d3)*q+d4)*q + 1)
	case u > high:
		q := math.Sqrt(-2 * math.Log(1-u))
		return -(((((c1*q+c2)*q+c3)*q+c4)*q+c5)*q + c6) /
			((((d1*q+d2)*q+d3)*q+d4)*q + 1)
	default:
		q := u - 0.5
		r := q * q
		return (((((a1*r+a2)*r+a3)*r+a4)*r+a5)*r + a6) * q /
			(((((b1*r+b2)*r+b3)*r+b4)*r+b5)*r + 1)
	}
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
