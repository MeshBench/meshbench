package dsp_test

import (
	"math"
	"testing"

	"github.com/MeshBench/meshbench/internal/dsp"
)

func frameInNoise(t *testing.T, sf, offsetSamples, cfoBins int, snrDB float64) ([]complex128, dsp.FrameLayout, int) {
	t.Helper()
	a, b := dsp.StandardSync(sf)
	layout := dsp.FrameLayout{SF: sf, Preamble: 16, SyncA: a, SyncB: b}
	data := make([]int, 24)
	for i := range data {
		data[i] = (i * 37) % dsp.SamplesPerSymbol(sf)
	}
	frame := layout.FrameSamples(data)

	if cfoBins != 0 {
		n := dsp.SamplesPerSymbol(sf)
		step := 2 * math.Pi * float64(cfoBins) / float64(n)
		for i := range frame {
			s, c := math.Sincos(step * float64(i))
			frame[i] *= complex(c, s)
		}
	}

	buf := make([]complex128, offsetSamples+len(frame)+4*dsp.SamplesPerSymbol(sf))
	copy(buf[offsetSamples:], frame)
	sig := dsp.SignalPower(frame)
	noise := dsp.NoisePowerForSNR(sig, snrDB)
	dsp.Philox{Seed: 8181}.AddAWGN(buf, noise, 0)
	return buf, layout, offsetSamples + layout.DataStart()
}

// The front end must find a frame nobody aligned: arbitrary sample offset,
// clean signal - and report where the data starts, to within a couple of
// samples of the truth the test kept to itself.
func TestDetectFindsAnUnalignedFrame(t *testing.T) {
	for _, sf := range []int{7, 9, 11} {
		for _, off := range []int{0, 1, 137, 1000, 2049} {
			buf, layout, trueStart := frameInNoise(t, sf, off, 0, 20)
			s, ok := dsp.Detect(buf, layout)
			if !ok {
				t.Fatalf("SF%d offset %d: no frame found", sf, off)
			}
			if d := s.DataStart - trueStart; d < -2 || d > 2 {
				t.Fatalf("SF%d offset %d: data start off by %d samples", sf, off, d)
			}
			if s.CFOBins != 0 {
				t.Fatalf("SF%d offset %d: phantom CFO %d bins", sf, off, s.CFOBins)
			}
		}
	}
}

// A frequency offset must be measured, not absorbed into timing: the SFD's
// downchirps are the second equation that separates the two.
func TestDetectSeparatesCFOFromTiming(t *testing.T) {
	for _, cfo := range []int{-3, -1, 1, 4} {
		buf, layout, trueStart := frameInNoise(t, 9, 300, cfo, 20)
		s, ok := dsp.Detect(buf, layout)
		if !ok {
			t.Fatalf("cfo %d: no frame found", cfo)
		}
		if s.CFOBins != cfo {
			t.Fatalf("cfo %d: estimated %d", cfo, s.CFOBins)
		}
		if d := s.DataStart - trueStart; d < -2 || d > 2 {
			t.Fatalf("cfo %d: data start off by %d samples", cfo, s.DataStart-trueStart)
		}
	}
}

// Noise alone must never yield a lock: a front end that hallucinates
// preambles turns every quiet channel into traffic.
func TestDetectRefusesPureNoise(t *testing.T) {
	buf := make([]complex128, 64*dsp.SamplesPerSymbol(9))
	dsp.Philox{Seed: 4242}.AddAWGN(buf, 1.0, 0)
	if _, ok := dsp.Detect(buf, dsp.FrameLayout{SF: 9, Preamble: 16}); ok {
		t.Fatal("detected a frame in pure noise")
	}
}

// After CFO correction the data decodes exactly - the estimate is good
// enough to hand to the demodulator, not merely to report.
func TestCorrectCFORestoresTheSymbols(t *testing.T) {
	sf, cfo := 8, 3
	buf, layout, _ := frameInNoise(t, sf, 512, cfo, 25)
	s, ok := dsp.Detect(buf, layout)
	if !ok {
		t.Fatal("no lock")
	}
	dsp.CorrectCFO(buf, sf, s.CFOBins)
	d := dsp.Demodulator{SF: sf}
	n := dsp.SamplesPerSymbol(sf)
	for i := 0; i < 24; i++ {
		at := s.DataStart + i*n
		got, _ := d.DemodulateSymbol(buf[at : at+n])
		want := (i * 37) % n
		if got != want {
			t.Fatalf("symbol %d: got %d want %d after CFO correction", i, got, want)
		}
	}
}

// CAD says busy over a preamble and quiet over noise - the chip's detector,
// several dB below the decode floor.
func TestCADBusy(t *testing.T) {
	sf := 9
	n := dsp.SamplesPerSymbol(sf)
	m := dsp.Modulator{SF: sf}

	quiet := make([]complex128, n)
	dsp.Philox{Seed: 77}.AddAWGN(quiet, 1.0, 0)
	if dsp.CADBusy(quiet, sf) {
		t.Fatal("CAD fired on pure noise")
	}

	chirp := m.BaseUpchirp()
	sig := dsp.SignalPower(chirp)
	noise := dsp.NoisePowerForSNR(sig, dsp.RequiredSNRdB[sf]) // at the decode floor
	dsp.Philox{Seed: 78}.AddAWGN(chirp, noise, 0)
	if !dsp.CADBusy(chirp, sf) {
		t.Fatal("CAD missed a preamble at the demodulation floor")
	}
}
