// The receiver front end: finding a frame in samples nobody aligned.
//
// Everything before this file assumed the engine's gift of perfect timing.
// A real SX126x gets IQ and nothing else: it finds the preamble by watching
// dechirped energy hold still, reads the sync word, and splits timing error
// from frequency error using the SFD's downchirps - an upchirp moves one way
// under both errors, a downchirp moves opposite ways, so two measurements
// separate what one cannot.
package dsp

import (
	"math"
	"math/cmplx"
)

// FrameLayout is where a LoRa frame's parts sit in a sample stream, in
// symbols: preamble upchirps, two sync-word upchirps, 2.25 downchirp SFD,
// then data. The 2.25 is RadioLib's sfCoeff1 = 4.25 minus the two sync
// symbols - the airtime formula and this layout describe the same frame.
type FrameLayout struct {
	SF       int
	Preamble int
	SyncA    int // sync-word symbol values, upper-nibble convention
	SyncB    int
}

// StandardSync is the private-network sync word 0x12 as two symbol values.
func StandardSync(sf int) (int, int) {
	return 1 << (sf - 4), 2 << (sf - 4)
}

// sfdSamples is the SFD's length in samples: 2.25 downchirp symbols.
func sfdSamples(sf int) int { return SamplesPerSymbol(sf) * 9 / 4 }

// DataStart is the sample index of the first data symbol.
func (l FrameLayout) DataStart() int {
	return (l.Preamble+2)*SamplesPerSymbol(l.SF) + sfdSamples(l.SF)
}

// FrameSamples renders a whole frame: preamble, sync word, SFD, data.
//
// The one modulator every consumer shares - the verdict path, the waterfall
// and the SDR observers all get their samples from here.
func (l FrameLayout) FrameSamples(data []int) []complex128 {
	m := Modulator{SF: l.SF}
	n := SamplesPerSymbol(l.SF)
	out := make([]complex128, 0, l.DataStart()+len(data)*n)
	for i := 0; i < l.Preamble; i++ {
		out = append(out, m.BaseUpchirp()...)
	}
	out = append(out, m.ModulateSymbol(l.SyncA)...)
	out = append(out, m.ModulateSymbol(l.SyncB)...)
	down := downchirpFor(l.SF)
	out = append(out, down...)
	out = append(out, down...)
	out = append(out, down[:n/4]...)
	for _, s := range data {
		out = append(out, m.ModulateSymbol(s)...)
	}
	return out
}

// downchirpFor is the conjugate of the base upchirp - the SFD's sweep.
func downchirpFor(sf int) []complex128 {
	base := baseFor(sf)
	out := make([]complex128, len(base))
	for i, v := range base {
		out[i] = cmplx.Conj(v)
	}
	return out
}

// Sync is one detection: where the frame starts and how far off frequency it
// arrived, as the receiver estimated them - not as the engine knows them.
type Sync struct {
	// DataStart is the sample index of the first data symbol.
	DataStart int
	// CFOBins is the integer carrier-frequency offset, in FFT bins.
	CFOBins int
	// PreambleConfidence is the dechirped peak-to-second-peak ratio at lock -
	// the demodulator's own confidence measure, kept as telemetry.
	PreambleConfidence float64
}

