package ui

import (
	"fmt"
	"image"
	"image/color"
	"math"

	"github.com/AllenDang/cimgui-go/imgui"

	"github.com/MeshBench/meshbench/internal/engine"
	"github.com/MeshBench/meshbench/internal/sdr"
)

// waterfallState is the spectrum view: what was captured, and what was clicked.
type waterfallState struct {
	node string
	tex  *imgui.TextureRef
	spec sdr.Spectrogram
	err  string

	// selected is the burst under the last click, as a frame index.
	selected int
	// symbols is the dechirped view of that burst.
	symbols []float64
	peak    int
	second  int
	ratioDB float64
}

// waterfallFFT is the transform size. 256 bins across the capture bandwidth is
// enough to separate two LoRa chirps visually, and small enough to compute for
// a whole window without a GPU.
const waterfallFFT = 256

// drawWaterfall is the spectrum at a receiver, and the symbol view under it.
//
// This is how you *see* a collision instead of inferring one. A packet-level
// simulator can only say "collision"; two overlapping chirp ramps in a
// waterfall show which one captured, and by how many dB — which is the
// difference between a model and an instrument.
func (a *App) drawWaterfall() {
	from, _ := a.Link()
	if from < 0 {
		textDim("select a node to listen from")
		return
	}
	name := a.Nodes[from].Name

	imgui.Text("listening at " + name)
	imgui.SameLine()
	if imgui.Button("capture now") {
		a.captureWaterfall(from)
	}
	imgui.SameLine()
	textDim("a window of baseband at this receiver, from the same channel the nodes use")

	if a.wf.err != "" {
		imgui.PushStyleColorVec4(imgui.ColText, imgui.NewVec4(0.95, 0.72, 0.25, 1))
		textWrap(a.wf.err)
		imgui.PopStyleColor()
		return
	}
	if a.wf.tex == nil {
		textDim("nothing captured yet - place an SDR observer and run; it records what it hears")
		return
	}

	// The waterfall itself: time down, frequency across.
	origin := imgui.CursorScreenPos()
	avail := imgui.ContentRegionAvail()
	h := avail.Y * 0.55
	if h < 80 {
		h = 80
	}
	dl := imgui.WindowDrawList()
	dl.AddImage(*a.wf.tex, origin, imgui.NewVec2(origin.X+avail.X, origin.Y+h))
	imgui.InvisibleButtonV("##wf", imgui.NewVec2(avail.X, h), 0)
	if imgui.IsItemHovered() && imgui.IsMouseClickedBool(imgui.MouseButtonLeft) {
		mouse := imgui.MousePos()
		frame := int((mouse.Y - origin.Y) / h * float32(len(a.wf.spec.Frames)))
		a.selectBurst(frame)
	}
	textDim(fmt.Sprintf("%.0f kHz across, %.1f ms deep, floor %.0f dB - click a burst",
		float64(len(a.wf.spec.Frames[0]))*a.wf.spec.BinHz/1000,
		float64(len(a.wf.spec.Frames))*a.wf.spec.FrameSeconds*1000,
		a.wf.spec.NoiseFloorDB))

	imgui.SeparatorText("Symbol view")
	if a.wf.symbols == nil {
		textDim("click a burst above to dechirp it")
		return
	}
	a.drawSymbolView()
}

// captureWaterfall observes the channel at a node and builds the spectrogram.
func (a *App) captureWaterfall(i int) {
	a.wf = waterfallState{node: a.Nodes[i].Name, selected: -1}
	if a.eng == nil {
		a.wf.err = "no simulation running"
		return
	}
	n := a.Nodes[i]
	obs := sdr.Observer{
		Name: n.Name, CentreHz: n.Radio.CentreHz, NoiseFigureDB: 6,
		SampleRateHz: n.Radio.BandwidthHz,
	}
	if obs.CentreHz <= 0 {
		obs.CentreHz, obs.SampleRateHz = a.freqMHz*1e6, 250e3
	}

	// The transmissions in flight right now, from the engine's own channel —
	// so what the waterfall shows is what the demodulator was handed, not a
	// separate rendering that could disagree with it.
	txs := a.eng.InFlightTransmissions(i)
	if len(txs) == 0 {
		a.wf.err = "nothing on the air at this instant; run the simulation and capture during a flood"
		return
	}
	window := int(obs.SampleRateHz * 0.2) // 200 ms
	capt := obs.Capture(txs, a.runSeed(), 0, window)
	a.wf.spec = capt.Spectrogram(waterfallFFT, waterfallFFT/2)
	if len(a.wf.spec.Frames) == 0 {
		a.wf.err = "the capture window held no samples"
		return
	}
	img := spectrogramImage(a.wf.spec)
	tex := a.backend.CreateTextureRgba(img, img.Bounds().Dx(), img.Bounds().Dy())
	a.wf.tex = &tex
}

