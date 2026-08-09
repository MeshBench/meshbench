// Package dsp holds the LoRa baseband maths: modulation, demodulation, and the
// link arithmetic they are judged against.
//
// Everything here is the CPU reference implementation. Per ADR-0004 the GPU
// kernels are a second implementation of the same maths, and a test asserts the
// two agree. The CPU path is not a fallback — it is the oracle, and it is the
// only path CI can run, because neither the dev VM nor a CI runner has a GPU.
package dsp

import "math"

// DefaultNoiseFigureDB is a typical SX1262 receiver noise figure. Real figures
// vary with matching and PCB layout; this is the value the technical design
// page's worked examples use, so changing it invalidates those figures.
const DefaultNoiseFigureDB = 6.0

// thermalNoiseDBmPerHz is kTB at room temperature, in dBm/Hz.
const thermalNoiseDBmPerHz = -174.0

// NoiseFloorDBm returns the thermal noise power in a channel of the given
// bandwidth, for a receiver with the given noise figure.
//
//	N = -174 + 10*log10(BW) + NF
//
// At BW 125 kHz / NF 6 dB this is -117.0 dBm; at 250 kHz, -114.0 dBm. Those two
// figures are pinned by tests because every sensitivity claim in the project is
// measured against them — if this drifts, so does everything downstream, and it
// would drift silently.
func NoiseFloorDBm(bandwidthHz, noiseFigureDB float64) float64 {
	return thermalNoiseDBmPerHz + 10*math.Log10(bandwidthHz) + noiseFigureDB
}

// ProcessingGainDB is the coherent gain of dechirping a spreading-factor sf
// symbol: 10*log10(2^sf).
//
// This is why LoRa decodes below the noise floor — 36.1 dB at SF12 — and it is
// the term that makes a sub-noise-floor "impossible" link work.
func ProcessingGainDB(sf int) float64 {
	return 10 * math.Log10(math.Exp2(float64(sf)))
}

// SamplesPerSymbol is 2^sf: a LoRa symbol carries sf bits as one of 2^sf
// cyclic shifts of the base chirp, sampled at the bandwidth rate.
func SamplesPerSymbol(sf int) int {
	return 1 << sf
}

// SymbolDuration returns a symbol's duration in seconds, 2^sf / BW.
//
// The SF12/SF7 ratio is 32, which is the entire reason a high spreading factor
// is expensive in airtime and in collision exposure.
func SymbolDuration(sf int, bandwidthHz float64) float64 {
	return float64(SamplesPerSymbol(sf)) / bandwidthHz
}
