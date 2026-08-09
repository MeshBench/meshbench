package sdr_test

import (
	"bytes"
	"math"
	"testing"

	"github.com/A13xB0/meshcoresim/internal/dsp"
	"github.com/A13xB0/meshcoresim/internal/rf"
	"github.com/A13xB0/meshcoresim/internal/sdr"
)

func observer() sdr.Observer {
	return sdr.Observer{Name: "obs-1", CentreHz: 869.525e6, SampleRateHz: 125_000, NoiseFigureDB: 6}
}

// A LoRa upchirp sweeps the whole bandwidth once per symbol. On a waterfall
// that is a diagonal line, and the diagonal is the thing that makes a LoRa
// signal recognisable — so the peak bin must climb steadily across frames.
//
// This tests the observer against the actual waveform rather than against a
// stored picture: if the modulator changes, this should notice.
func TestChirpSweepsAcrossTheWaterfall(t *testing.T) {
	const sf = 7
	iq := dsp.Modulator{SF: sf}.Modulate([]int{0, 0, 0, 0})

	c := observer().Capture([]rf.Transmission{{
		Node: "tx", Samples: iq, GainDB: 0,
	}}, 4417, 0, len(iq))

	s := c.Spectrogram(64, 16)
	if len(s.Frames) < 8 {
		t.Fatalf("only %d frames; too few to see a sweep", len(s.Frames))
	}

	// Peak frequency should rise, wrapping once per symbol. Count how often it
	// steps up versus down: a sweep is mostly rising with one wrap per symbol.
	up, down := 0, 0
	prev, _ := s.PeakBin(0)
	for i := 1; i < len(s.Frames); i++ {
		b, _ := s.PeakBin(i)
		switch {
		case b > prev:
			up++
		case b < prev:
			down++
		}
		prev = b
	}
	if up <= down {
		t.Errorf("peak rose %d times and fell %d — that is not a sweep", up, down)
	}
}

// The waterfall must read in absolute terms, not relative to whatever is on
// screen. A unit-amplitude carrier with no noise should land at about 0 dB.
func TestSpectrogramIsCalibrated(t *testing.T) {
	const n = 4096
	iq := make([]complex128, n)
	// A tone a quarter of the way up the band, so it is not at DC where a
	// mistake in the fftshift would be invisible.
	for i := range iq {
		iq[i] = complex(math.Cos(2*math.Pi*0.25*float64(i)), math.Sin(2*math.Pi*0.25*float64(i)))
	}
	c := sdr.Capture{Observer: observer(), IQ: iq}

	s := c.Spectrogram(256, 128)
	_, peakDB := s.PeakBin(len(s.Frames) / 2)
	if math.Abs(peakDB) > 1.0 {
		t.Errorf("unit carrier read %.2f dB, want about 0 dB — window gain is not corrected", peakDB)
	}
}

// A tone above centre must appear above centre. Getting this wrong mirrors the
// display, which is the single most common waterfall bug and looks entirely
// plausible until you compare it with a real receiver.
func TestPositiveFrequenciesAppearAboveCentre(t *testing.T) {
	const n = 4096
	iq := make([]complex128, n)
	for i := range iq {
		iq[i] = complex(math.Cos(2*math.Pi*0.25*float64(i)), math.Sin(2*math.Pi*0.25*float64(i)))
	}
	c := sdr.Capture{Observer: observer(), IQ: iq}
	s := c.Spectrogram(256, 128)

	bin, _ := s.PeakBin(0)
	if got := s.FrequencyOf(bin); got <= s.CentreHz {
		t.Errorf("a +31.25 kHz tone appeared at %.0f Hz, at or below centre %.0f Hz", got, s.CentreHz)
	}
}

// Noise must show at the floor the engine actually used, otherwise every
// absolute reading off the display is wrong by the same hidden amount.
func TestNoiseFloorMatchesTheEngine(t *testing.T) {
	o := observer()
	c := o.Capture(nil, 4417, 0, 8192)
	s := c.Spectrogram(256, 128)

	// Averaged as power, not as decibels. Bin powers are exponentially
	// distributed, and the mean of their logarithms sits about 2.5 dB below the
	// logarithm of their mean — enough to fail a correct implementation.
	var sum float64
	var count int
	for _, row := range s.Frames {
		for _, v := range row {
			sum += math.Pow(10, v/10)
			count++
		}
	}
	mean := 10 * math.Log10(sum/float64(count))

	// A bin holds 1/fftSize of the band's noise, raised by Hann's noise
	// bandwidth of 1.5 bins — the standard consequence of calibrating a display
	// for tones rather than for noise.
	const hannNoiseBandwidth = 1.5
	wantPerBin := s.NoiseFloorDB - 10*math.Log10(256) + 10*math.Log10(hannNoiseBandwidth)
	if math.Abs(mean-wantPerBin) > 1.0 {
		t.Errorf("mean bin power %.1f dB, want about %.1f dB", mean, wantPerBin)
	}
}

