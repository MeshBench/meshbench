package sdr

import (
	"math"

	"github.com/MeshBench/meshbench/internal/dsp"
)

// Tuning is where in the captured spectrum to listen, and how wide.
type Tuning struct {
	// OffsetHz from the observer's centre frequency. Negative is below.
	OffsetHz float64

	// AudioRateHz is the output sample rate; 48000 unless there is a reason.
	// It also sets the receive bandwidth, because the filter that prevents
	// aliasing *is* the receiver's filter — same as on real hardware.
	AudioRateHz float64
}

// Listen turns the captured spectrum into something audible.
//
// This is a real receiver, not a sonification. It tunes to an offset, filters
// to the audio bandwidth and resamples — so what comes out is what an operator
// with an SDR and a narrow filter would actually hear at that frequency.
//
// A LoRa transmission sweeps its whole bandwidth every symbol, so through a few
// kHz of filter it arrives as a rising whistle repeating at the symbol rate:
// the sound that makes LoRa identifiable on a waterfall-plus-speakers setup, and
// the reason listening is worth having at all rather than just plotting.
//
// The output is real audio, taken as the real part of the tuned baseband. That
// is a double-sideband listen; it folds the spectrum about the tuning point, in
// exactly the way an AM-style detector does.
func (c Capture) Listen(t Tuning) []float64 {
	if t.AudioRateHz <= 0 {
		t.AudioRateHz = 48000
	}
	if len(c.IQ) == 0 {
		return nil
	}

	// Frequency-domain tune, filter and decimate in one step: shift the spectrum
	// so the tuning point is at DC, keep only the bins inside the audio
	// bandwidth, and transform back. Discarding bins *is* the anti-alias filter,
	// and it is a brick wall, which no real receiver has — noted in
	// docs/shortcomings.md rather than papered over with a synthetic roll-off.
	n := nextPow2(len(c.IQ))
	spec := make([]complex128, n)
	copy(spec, c.IQ)
	dsp.FFT(spec)

	binHz := c.Observer.SampleRateHz / float64(n)
	shift := int(math.Round(t.OffsetHz / binHz))

	keep := nextPow2(int(t.AudioRateHz / binHz))
	if keep < 2 {
		keep = 2
	}
	if keep > n {
		keep = n
	}

	out := make([]complex128, keep)
	for i := range out {
		// Bins either side of DC, taken from around the tuning point.
		var src int
		if i < keep/2 {
			src = (i + shift + n) % n // positive frequencies
		} else {
			src = (i - keep + shift + n) % n // negative frequencies, wrapped
		}
		out[i] = spec[src]
	}
	// Rotate so DC sits at index 0 for the inverse transform, matching the
	// layout FFT/IFFT expect rather than the display order used by Spectrogram.
	dsp.IFFT(out)

	// Trim the zero padding back off. The transform runs on a power-of-two
	// length, so a capture that is not one comes back stretched — 41 ms of
	// signal arriving as 65 ms of audio, which sounds entirely plausible and is
	// simply the wrong speed.
	keptLen := int(math.Round(float64(len(c.IQ)) / float64(n) * float64(keep)))
	if keptLen > 0 && keptLen < len(out) {
		out = out[:keptLen]
	}

	audio := make([]float64, len(out))
	peak := 0.0
	for i, v := range out {
		audio[i] = real(v)
		if a := math.Abs(audio[i]); a > peak {
			peak = a
		}
	}
	// Normalise, because the linear scale that keeps the physics honest puts a
	// received signal around 1e-10 — inaudible, and not a meaningful number to
	// write into a sound file. Amplitude relative to the noise floor is
	// preserved; only the overall level changes.
	if peak > 0 {
		for i := range audio {
			audio[i] /= peak
		}
	}
	return audio
}

// AudioRate is the real output rate of Listen, which differs from the requested
// rate because the decimation factor is a power of two.
//
// It is returned rather than silently applied: writing a WAV header with the
// requested rate when the samples are at another is how a recording ends up
// playing back at the wrong pitch, and a chirp at the wrong pitch still sounds
// entirely plausible.
func (c Capture) AudioRate(t Tuning) float64 {
	if t.AudioRateHz <= 0 {
		t.AudioRateHz = 48000
	}
	n := nextPow2(len(c.IQ))
	if n == 0 {
		return t.AudioRateHz
	}
	binHz := c.Observer.SampleRateHz / float64(n)
	keep := nextPow2(int(t.AudioRateHz / binHz))
	if keep < 2 {
		keep = 2
	}
	if keep > n {
		keep = n
	}
	return float64(keep) * binHz
}

func nextPow2(n int) int {
	if n <= 1 {
		return 1
	}
	p := 1
	for p < n {
		p <<= 1
	}
	return p
}