// spectrogramImage colours a spectrogram against its own noise floor.
//
// Against the floor, not against whatever happened to be in view: a display
// that autoscales makes an empty band look busy and a busy one look calm, and
// both are lies about the same colour.
func spectrogramImage(s sdr.Spectrogram) *image.RGBA {
	h, w := len(s.Frames), len(s.Frames[0])
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	const rangeDB = 45
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			t := (s.Frames[y][x] - s.NoiseFloorDB) / rangeDB
			t = math.Max(0, math.Min(1, t))
			// Dark blue through green to white: monotone in brightness, so the
			// picture survives being read in greyscale.
			img.SetRGBA(x, y, color.RGBA{
				R: uint8(255 * math.Max(0, t*1.6-0.6)),
				G: uint8(255 * math.Max(0, t*1.4-0.15)),
				B: uint8(40 + 215*math.Min(1, t*1.2)),
				A: 255,
			})
		}
	}
	return img
}

// selectBurst dechirps the clicked frame and reports what the demodulator saw.
func (a *App) selectBurst(frame int) {
	if frame < 0 || frame >= len(a.wf.spec.Frames) {
		return
	}
	a.wf.selected = frame
	bins := a.wf.spec.Frames[frame]
	a.wf.symbols = append(a.wf.symbols[:0], bins...)

	// Peak and runner-up: the two numbers the capture decision turns on.
	a.wf.peak, a.wf.second = -1, -1
	for i, v := range bins {
		if a.wf.peak < 0 || v > bins[a.wf.peak] {
			a.wf.second, a.wf.peak = a.wf.peak, i
		} else if a.wf.second < 0 || v > bins[a.wf.second] {
			a.wf.second = i
		}
	}
	if a.wf.peak >= 0 && a.wf.second >= 0 {
		a.wf.ratioDB = bins[a.wf.peak] - bins[a.wf.second]
	}
}

// drawSymbolView is the dechirped FFT and the decision it implies.
func (a *App) drawSymbolView() {
	bins := a.wf.symbols
	if len(bins) == 0 {
		return
	}
	origin := imgui.CursorScreenPos()
	avail := imgui.ContentRegionAvail()
	h := float32(80)
	if avail.Y < h+30 {
		h = avail.Y - 30
	}
	if h < 24 {
		return
	}
	dl := imgui.WindowDrawList()

	lo, hi := bins[0], bins[0]
	for _, v := range bins {
		lo, hi = math.Min(lo, v), math.Max(hi, v)
	}
	span := hi - lo
	if span <= 0 {
		span = 1
	}
	bw := avail.X / float32(len(bins))
	for i, v := range bins {
		frac := float32((v - lo) / span)
		col := colour(0.4, 0.6, 0.9, 0.9)
		switch i {
		case a.wf.peak:
			col = colour(0.45, 0.9, 0.55, 1)
		case a.wf.second:
			col = colour(0.95, 0.72, 0.25, 1)
		}
		x := origin.X + float32(i)*bw
		dl.AddRectFilledV(imgui.NewVec2(x, origin.Y+h-frac*h),
			imgui.NewVec2(x+bw-1, origin.Y+h), col, 0, 0)
	}
	imgui.Dummy(imgui.NewVec2(avail.X, h))

	imgui.Text(fmt.Sprintf("peak bin %d, second %d, ratio %.1f dB",
		a.wf.peak, a.wf.second, a.wf.ratioDB))
	// The capture verdict, in the same terms MeshCore's own demodulator would
	// reach it: a strong enough winner takes the channel, and the loser is not
	// merely weaker — it is gone.
	switch {
	case a.wf.ratioDB >= engine.CaptureThresholdDB():
		imgui.PushStyleColorVec4(imgui.ColText, imgui.NewVec4(0.45, 0.85, 0.5, 1))
		textWrap(fmt.Sprintf("The stronger signal captured, by %.1f dB. "+
			"The weaker frame is lost - not corrupted, never demodulated.", a.wf.ratioDB))
		imgui.PopStyleColor()
	default:
		imgui.PushStyleColorVec4(imgui.ColText, imgui.NewVec4(0.95, 0.72, 0.25, 1))
		textWrap(fmt.Sprintf("No capture: %.1f dB between them, under the %.0f dB "+
			"a receiver needs to lock the stronger one. Both are likely lost.",
			a.wf.ratioDB, engine.CaptureThresholdDB()))
		imgui.PopStyleColor()
	}
}
