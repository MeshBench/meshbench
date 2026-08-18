package sdr

import (
	"math"
	"testing"
)

// ramp counts upward forever: any dropped or repeated native sample shows as
// a kink in the interpolated output.
type ramp struct{ at, rate float64 }

func (r *ramp) SampleRateHz() float64 { return r.rate }
func (r *ramp) NextSamples(n int) []complex128 {
	out := make([]complex128, n)
	for i := range out {
		out[i] = complex(r.at, 0)
		r.at++
	}
	return out
}

// Upsampled 4x, a ramp must stay a ramp with slope native/out - across chunk
// boundaries, which is where a stateless resampler tears.
func TestResamplerKeepsARampStraightAcrossChunks(t *testing.T) {
	src := &ramp{rate: 62500}
	rs := newResampler(src)
	rs.setRate(250000)
	var got []complex128
	for c := 0; c < 4; c++ {
		chunk := make([]complex128, 1000)
		rs.next(chunk)
		got = append(got, chunk...)
	}
	// The first outputs lean on history that does not exist yet; judge the
	// stream once the kernel is fully fed.
	for i := resampTaps*4 + 1; i < len(got); i++ {
		d := real(got[i]) - real(got[i-1])
		if math.Abs(d-0.25) > 0.02 {
			t.Fatalf("step %d is %f, want 0.25: the stream tore at a chunk boundary", i, d)
		}
	}
}

// With no client request the output rate is the native rate, and samples
// pass through unchanged - the pre-resampler behaviour, kept.
func TestResamplerNativeIsIdentity(t *testing.T) {
	src := &ramp{rate: 62500}
	rs := newResampler(src)
	rs.setRate(0)
	out := make([]complex128, 100)
	rs.next(out)
	for i := resampTaps + 1; i < len(out); i++ {
		if d := real(out[i]) - real(out[i-1]); math.Abs(d-1) > 1e-6 {
			t.Fatalf("native passthrough stepped %f at %d, want 1", d, i)
		}
	}
}

// The reason sinc replaced linear: a tone inside the native band must stay
// a tone, and the client-rate spectrum beyond the native band must hold
// almost nothing. Linear interpolation left images ~20 dB down and a strong
// burst painted the client's whole span.
func TestResamplerSuppressesImages(t *testing.T) {
	const native, out = 62500.0, 250000.0
	src := &tone{rate: native, hz: 10000}
	rs := newResampler(src)
	rs.setRate(out)
	n := 8192
	buf := make([]complex128, n)
	rs.next(buf)
	buf = buf[512:] // past the kernel's warm-up
	// Goertzel power at the tone and at its first image (native + tone
	// folded: 62500 - 10000 above... the image of +10 kHz sits at
	// +10k + 62.5k = 72.5 kHz in the 250 kHz output).
	power := func(hz float64) float64 {
		var acc complex128
		for i, v := range buf {
			ph := -2 * math.Pi * hz * float64(i) / out
			acc += v * complex(math.Cos(ph), math.Sin(ph))
		}
		return real(acc)*real(acc) + imag(acc)*imag(acc)
	}
	sig := power(10000)
	img := power(72500)
	if sig <= 0 {
		t.Fatal("the tone vanished")
	}
	if ratio := 10 * math.Log10(img/sig); ratio > -50 {
		t.Fatalf("first image only %.1f dB down; the band leaks across the client span", ratio)
	}
}

type tone struct{ rate, hz, at float64 }

func (s *tone) SampleRateHz() float64 { return s.rate }
func (s *tone) NextSamples(n int) []complex128 {
	out := make([]complex128, n)
	for i := range out {
		ph := 2 * math.Pi * s.hz * s.at / s.rate
		out[i] = complex(math.Cos(ph), math.Sin(ph))
		s.at++
	}
	return out
}