// Listening must produce audio at the rate it claims. A chirp played back at
// the wrong pitch still sounds entirely plausible, which is what makes this
// worth asserting rather than eyeballing.
func TestListenReportsItsRealRate(t *testing.T) {
	const sf = 9
	iq := dsp.Modulator{SF: sf}.Modulate([]int{0, 1, 2, 3, 4, 5, 6, 7})
	c := observer().Capture([]rf.Transmission{{Node: "tx", Samples: iq, GainDB: 0}}, 4417, 0, len(iq))

	tune := sdr.Tuning{OffsetHz: 0, AudioRateHz: 48000}
	audio := c.Listen(tune)
	rate := c.AudioRate(tune)

	if len(audio) == 0 {
		t.Fatal("no audio")
	}
	// Duration must survive the resample: the same stretch of time, fewer
	// samples.
	captured := float64(len(c.IQ)) / c.Observer.SampleRateHz
	played := float64(len(audio)) / rate
	if math.Abs(captured-played)/captured > 0.02 {
		t.Errorf("captured %.4f s but audio runs %.4f s", captured, played)
	}
	if rate > c.Observer.SampleRateHz {
		t.Errorf("audio rate %.0f exceeds the capture rate %.0f", rate, c.Observer.SampleRateHz)
	}
}

// Tuning away from a signal must make it quieter. Otherwise the tuner is not
// tuning and the listener hears the whole band wherever it is pointed.
func TestTuningAwayFromASignalAttenuatesIt(t *testing.T) {
	const n = 32768
	iq := make([]complex128, n)
	for i := range iq {
		iq[i] = complex(math.Cos(2*math.Pi*0.2*float64(i)), math.Sin(2*math.Pi*0.2*float64(i)))
	}
	c := sdr.Capture{Observer: observer(), IQ: iq}

	onTone := rms(c.Listen(sdr.Tuning{OffsetHz: 0.2 * c.Observer.SampleRateHz, AudioRateHz: 4000}))
	offTone := rms(c.Listen(sdr.Tuning{OffsetHz: -0.3 * c.Observer.SampleRateHz, AudioRateHz: 4000}))
	if offTone >= onTone/10 {
		t.Errorf("off-tune RMS %.4g is not meaningfully below on-tune %.4g", offTone, onTone)
	}
}

func TestWAVRoundTripsAHeader(t *testing.T) {
	var buf bytes.Buffer
	if err := sdr.WriteWAV(&buf, []float64{0, 0.5, -0.5, 1, -1}, 48000); err != nil {
		t.Fatal(err)
	}
	b := buf.Bytes()
	if string(b[0:4]) != "RIFF" || string(b[8:12]) != "WAVE" {
		t.Fatalf("not a RIFF/WAVE file: % x", b[:12])
	}
	if want := 44 + 5*2; len(b) != want {
		t.Errorf("file is %d bytes, want %d", len(b), want)
	}
	// Clipping, not wrapping: +1.0 must be full scale positive, never negative.
	last := int16(b[len(b)-2]) | int16(b[len(b)-1])<<8
	if last != -32767 {
		t.Errorf("-1.0 encoded as %d, want -32767", last)
	}
}

func rms(x []float64) float64 {
	if len(x) == 0 {
		return 0
	}
	var s float64
	for _, v := range x {
		s += v * v
	}
	return math.Sqrt(s / float64(len(x)))
}

// A capture whose length is not a power of two must not come back stretched.
// The transform pads to a power of two, so 41 ms of signal returned as 65 ms of
// audio — the wrong speed, and it sounds entirely plausible.
func TestListenDoesNotStretchAnUnpaddedCapture(t *testing.T) {
	const sf = 10
	n := dsp.SamplesPerSymbol(sf)
	iq := dsp.Modulator{SF: sf}.Modulate([]int{0, 0, 0, 0, 0})
	if len(iq)&(len(iq)-1) == 0 {
		t.Fatalf("capture of %d samples is a power of two; this test needs one that is not", len(iq))
	}
	c := observer().Capture([]rf.Transmission{{Node: "tx", Samples: iq, GainDB: -100}}, 4417, 0, n*5)

	tune := sdr.Tuning{OffsetHz: 0, AudioRateHz: 6000}
	captured := float64(len(c.IQ)) / c.Observer.SampleRateHz
	played := float64(len(c.Listen(tune))) / c.AudioRate(tune)
	if math.Abs(captured-played)/captured > 0.02 {
		t.Errorf("captured %.4f s but audio runs %.4f s — padding was not trimmed", captured, played)
	}
}
