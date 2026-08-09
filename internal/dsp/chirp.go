package dsp

import (
	"math"
	"sync"
)

// Modulator turns LoRa symbols into complex baseband, at one sample per Hz of
// bandwidth — so a symbol is exactly N = 2^SF samples.
type Modulator struct {
	SF int
}

// BaseUpchirp returns the unmodulated reference chirp: the sweep every symbol is
// a cyclic shift of, and the conjugate of what the demodulator dechirps with.
//
// A copy, because callers own what they are given. Internally the shared cached
// one is used instead — see baseFor.
func (m Modulator) BaseUpchirp() []complex128 {
	base := baseFor(m.SF)
	out := make([]complex128, len(base))
	copy(out, base)
	return out
}

// baseCache holds one reference chirp per spreading factor.
//
// The chirp depends on nothing but SF, and building it costs 2^SF sine and
// cosine pairs — 4096 of them at SF12. Rebuilding it per symbol, which is what
// the demodulator used to do, dominated the entire Monte Carlo sensitivity
// sweep: more time went into regenerating a constant than into the FFTs it
// feeds.
//
// The cached slice is read-only. Everything internal treats it as such, and
// BaseUpchirp hands out copies so an external caller cannot corrupt it.
var baseCache sync.Map // int -> []complex128

func baseFor(sf int) []complex128 {
	if v, ok := baseCache.Load(sf); ok {
		return v.([]complex128)
	}
	n := SamplesPerSymbol(sf)
	out := make([]complex128, n)
	for i := 0; i < n; i++ {
		out[i] = chirpSample(i, 0, n)
	}
	actual, _ := baseCache.LoadOrStore(sf, out)
	return actual.([]complex128)
}

// ModulateSymbol returns the waveform for one symbol value s in [0, 2^SF).
//
// A LoRa symbol is the base chirp cyclically shifted by s. Phase is accumulated
// from the shifted index rather than the sample index, which is what makes the
// symbol wrap continuously instead of stepping at the fold.
func (m Modulator) ModulateSymbol(s int) []complex128 {
	out := make([]complex128, SamplesPerSymbol(m.SF))
	m.ModulateSymbolInto(out, s)
	return out
}

// ModulateSymbolInto writes a symbol into a buffer the caller owns.
//
// The Monte Carlo sensitivity sweep modulates millions of symbols, and at SF12
// each one is a 64 kB allocation. Handing the same buffer back is not a
// micro-optimisation there — it is most of the garbage the sweep produces.
func (m Modulator) ModulateSymbolInto(dst []complex128, s int) {
	// x_s[i] = chirpSample(i, s, n) uses k = (s+i) mod n, and the base chirp is
	// chirpSample(j, 0, n) with k = j. So a modulated symbol is exactly the base
	// chirp rotated by s — no trigonometry at all, just two copies. Identical
	// bit for bit, which TestModulationIsACyclicShift asserts.
	base := baseFor(m.SF)
	n := len(base)
	if len(dst) < n {
		panic("dsp: ModulateSymbolInto needs a buffer of 2^SF samples")
	}
	s = ((s % n) + n) % n
	copy(dst[:n], base[s:])
	copy(dst[n-s:n], base[:s])
}

// Modulate concatenates symbols into one waveform.
func (m Modulator) Modulate(symbols []int) []complex128 {
	n := SamplesPerSymbol(m.SF)
	out := make([]complex128, 0, n*len(symbols))
	for _, s := range symbols {
		out = append(out, m.ModulateSymbol(s)...)
	}
	return out
}

// chirpSample is the sample at index i of a chirp shifted by s over N samples:
//
//	k    = (s + i) mod N
//	x[i] = exp( j2π ( k²/(2N) − k/2 ) )
func chirpSample(i, s, n int) complex128 {
	k := float64((s + i) % n)
	nf := float64(n)
	phase := 2 * math.Pi * (k*k/(2*nf) - k/2)
	return complex(math.Cos(phase), math.Sin(phase))
}

// Demodulator recovers symbols by dechirping and taking the FFT peak — what a
// real LoRa receiver does, and the reason capture effect and sensitivity are
// emergent here rather than rules someone wrote.
type Demodulator struct {
	SF int
}

// DemodulateSymbol returns the symbol value and a confidence ratio (peak
// magnitude over the next strongest bin). The ratio is what the UI shows when
// explaining a marginal decode, and what makes a collision legible.
func (d Demodulator) DemodulateSymbol(rx []complex128) (symbol int, confidence float64) {
	return d.DemodulateSymbolInto(make([]complex128, SamplesPerSymbol(d.SF)), rx)
}

// DemodulateSymbolInto is DemodulateSymbol with a scratch buffer the caller
// owns, for the same reason ModulateSymbolInto exists.
func (d Demodulator) DemodulateSymbolInto(scratch, rx []complex128) (symbol int, confidence float64) {
	n := SamplesPerSymbol(d.SF)
	if len(rx) < n || len(scratch) < n {
		return 0, 0
	}
	base := baseFor(d.SF)
	buf := scratch[:n]
	for i := 0; i < n; i++ {
		// multiply by the conjugate of the base chirp
		buf[i] = rx[i] * complex(real(base[i]), -imag(base[i]))
	}
	FFT(buf)

	peakIdx, peak, second := 0, 0.0, 0.0
	for i, v := range buf {
		mag := real(v)*real(v) + imag(v)*imag(v)
		if mag > peak {
			second, peak, peakIdx = peak, mag, i
		} else if mag > second {
			second = mag
		}
	}
	if second <= 0 {
		return peakIdx, math.Inf(1)
	}
	return peakIdx, math.Sqrt(peak / second)
}

// Demodulate recovers a sequence of symbols.
func (d Demodulator) Demodulate(rx []complex128) []int {
	n := SamplesPerSymbol(d.SF)
	out := make([]int, 0, len(rx)/n)
	for off := 0; off+n <= len(rx); off += n {
		s, _ := d.DemodulateSymbol(rx[off : off+n])
		out = append(out, s)
	}
	return out
}
