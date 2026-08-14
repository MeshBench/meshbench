package engine

import (
	"math"

	"github.com/MeshBench/meshbench/internal/dsp"
	"github.com/MeshBench/meshbench/internal/scenario"
)

// emitterNoiseAt is the extra noise power an emitter fleet delivers into one
// receiver's bandwidth, in dBm — -Inf when there are no emitters or none
// reach.
//
// Per ADR-0012: continuous interferers do not fit the burst channel loop, so
// they are a raised floor per receiver — computed through the same path loss,
// diffraction and antenna gains as a mesh transmission, so a mast behind a
// hill interferes less. The result is cached with the link cache: emitters
// move exactly as often as nodes do.
func (e *Engine) emitterNoiseAt(rx int) float64 {
	e.mu.Lock()
	if v, ok := e.emitterNoise[rx]; ok {
		e.mu.Unlock()
		return v
	}
	nodes := make([]*Node, len(e.nodes))
	copy(nodes, e.nodes)
	e.mu.Unlock()

	total := math.Inf(-1)
	rxSpec := nodes[rx].Spec
	rxPHY := e.phyOf(rxSpec)
	for i, n := range nodes {
		if n.Spec.Kind != scenario.Emitter || i == rx {
			continue
		}
		loss, ok := e.pathLoss(i, rx)
		if !ok {
			continue
		}
		p := n.Spec.TxPowerDBm + gain(n.Spec) - loss + gain(rxSpec)
		// Only the emitter power that lands inside the receiver's bandwidth
		// raises its floor. An out-of-band emitter contributes nothing here —
		// front-end blocking is a different curve, and pretending overlap
		// covers it would be wrong in the flattering direction.
		frac := overlapFraction(n.Spec.Radio.CentreHz, n.Spec.Radio.BandwidthHz,
			rxPHY.freqMHz*1e6, rxPHY.bandwidthHz)
		if frac <= 0 {
			continue
		}
		p += 10 * math.Log10(frac)
		duty := n.Spec.EmitterDutyPct
		if duty <= 0 || duty > 100 {
			duty = 100
		}
		p += 10 * math.Log10(duty/100)
		total = addDBm(total, p)
	}
	e.mu.Lock()
	e.emitterNoise[rx] = total
	e.mu.Unlock()
	return total
}

// overlapFraction is how much of an emitter's power falls inside a receiver's
// passband, assuming the emitter's power is spread evenly over its bandwidth —
// flat-spectrum, which overstates a carrier's edges and understates its
// centre, and is the honest simple model.
func overlapFraction(emCentre, emBW, rxCentre, rxBW float64) float64 {
	if emBW <= 0 {
		emBW = 1
	}
	lo := math.Max(emCentre-emBW/2, rxCentre-rxBW/2)
	hi := math.Min(emCentre+emBW/2, rxCentre+rxBW/2)
	if hi <= lo {
		return 0
	}
	return (hi - lo) / emBW
}

// FloorAt reports a node's noise floor: thermal, and with the emitter fleet's
// contribution — the per-receiver figure the link budget shows.
func (e *Engine) FloorAt(name string) (thermalDBm, withEmittersDBm float64, ok bool) {
	e.mu.Lock()
	idx := -1
	for i, n := range e.nodes {
		if n.Spec.Name == name {
			idx = i
			break
		}
	}
	e.mu.Unlock()
	if idx < 0 {
		return 0, 0, false
	}
	spec := e.nodeSpec(idx)
	thermal := dsp.NoiseFloorDBm(e.phyOf(spec).bandwidthHz, e.Config.NoiseFigDB)
	extra := e.emitterNoiseAt(idx)
	if math.IsInf(extra, -1) {
		return thermal, thermal, true
	}
	return thermal, addDBm(thermal, extra), true
}

func (e *Engine) nodeSpec(i int) scenario.Node {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.nodes[i].Spec
}
