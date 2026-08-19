// Resampling the observer's stream to the client's chosen rate.
//
// A real rtl_tcp changes the dongle's rate when the client asks; this server
// synthesises at the observer's bandwidth - one sample per hertz - and no
// SDR client offers 62,500 S/s in its rate menu. So the server follows the
// client instead. Interpolation is windowed-sinc, not linear: linear leaves
// images of the band repeating across the client's whole span, and a strong
// burst painted the full spectrum width instead of its own bandwidth.
// Band-limited interpolation keeps the transmission exactly as wide as it
// is, and the ground beyond the observer's bandwidth honestly silent.
package sdr

import "math"

// resampTaps is the kernel's reach each side of the point. Twelve taps
// total puts the images ~60 dB down, below the 8-bit format's own floor.
const resampTaps = 6

// resampler pulls native samples and produces client-rate samples, keeping
// its position and tap history across chunks so the stream stays continuous.
type resampler struct {
	src  SampleSource
	step float64 // native samples per output sample
	// pos is the next output's position in native-sample units, relative to
	// hist[0].
	pos  float64
	hist []complex128
}

func newResampler(src SampleSource) *resampler {
	return &resampler{src: src}
}

// setRate fixes the output rate. Changing rates keeps the stream position:
// a client flipping its menu mid-run gets a new pace, not a new stream.
func (r *resampler) setRate(outHz float64) {
	native := r.src.SampleRateHz()
	if outHz <= 0 {
		outHz = native
	}
	r.step = native / outHz
}

// kern is the windowed sinc: exact one at zero, zero at the other
// integers, Hann-tapered to the kernel's edge.
func kern(x float64) float64 {
	if x < 0 {
		x = -x
	}
	if x >= resampTaps {
		return 0
	}
	if x < 1e-9 {
		return 1
	}
	px := math.Pi * x
	return math.Sin(px) / px * (0.5 + 0.5*math.Cos(px/resampTaps))
}

// next fills out with client-rate samples.
func (r *resampler) next(out []complex128) {
	if len(out) == 0 {
		return
	}
	// The highest native index the last output's right taps reach; fetch
	// exactly the shortfall so the source's own pacing stays in charge.
	lastPos := r.pos + r.step*float64(len(out)-1)
	needTop := int(lastPos) + resampTaps + 1
	if short := needTop + 1 - len(r.hist); short > 0 {
		r.hist = append(r.hist, r.src.NextSamples(short)...)
	}
	for i := range out {
		k0 := int(r.pos)
		var acc complex128
		var wsum float64
		for k := k0 - resampTaps + 1; k <= k0+resampTaps; k++ {
			if k < 0 || k >= len(r.hist) {
				continue
			}
			w := kern(r.pos - float64(k))
			acc += r.hist[k] * complex(w, 0)
			wsum += w
		}
		if wsum != 0 {
			// Normalised so the kernel's finite window cannot tilt the
			// level - a ramp comes back a ramp, DC comes back DC.
			acc /= complex(wsum, 0)
		}
		out[i] = acc
		r.pos += r.step
	}
	// Drop history the next chunk's left taps can no longer reach.
	if drop := int(r.pos) - resampTaps; drop > 0 {
		if drop > len(r.hist) {
			drop = len(r.hist)
		}
		r.hist = append(r.hist[:0], r.hist[drop:]...)
		r.pos -= float64(drop)
	}
}