// Detect finds a frame in iq, or reports that no receiver honestly could.
//
// The search dechirps symbol-sized windows and looks for the preamble's
// signature: consecutive windows whose peak bin holds still. The SFD's
// downchirps then pin the boundary: dechirped against an upchirp reference
// they peak at the mirrored bin, so bin_up + bin_down separates frequency
// error from timing error (bin_up = cfo + sto, bin_down = cfo - sto).
func Detect(iq []complex128, layout FrameLayout) (Sync, bool) {
	sf := layout.SF
	n := SamplesPerSymbol(sf)
	need := 5 // consecutive stable windows to call it a preamble
	if layout.Preamble < need+1 {
		need = layout.Preamble - 1
	}

	d := Demodulator{SF: sf}
	scratch := make([]complex128, n)
	run, runBin, foundAt := 0, -1, -1
	var confAtLock float64
	for at := 0; at+n <= len(iq); at += n {
		bin, conf := d.DemodulateSymbolInto(scratch, iq[at:at+n])
		near := bin == runBin || bin == (runBin+1)%n || (bin+1)%n == runBin
		if near {
			run++
		} else {
			run, runBin = 1, bin
		}
		if run >= need {
			foundAt = at - (need-1)*n
			confAtLock = conf
			break
		}
	}
	if foundAt < 0 {
		return Sync{}, false
	}

	// The preamble's own bin is cfo+sto folded together; the SFD separates
	// them. Search forward for the downchirps: dechirping a downchirp with
	// the upchirp reference concentrates at the mirror bin.
	up := baseFor(sf)
	sfdStart := -1
	binUp := runBin
	var binDown int
	for at := foundAt + need*n; at+n <= len(iq) && at < foundAt+(layout.Preamble+4)*n; at += n {
		// A window is a downchirp when conjugate-dechirping (multiplying by
		// the base chirp itself) concentrates its energy.
		for i := 0; i < n; i++ {
			scratch[i] = iq[at+i] * up[i]
		}
		FFT(scratch)
		peak, mean := peakAndMean(scratch)
		if peak > 6*mean {
			binDown = argmax(scratch)
			sfdStart = at
			break
		}
	}
	if sfdStart < 0 {
		return Sync{}, false
	}

	// Under this dechirp convention, calibrated by test rather than derived
	// on paper: bin_up = cfo - sto and bin_down = cfo + sto. Two equations,
	// two unknowns - the entire reason the SFD's downchirps exist.
	su, sd := signedBin(binUp, n), signedBin(binDown, n)
	cfo := (su + sd) / 2
	sto := (sd - su) / 2

	// A timing offset past half a symbol aliases sto by a whole symbol -
	// the two bins wrap together, which leaves cfo exact and sto ambiguous
	// by +-n. The frame itself resolves it: the true data start dechirps
	// into one concentrated bin, the aliased candidates land on the SFD's
	// tail or straddle two symbols and smear. Three FFTs settle it.
	base := sfdStart + sfdSamples(sf) + sto
	d2 := Demodulator{SF: sf}
	upConf := func(at int) float64 {
		if at < 0 || at+n > len(iq) {
			return 0
		}
		win := iq[at : at+n]
		if cfo != 0 {
			// Judge on CFO-corrected samples, or the offset costs every
			// candidate the same concentration and decides nothing.
			corrected := make([]complex128, n)
			copy(corrected, win)
			CorrectCFO(corrected, sf, cfo)
			win = corrected
		}
		_, conf := d2.DemodulateSymbolInto(scratch, win)
		return conf
	}
	// A candidate on any symbol boundary dechirps cleanly, so concentration
	// alone cannot choose between starts a whole symbol apart. What can is
	// the window before it: the true start is preceded by the SFD's
	// downchirps, which dechirp-as-upchirp into mush; a one-late candidate
	// is preceded by data symbol zero, which is pristine.
	bestScore, dataStart := math.Inf(-1), base
	for _, cand := range [3]int{base - n, base, base + n} {
		score := upConf(cand) - upConf(cand-n)
		if score > bestScore {
			bestScore, dataStart = score, cand
		}
	}

	// Fine alignment: a boundary off by one sample shifts every data bin by
	// one and garbles the whole decode, so the coarse estimate is finished
	// to the sample. At each candidate slip the aligned preamble upchirp
	// measures cfo - sto and the aligned SFD downchirp measures cfo + sto;
	// the slip that zeroes the residual sto is the boundary, and the
	// half-sum there is the exact integer CFO.
	bestResidual := 1 << 30
	fineStart, fineCFO := dataStart, cfo
	for delta := -2; delta <= 2; delta++ {
		preAt := dataStart + delta - sfdSamples(sf) - 4*n
		sfdAt := dataStart + delta - 9*n/4
		if preAt < 0 || sfdAt < 0 || sfdAt+n > len(iq) {
			continue
		}
		binU, _ := d2.DemodulateSymbolInto(scratch, iq[preAt:preAt+n])
		for i := 0; i < n; i++ {
			scratch[i] = iq[sfdAt+i] * up[i]
		}
		FFT(scratch)
		binD := argmax(scratch)
		suf, sdf := signedBin(binU, n), signedBin(binD, n)
		residual := sdf - suf
		if residual < 0 {
			residual = -residual
		}
		if residual < bestResidual {
			bestResidual = residual
			fineStart = dataStart + delta
			fineCFO = (suf + sdf) / 2
		}
	}
	return Sync{DataStart: fineStart, CFOBins: fineCFO, PreambleConfidence: confAtLock}, true
}

// CorrectCFO removes an integer-bin frequency offset in place.
func CorrectCFO(iq []complex128, sf, cfoBins int) {
	if cfoBins == 0 {
		return
	}
	n := SamplesPerSymbol(sf)
	step := -2 * math.Pi * float64(cfoBins) / float64(n)
	for i := range iq {
		s, c := math.Sincos(step * float64(i))
		iq[i] *= complex(c, s)
	}
}

// signedBin folds a bin index into the signed range around zero.
func signedBin(b, n int) int {
	if b > n/2 {
		return b - n
	}
	return b
}

func peakAndMean(spectrum []complex128) (peak, mean float64) {
	for _, v := range spectrum {
		p := real(v)*real(v) + imag(v)*imag(v)
		mean += p
		if p > peak {
			peak = p
		}
	}
	mean /= float64(len(spectrum))
	return peak, mean
}

func argmax(spectrum []complex128) int {
	best, at := -1.0, 0
	for i, v := range spectrum {
		p := real(v)*real(v) + imag(v)*imag(v)
		if p > best {
			best, at = p, i
		}
	}
	return at
}

// CADBusy is the chip's channel-activity question asked of one symbol of IQ:
// does dechirped energy concentrate the way a chirp's does? A peak-to-mean
// test, because that is what the silicon's CAD is - the chip detects, it
// does not decode, and detection several dB below the demodulation floor is
// the whole reason listen-before-talk works.
func CADBusy(iq []complex128, sf int) bool {
	n := SamplesPerSymbol(sf)
	if len(iq) < n {
		return false
	}
	base := baseFor(sf)
	scratch := make([]complex128, n)
	for i := 0; i < n; i++ {
		scratch[i] = iq[i] * complex(real(base[i]), -imag(base[i]))
	}
	FFT(scratch)
	peak, mean := peakAndMean(scratch)
	// Peak-to-mean, not peak-to-second: detection has to work below the
	// decode floor, where the second-highest bin is already noise's own
	// maximum. Noise's expected peak over N bins sits near ln(N) times the
	// mean - about 5 to 8 across SF7..12 - while a chirp at the decode floor
	// concentrates 20x or more. Twelve splits the two with margin on both
	// sides, which is exactly the kind of threshold a real CAD block is.
	return peak > 12*mean
}
