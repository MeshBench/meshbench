// The waterfall: what a receiver at one node actually heard.
//
// Captured through internal/sdr from the engine's own in-flight
// transmissions, which is the same signal the demodulator was handed. A
// waterfall rendered from a separate model would be a picture that can
// disagree with the decode beside it, and the moment it did, neither would be
// trustworthy.
package session

import (
	"context"
	"image"
	"image/color"
	"math"
	"sort"

	"github.com/MeshBench/meshbench/internal/gui/state"
	"github.com/MeshBench/meshbench/internal/sdr"
)

const (
	waterfallFFT      = 256
	waterfallWindowS  = 0.2
	waterfallNoiseFig = 6
)

// capture observes the channel at one node and returns the spectrogram as an
// image, ready for the renderer.
func (s *Sim) capture(_ context.Context, at int) (*state.Coverage, string) {
	if s.eng == nil || at < 0 || at >= len(s.nodes) {
		return nil, "no simulation running"
	}
	n := s.nodes[at]
	obs := sdr.Observer{
		Name: n.Name, CentreHz: n.Radio.CentreHz, NoiseFigureDB: waterfallNoiseFig,
		SampleRateHz: n.Radio.BandwidthHz,
	}
	if obs.CentreHz <= 0 {
		obs.CentreHz, obs.SampleRateHz = 869.618e6, 250e3
	}
	txs := s.eng.InFlightTransmissions(at)
	if len(txs) == 0 {
		// Said plainly, because an empty waterfall and a broken one look
		// identical and only one of them is worth investigating.
		return nil, "nothing on the air at " + n.Name +
			" at this instant - play the run and capture during a flood"
	}
	capt := obs.Capture(txs, 9001, 0, int(obs.SampleRateHz*waterfallWindowS))
	spec := capt.Spectrogram(waterfallFFT, waterfallFFT/2)
	if len(spec.Frames) == 0 {
		return nil, "the capture window held no samples"
	}
	return &state.Coverage{Node: n.Name, Image: spectrogramImage(spec)}, ""
}

// spectrogramImage colours a spectrogram against its own noise floor.
//
// Against the floor rather than against the loudest thing in view: a display
// normalised to its own peak makes a quiet channel look as busy as a loud one,
// which is the opposite of what a waterfall is read for.
func spectrogramImage(spec sdr.Spectrogram) *image.RGBA {
	h := len(spec.Frames)
	if h == 0 {
		return nil
	}
	w := len(spec.Frames[0])
	img := image.NewRGBA(image.Rect(0, 0, w, h))

	// The floor is the median bin: most of a channel is noise most of the
	// time, and the median is not moved by the signal the way a mean is.
	var all []float64
	for _, f := range spec.Frames {
		all = append(all, f...)
	}
	floor := median(all)

	for y, frame := range spec.Frames {
		for x, v := range frame {
			img.SetRGBA(x, y, waterfallColour(v-floor))
		}
	}
	return img
}

// waterfallColour maps decibels above the noise floor to the usual cold-to-hot
// ramp, topping out at 40 dB where more is not a distinction anybody reads.
func waterfallColour(aboveDB float64) color.RGBA {
	t := math.Max(0, math.Min(1, aboveDB/40))
	switch {
	case t < 0.25:
		u := t / 0.25
		return color.RGBA{R: 0, G: uint8(40 * u), B: uint8(40 + 120*u), A: 255}
	case t < 0.5:
		u := (t - 0.25) / 0.25
		return color.RGBA{R: 0, G: uint8(40 + 150*u), B: uint8(160 - 40*u), A: 255}
	case t < 0.75:
		u := (t - 0.5) / 0.25
		return color.RGBA{R: uint8(220 * u), G: 190, B: uint8(120 - 120*u), A: 255}
	}
	u := (t - 0.75) / 0.25
	return color.RGBA{R: 255, G: uint8(190 - 60*u), B: uint8(60 * u), A: 255}
}

func median(v []float64) float64 {
	if len(v) == 0 {
		return 0
	}
	c := append([]float64(nil), v...)
	sort.Float64s(c)
	return c[len(c)/2]
}
