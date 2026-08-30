package engine

import (
	"math"

	"github.com/MeshBench/meshbench/internal/rf/channel"
	"github.com/MeshBench/meshbench/internal/rf/dsp"
	"github.com/MeshBench/meshbench/internal/rf/lora"
)

// captureThresholdDB is how much stronger one signal must be for a receiver to
// lock it and ignore the other.
//
// Six dB is the figure LoRa capture is usually quoted at, and it is the reason
// a flood works at all: without capture, every simultaneous relay would destroy
// every other one.
//
// Quoted is not measured, and this constant decides which packets survive a
// collision, so TestCaptureThresholdMatchesTheDemodulator puts two real chirps
// into one window at a sweep of power ratios and asks our own receive chain
// where it actually starts recovering the stronger one. The calculated path is
// supposed to be a fast twin of that chain; if the two disagree, the constant
// is wrong rather than the demodulator, and the test says so.
const captureThresholdDB = 6

// CaptureThresholdDB is that figure, for a UI that has to explain a verdict.
func CaptureThresholdDB() float64 { return captureThresholdDB }

// InFlightTransmissions renders what is on the air right now as baseband, as
// one receiver would see it.
//
// The engine works in frames because that is what the firmware exchanges; a
// waterfall needs samples. Rather than keep every transmission modulated for
// the whole run — hundreds of megabytes nobody looks at — the samples are
// synthesised on demand from the frames still in flight, using the same
// modulator the demodulator is tested against.
//
// So the picture is of the same signal the channel carried, arrived at through
// a second path. If the two ever disagreed, the waterfall would be showing a
// different simulation from the one producing the results.
func (e *Engine) InFlightTransmissions(rxIndex int) []channel.Transmission {
	e.mu.Lock()
	inFlight := append([]transmission(nil), e.inFlight...)
	nodes := make([]*Node, len(e.nodes))
	copy(nodes, e.nodes)
	e.mu.Unlock()

	if rxIndex < 0 || rxIndex >= len(nodes) {
		return nil
	}
	rxPHY := e.phyOf(nodes[rxIndex].Spec())

	var out []channel.Transmission
	cache := modCache{}
	for _, t := range inFlight {
		// Only what this receiver could actually see. A transmission on another
		// channel is not in this waterfall, for the same reason it is not in
		// this receiver's ledger.
		if !e.phyOf(nodes[t.from].Spec()).sameChannel(rxPHY) {
			continue
		}
		tx, ok := e.rxTransmission(t, rxIndex, float64(t.startMs), nodes, cache)
		if !ok {
			continue
		}
		tx.StartSample = 0 // the waterfall window is "now", not t's own start
		out = append(out, tx)
	}
	return out
}

// rxTransmission is one transmission as one receiver gets it: the shared
// synthesis the verdict, the waterfall and (in time) the SDR observers all
// render from. If these ever rendered from different signals, the picture
// would lie about the physics.
//
// anchorMs is the start of the observation window; StartSample lands the
// transmission at its true offset within it, at the channel's baseband rate.
// The anchor is fractional milliseconds because the observers walk a sample
// clock: a millisecond is 62.5 samples at 62.5 kHz, and truncating the half
// tore the stream's phase at every window seam - forty broadband clicks a
// second, drawn as a burst filling the whole span. The sub-sample remainder
// rides on DelaySamples, which channel.Observe turns into the phase it is.
func (e *Engine) rxTransmission(t transmission, rxIdx int, anchorMs float64,
	nodes []*Node, cache modCache) (channel.Transmission, bool) {
	src := nodes[t.from]
	txPHY := e.phyOf(src.Spec())
	loss, ok := e.pathLoss(t.from, rxIdx)
	if !ok {
		return channel.Transmission{}, false
	}
	spms := txPHY.bandwidthHz / 1000
	rel := (float64(t.startMs) - anchorMs) * spms
	start := math.Floor(rel)
	tx := channel.Transmission{
		Node:         src.Spec().Name,
		Samples:      e.modulated(cache, t, txPHY),
		GainDB:       src.Spec().TxPowerDBm + gain(src.Spec()) - loss + gain(nodes[rxIdx].Spec()),
		StartSample:  int(start),
		DelaySamples: rel - start,
		PhaseStepRad: e.phaseStepFor(src, nodes[rxIdx], txPHY),
	}
	return tx, true
}

// rxTransmissions is rxTransmission plus whatever realism adds to the path -
// today one multipath echo when that switch is on. Every synthesis consumer
// takes this, so an echo the verdict hears is an echo the waterfall shows.
func (e *Engine) rxTransmissions(t transmission, rxIdx int, anchorMs float64,
	nodes []*Node, cache modCache) []channel.Transmission {
	direct, ok := e.rxTransmission(t, rxIdx, anchorMs, nodes, cache)
	if !ok {
		return nil
	}
	out := []channel.Transmission{direct}
	if echo, has := e.echoFor(direct, nodes[t.from].Spec().Name,
		nodes[rxIdx].Spec().Name, e.phyOf(nodes[t.from].Spec()), t.startMs); has {
		out = append(out, echo)
	}
	return out
}

// loraParams is the coding configuration a transmitter's modem implies:
// explicit header, hardware CRC, and LDRO by the chip's own 16 ms rule -
// exactly the terms RadioLib's airtime formula uses, which is what keeps
// the waveform's length and the firmware's CSMA arithmetic identical.
func loraParams(p phy) lora.Params {
	symbolMs := float64(uint64(1)<<uint(p.sf)) / (p.bandwidthHz / 1000)
	return lora.Params{SF: p.sf, CR: p.codingRate, LDRO: symbolMs >= 16, CRC: true}
}

// frameLayout is the on-air arrangement a transmitter's modem implies:
// MeshCore's own preamble length, the standard private sync word, the SFD.
func frameLayout(p phy) dsp.FrameLayout {
	a, b := dsp.StandardSync(p.sf)
	return dsp.FrameLayout{SF: p.sf, Preamble: dsp.PreambleSymbols(p.sf), SyncA: a, SyncB: b}
}

// frameSamples renders a frame as a real SX126x would send it: preamble,
// sync word, SFD downchirps, then the fully coded data symbols - header,
// whitening, Hamming, interleaving, Gray. The waterfall, the verdict and the
// SDR observers all render from this one stream, bit-faithful.
func frameSamples(frame []byte, p phy) []complex128 {
	data, err := lora.Encode(loraParams(p), frame)
	if err != nil {
		// An unencodable frame (SF outside 7..12, >255 bytes) still needs a
		// waveform for the ledger to reason about; a bare preamble is honest
		// about carrying nothing.
		data = nil
	}
	return frameLayout(p).FrameSamples(data)
}
