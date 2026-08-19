package sdr

import (
	"math"

	"github.com/MeshBench/meshbench/internal/rf/dsp"
)

// Spectrogram is the waterfall: power against frequency, over time.
type Spectrogram struct {
	// Frames[t][b] is power in dB relative to a unit-amplitude carrier.
	// Frequency runs low to high across each frame, with DC in the middle —
	// the order an operator expects to see, not FFT output order.
	Frames [][]float64

	// BinHz is the width of one frequency bin, FrameSeconds the time step
	// between rows, and CentreHz the frequency at the middle column.
	BinHz        float64
	FrameSeconds float64
	CentreHz     float64

	// NoiseFloorDB is the observer's thermal floor across the whole band, on the
	// same scale as Frames. A single bin holds 1/fftSize of it, raised by the
	// window's noise bandwidth — 1.76 dB for Hann — because Frames is calibrated
	// for tones, as a spectrum display should be. Kept so a display can put its
	// colour map somewhere meaningful instead of stretching it to whatever
	// happened to be in view.
	NoiseFloorDB float64
}

// Spectrogram computes the waterfall over the capture.
//
// fftSize sets the frequency resolution and hop the time resolution. They trade
// against each other and both matter for LoRa: a chirp sweeps the whole
// bandwidth in one symbol, so too coarse a hop smears it into a solid block and
// too fine an FFT smears it across every bin. For SF7 at 125 kHz, an fftSize
// around 256 with 50% overlap shows the sweep as a diagonal, which is what makes
// a LoRa signal recognisable by eye.
func (c Capture) Spectrogram(fftSize, hop int) Spectrogram {
	if fftSize <= 1 || fftSize&(fftSize-1) != 0 {
		panic("sdr: fftSize must be a power of two")
	}
	if hop <= 0 {
		hop = fftSize / 2
	}

	win := hann(fftSize)
	// Coherent gain, sum(w): a windowed tone of amplitude A produces a bin of
	// magnitude A*sum(w), so this is what the power normalises against. It is
	// what makes the display read in absolute terms — without it every reading
	// is wrong by the same hidden amount and nothing on screen looks unusual.
	var norm float64
	for _, w := range win {
		norm += w
	}

	var frames [][]float64
	buf := make([]complex128, fftSize)
	for start := 0; start+fftSize <= len(c.IQ); start += hop {
		for i := 0; i < fftSize; i++ {
			buf[i] = c.IQ[start+i] * complex(win[i], 0)
		}
		dsp.FFT(buf)

		row := make([]float64, fftSize)
		for i := 0; i < fftSize; i++ {
			// fftshift: bins above Nyquist are negative frequencies and belong
			// on the left. Skipping this is the classic waterfall bug where a
			// signal appears mirrored at the wrong side of centre.
			src := (i + fftSize/2) % fftSize
			re, im := real(buf[src]), imag(buf[src])
			// Coherent gain normalisation: a tone at amplitude A produces a bin
			// of A*N*mean(w), so the power divides by (N*mean(w))^2. Dividing by
			// N instead of N^2 is a 24 dB error at fftSize 256 and looks like a
			// perfectly reasonable waterfall.
			p := (re*re + im*im) / (norm * norm)
			row[i] = 10 * math.Log10(math.Max(p, 1e-300))
		}
		frames = append(frames, row)
	}

	return Spectrogram{
		Frames:       frames,
		BinHz:        c.Observer.SampleRateHz / float64(fftSize),
		FrameSeconds: float64(hop) / c.Observer.SampleRateHz,
		CentreHz:     c.Observer.CentreHz,
		NoiseFloorDB: 10 * math.Log10(math.Max(c.NoisePowerLinear, 1e-300)),
	}
}

// FrequencyOf is the absolute frequency at a bin index.
func (s Spectrogram) FrequencyOf(bin int) float64 {
	return s.CentreHz + (float64(bin)-float64(len(s.Frames[0]))/2)*s.BinHz
}

// PeakBin is the strongest bin in a frame, and its power. Enough to trace a
// chirp across the waterfall without a decoder — which is the whole point of an
// observer that decides nothing.
func (s Spectrogram) PeakBin(frame int) (bin int, powerDB float64) {
	row := s.Frames[frame]
	powerDB = math.Inf(-1)
	for i, v := range row {
		if v > powerDB {
			bin, powerDB = i, v
		}
	}
	return bin, powerDB
}

// hann is the window. Hann rather than rectangular because a rectangular window
// leaks a strong signal across the whole display, and the first thing anyone
// does with a waterfall is look for a weak signal next to a strong one.
func hann(n int) []float64 {
	w := make([]float64, n)
	for i := range w {
		w[i] = 0.5 - 0.5*math.Cos(2*math.Pi*float64(i)/float64(n))
	}
	return w
}
