package engine

import (
	"math"
	"testing"

	"github.com/MeshBench/meshbench/internal/rf/dsp"
	"github.com/MeshBench/meshbench/internal/rf/lora"
)

// The repair rule is the interleaver's guarantee, not a guess, so it is worth
// pinning to the guarantee's own wording: one destroyed symbol costs every
// codeword in its block exactly one bit, and Hamming corrects one bit at CR
// 4/7 and 4/8, detects at 4/6 and does nothing at 4/5.
func TestSurvivesCorruptionFollowsTheInterleaver(t *testing.T) {
	for _, c := range []struct {
		name    string
		damaged float64
		cr      int
		want    bool
	}{
		{"undamaged survives at 4/5", 0, 1, true},
		{"undamaged survives at 4/8", 0, 4, true},
		{"one symbol is one bit per codeword, repairable at 4/7", 1, 3, true},
		{"one symbol is one bit per codeword, repairable at 4/8", 1, 4, true},
		{"4/6 only detects, so one symbol is fatal", 1, 2, false},
		{"4/5 is a bare checksum, so one symbol is fatal", 1, 1, false},
		{"two symbols put two bits in a codeword: nothing repairs that", 2, 4, false},
		{"a long burst is hopeless however good the code", 40, 4, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			repaired, ok := survivesCorruption(c.damaged, c.cr)
			if ok != c.want {
				t.Fatalf("survivesCorruption(%v, CR 4/%d) = %v, want %v",
					c.damaged, c.cr+4, ok, c.want)
			}
			if ok && c.damaged > 0 && repaired == 0 {
				t.Fatal("a repaired packet should report the repair; silence is a bug")
			}
		})
	}
}

// The constant that decides which packets survive a collision, measured
// against the receive chain it is supposed to be a fast twin of.
//
// Two real frames go into one window at a sweep of power ratios, and the
// question asked of our own demodulator is the only one that matters: from how
// far ahead does the stronger one come back intact? If the answer drifts from
// captureThresholdDB, the calculated path is quietly deciding collisions on a
// number the physics does not support - which is exactly the class of thing
// this project treats as a bug rather than a tuning preference.
func TestCaptureThresholdMatchesTheDemodulator(t *testing.T) {
	const (
		sf   = 8
		bw   = 125e3
		cr   = 4
		seed = 4417
	)
	p := phy{sf: sf, bandwidthHz: bw, codingRate: cr}
	params := loraParams(p)

	frame := make([]byte, 24)
	for i := range frame {
		frame[i] = byte(13 + i*7)
	}
	other := make([]byte, 24)
	for i := range other {
		other[i] = byte(211 - i*5)
	}

	// Swept across alignments, not just the easy one. Two symbol-aligned
	// chirps put two clean peaks in the FFT and the stronger wins from almost
	// nothing ahead - which is the best case, not the usual one. An interferer
	// landing part-way through a symbol smears across bins instead, and that
	// is what a real collision does, so the threshold has to be the lead that
	// works whatever the offset.
	n := dsp.SamplesPerSymbol(sf)
	offsets := []int{0, n / 8, n / 4, n / 3, n / 2, (2 * n) / 3, (3 * n) / 4, n + n/5}

	measured := math.Inf(1)
	for lead := 0.0; lead <= 30.0; lead += 0.5 {
		all := true
		for _, off := range offsets {
			if !decodesThrough(t, frame, other, p, params, lead, off, seed) {
				all = false
				break
			}
		}
		if all {
			measured = lead
			break
		}
	}
	if math.IsInf(measured, 1) {
		t.Fatal("the demodulator never captured, even 30 dB ahead: the chain is broken")
	}
	t.Logf("our receive chain captures from %.1f dB ahead, across %d alignments; "+
		"the calculated path requires %d dB, as real hardware does",
		measured, len(offsets), captureThresholdDB)

	// Our chain captures from far less of a lead than a real radio needs, and
	// that is the expected answer rather than a failure: it has no oscillator
	// error, no AGC transient and no quantisation, so two clean tones in an FFT
	// are separated by whichever is larger. Six decibels is the hardware
	// figure, and the calculated path deliberately uses the hardware one - a
	// simulator that captured as easily as its own idealised DSP would let a
	// packet survive collisions that kill it on the bench.
	//
	// So the guard is the relationship, not the value. The constant must stay
	// conservative: if the receive chain ever needs *more* lead than real
	// hardware, the calculated path is the optimistic one and this stops being
	// a safe assumption.
	if measured > float64(captureThresholdDB) {
		t.Fatalf("our receive chain needs %.1f dB of lead to capture but the "+
			"calculated path assumes %d dB is enough: the fast twin is now more "+
			"forgiving than the physics it stands in for", measured, captureThresholdDB)
	}
}

// decodesThrough sums a wanted frame and an interferer at a given power lead
// and reports whether the full receive chain recovers the wanted bytes.
func decodesThrough(t *testing.T, want, interferer []byte, p phy,
	params lora.Params, leadDB float64, offset int, seed uint64) bool {
	t.Helper()

	wanted := frameSamples(want, p)
	noise := frameSamples(interferer, p)

	// Unit-amplitude chirps: the lead is applied to the interferer as
	// attenuation, so the wanted signal keeps its scale and the ratio is
	// exactly leadDB.
	scale := math.Pow(10, -leadDB/20)
	sum := make([]complex128, len(wanted))
	copy(sum, wanted)
	for i := offset; i < len(sum); i++ {
		if j := i - offset; j < len(noise) {
			sum[i] += noise[j] * complex(scale, 0)
		}
	}
	// A little thermal noise, so this is a receiver rather than an algebra
	// exercise: well below the demodulator floor, deterministic in seed.
	rng := dsp.Philox{Seed: seed}
	rng.AddAWGN(sum, 1e-4, 0)

	sync, locked := dsp.Detect(sum, frameLayout(p))
	if !locked {
		return false
	}
	if sync.CFOBins != 0 {
		dsp.CorrectCFO(sum, p.sf, sync.CFOBins)
	}
	d := dsp.Demodulator{SF: p.sf}
	n := dsp.SamplesPerSymbol(p.sf)
	scratch := make([]complex128, n)
	var shifts []int
	for at := sync.DataStart; at+n <= len(sum); at += n {
		got, _ := d.DemodulateSymbolInto(scratch, sum[at:at+n])
		shifts = append(shifts, got)
	}
	payload, ok, _ := lora.Decode(params, shifts)
	if !ok || len(payload) != len(want) {
		return false
	}
	for i := range want {
		if payload[i] != want[i] {
			return false
		}
	}
	return true
}
