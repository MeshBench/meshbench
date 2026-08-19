package main

import (
	"fmt"
	"math"
	"os"

	"github.com/MeshBench/meshbench/internal/rf/channel"
	"github.com/MeshBench/meshbench/internal/rf/dsp"
	"github.com/MeshBench/meshbench/internal/world/sdr"
	m "github.com/MeshBench/meshbench/tools/internal/mockup"
)

// sdrCapture is the scene both the waterfall and the audio are made from — the
// same nodes, budgets and seed, differing only in how long it runs for. The
// figure wants a few symbols so the structure is readable; the sound file wants
// a second of it so there is something to hear.
func sdrCapture(symbols int) sdr.Capture {
	const sf = 10
	n := dsp.SamplesPerSymbol(sf)
	mod := dsp.Modulator{SF: sf}

	// Real link budgets, not convenient ones: -100 dBm and -112 dBm against a
	// -117 dBm floor, so one signal is a comfortable 17 dB up and the other is
	// 5 dB up and genuinely marginal. Drawing this with a strong signal makes a
	// picture that could never occur.
	obs := sdr.Observer{Name: "obs-1", CentreHz: 869.525e6, SampleRateHz: 125_000, NoiseFigureDB: 6}
	up := func(k int) []int { return make([]int, k) } // symbol 0 is a plain upchirp
	return obs.Capture([]channel.Transmission{
		{Node: "GB7XYZ", Samples: mod.Modulate(up(symbols)), GainDB: -100, DelaySamples: 0.4},
		{Node: "node-04", Samples: mod.Modulate(up(symbols / 2)), GainDB: -112,
			StartSample: n * 2, DelaySamples: 1.7},
	}, 4417, 0, n*(symbols+1))
}

// writeSDRAudio renders the same capture to a sound file: tuned near the middle
// of the band with a 6 kHz filter, which is roughly what an operator would do
// on hearing something and wanting to know what it was.
//
// Through that filter a LoRa chirp arrives as a rising whistle repeating at the
// symbol rate. That is what it genuinely sounds like on a real receiver, and
// the reason listening is worth having rather than only plotting.
func writeSDRAudio(path string) error {
	c := sdrCapture(120) // about a second at SF10
	t := sdr.Tuning{OffsetHz: 0, AudioRateHz: 6000}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	if err := sdr.WriteWAV(f, c.Listen(t), int(c.AudioRate(t))); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

// realSDRObserver draws what an SDR observer node actually captures: a true
// time-frequency waterfall of the summed field at its antenna, with no
// demodulation anywhere in the path.
//
// Two transmissions overlap. Both chirps are visible as diagonals because the
// observer decides nothing — unlike the receiver, which has to pick one.
func realSDRObserver() *m.Canvas {
	c := m.New(1200, 620)
	c.Fill(0, 0, 1200, 40, m.BgSurface)
	c.Text(20, 26, "SDR OBSERVER — summed field at the antenna, nothing demodulated", m.TextHi, m.SansBold, 12)

	cap := sdrCapture(4)
	obs := cap.Observer
	spec := cap.Spectrogram(128, 32)
	if len(spec.Frames) == 0 {
		return c
	}
	bins := len(spec.Frames[0])

	// Colour map anchored to the engine's own noise floor, not stretched to the
	// data. A display that autoscales makes every capture look equally busy.
	perBin := spec.NoiseFloorDB - 10*math.Log10(float64(bins)) + 1.76
	lo, hi := perBin-2, perBin+22

	x0, y0, w, h := 60, 76, 1080, 380
	c.RoundRect(x0-8, y0-8, w+16, h+16, 8, m.BgInset, m.Border)
	for py := 0; py < h; py++ {
		row := spec.Frames[py*len(spec.Frames)/h]
		for px := 0; px < w; px++ {
			v := row[px*bins/w]
			t := math.Max(0, math.Min(1, (v-lo)/(hi-lo)))
			c.Fill(x0+px, y0+py, 1, 1, waterfallColour(t))
		}
	}

	// Axes, in real units — a waterfall without them is wallpaper.
	half := float64(bins) / 2 * spec.BinHz / 1000
	for i, lbl := range []string{
		fmt.Sprintf("%.0f kHz", -half), fmt.Sprintf("%.0f", -half/2), "centre",
		fmt.Sprintf("+%.0f", half/2), fmt.Sprintf("+%.0f kHz", half),
	} {
		c.Text(x0+i*w/4-16, y0+h+22, lbl, m.TextLo, m.Sans, 9.5)
	}
	total := float64(len(spec.Frames)) * spec.FrameSeconds * 1000
	c.Text(20, y0+8, "0 ms", m.TextLo, m.Sans, 9)
	c.Text(20, y0+h, fmt.Sprintf("%.0f ms", total), m.TextLo, m.Sans, 9)

	c.RoundRect(20, 500, 1160, 100, 10, m.BgSurface, m.Border)
	c.Text(40, 528, "WHAT THIS IS", m.TextHi, m.SansBold, 11)
	c.Text(40, 552, fmt.Sprintf(
		"%s   |   noise floor %.1f dB across the band, %.1f dB per bin   |   %d x %d FFT, %.2f ms per row",
		obs, spec.NoiseFloorDB, perBin, len(spec.Frames), bins, spec.FrameSeconds*1000),
		m.TextLo, m.Sans, 10)
	c.Text(40, 574, "Two overlapping SF10 transmissions at -100 and -112 dBm. Both chirps are present because "+
		"an observer decides nothing —", m.TextLo, m.Sans, 10)
	c.Text(40, 590, "a receiver at this position would have to pick one, and the capture effect is what decides which.",
		m.TextLo, m.Sans, 10)
	return c
}

// waterfallColour is the usual dark-blue-through-yellow ramp. Perceptually
// ordered so a stronger signal always looks stronger — a rainbow map does not
// have that property and invents features at its hue boundaries.
func waterfallColour(t float64) m.NRGBA {
	stops := []struct {
		at      float64
		r, g, b float64
	}{
		{0.00, 8, 12, 30}, {0.30, 20, 60, 120}, {0.55, 30, 150, 160},
		{0.78, 200, 190, 70}, {1.00, 255, 245, 200},
	}
	for i := 1; i < len(stops); i++ {
		if t <= stops[i].at {
			a, b := stops[i-1], stops[i]
			f := (t - a.at) / (b.at - a.at)
			return m.NRGBA{
				R: uint8(a.r + (b.r-a.r)*f),
				G: uint8(a.g + (b.g-a.g)*f),
				B: uint8(a.b + (b.b-a.b)*f),
				A: 0xff,
			}
		}
	}
	s := stops[len(stops)-1]
	return m.NRGBA{R: uint8(s.r), G: uint8(s.g), B: uint8(s.b), A: 0xff}
}
