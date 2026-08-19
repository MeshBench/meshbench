// The imperfections a kind simulator leaves out, made optional and honest.
//
// Every effect here defaults off, is switched in the RF Simulation section
// of Configuration, and is stamped into results with the rest of the run's
// physics. They apply to the waveform paths - synthesis and reception - so
// switching one on changes IQ, and the receiver has to cope the way silicon
// does: the front end's CFO estimator earns its keep the first time
// oscillator error is real.
package engine

import (
	"math"

	"github.com/MeshBench/meshbench/internal/rf"
)

// Realism is the switch set, carried on Config.
type Realism struct {
	// OscillatorPPM is the worst-case crystal error, in parts per million.
	// Each node gets a deterministic offset in [-ppm, +ppm] derived from its
	// name, and what the channel carries is the difference between the
	// transmitter's and the receiver's - exactly as two real radios disagree.
	OscillatorPPM float64
	// MultipathEchoDB adds one delayed reflection per path, this many dB
	// below the direct ray. Zero is off. The echo's excess delay is
	// deterministic per pair, and its phase is what makes two arrivals
	// reinforce or cancel - fading, from geometry rather than a dice roll.
	MultipathEchoDB float64
	// FadingHz slowly rotates each echo's phase over simulated time, so the
	// cancellation pattern breathes the way a real marginal link does.
	FadingHz float64
	// ImplementationLossDB is the receiver's shortfall from theory - the
	// SX1262's datasheet floors already include roughly 2 dB of it. Applied
	// as extra receiver noise, which is what it physically is.
	ImplementationLossDB float64
	// SaturationDBm clips the receiver's front end: any sample stronger is
	// flattened, harmonics and all. Zero means no clipping is modelled.
	SaturationDBm float64
}

// oscOffsetPPM is one node's crystal error: deterministic in the name, so
// the same scenario always disagrees with itself in the same way.
func oscOffsetPPM(name string, worstPPM float64) float64 {
	if worstPPM == 0 {
		return 0
	}
	h := uint64(1469598103934665603)
	for _, b := range []byte(name) {
		h ^= uint64(b)
		h *= 1099511628211
	}
	// Uniform in [-1, 1) from the hash's top bits.
	u := float64(h>>11) / float64(1<<53)
	return (u*2 - 1) * worstPPM
}

// phaseStepFor is the baseband rotation per sample that a transmitter and
// receiver pair's oscillator disagreement produces.
func (e *Engine) phaseStepFor(txNode, rxNode *Node, txPHY phy) float64 {
	ppm := e.Config.Realism.OscillatorPPM
	if ppm == 0 {
		return 0
	}
	dppm := oscOffsetPPM(txNode.Spec.Name, ppm) - oscOffsetPPM(rxNode.Spec.Name, ppm)
	offsetHz := txPHY.freqMHz * 1e6 * dppm / 1e6
	return 2 * math.Pi * offsetHz / txPHY.bandwidthHz
}

// echoFor is the one reflection multipath adds for a pair, or nothing.
//
// Excess path is deterministic in the pair - a few hundred metres to a
// couple of kilometres, the scale of a hill or a building line - and the
// phase it arrives with is the delay times the carrier, drifted by the
// fading rate. This is not a channel sounder's tap model; it is the minimum
// physics that makes flat fading emerge from the sum instead of a dice roll.
func (e *Engine) echoFor(direct rf.Transmission, txName, rxName string,
	txPHY phy, atMs uint32) (rf.Transmission, bool) {
	echoDB := e.Config.Realism.MultipathEchoDB
	if echoDB <= 0 {
		return rf.Transmission{}, false
	}
	h := uint64(14695981039346656037)
	for _, b := range []byte(txName + "\x00" + rxName) {
		h ^= uint64(b)
		h *= 1099511628211
	}
	excessM := 200 + float64(h%1800) // 200..2000 m of extra path
	delaySec := excessM / 299792458.0
	delaySamples := delaySec * txPHY.bandwidthHz
	// Carrier phase across the excess path, plus the fading drift.
	carrierCycles := delaySec * txPHY.freqMHz * 1e6
	fade := e.Config.Realism.FadingHz * float64(atMs) / 1000
	phase := 2 * math.Pi * (carrierCycles + fade + float64(h%997)/997)

	echo := direct
	echo.GainDB = direct.GainDB - echoDB
	echo.DelaySamples = direct.DelaySamples + delaySamples
	// The phase rides on DelaySamples' fractional part in rf.Observe; fold
	// the carrier phase in by adjusting the fractional delay it derives
	// from. One rotation is one sample's fraction there.
	_, frac := math.Modf(phase / (2 * math.Pi))
	echo.DelaySamples = math.Floor(echo.DelaySamples) + frac
	return echo, true
}

// applyImplementationLoss raises a noise floor by the configured shortfall.
func (e *Engine) applyImplementationLoss(noiseDBm float64) float64 {
	return noiseDBm + e.Config.Realism.ImplementationLossDB
}

// saturate clips a window at the configured front-end ceiling, in place.
func (e *Engine) saturate(iq []complex128) {
	satDBm := e.Config.Realism.SaturationDBm
	if satDBm == 0 {
		return
	}
	limit := math.Pow(10, satDBm/20)
	for i, v := range iq {
		mag := math.Hypot(real(v), imag(v))
		if mag > limit {
			scale := limit / mag
			iq[i] = complex(real(v)*scale, imag(v)*scale)
		}
	}
}

// SetRealism switches the imperfections live, on the same
// whole-transmission boundary the mode switch uses.
func (e *Engine) SetRealism(r Realism) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.Config.Realism = r
}
