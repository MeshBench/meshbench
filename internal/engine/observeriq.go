// Continuous IQ for a placed observer: the same RF world, pulled as samples.
//
// An SDR observer is a node like any other - position, antenna, noise figure
// - and what it streams is rendered by the same shared synthesis the verdict
// and the waterfall use. Nothing here reads packet events or metadata: if
// two nodes collide, the observer sees the collision because the summed IQ
// contains it, and for no other reason.
package engine

import (
	"math"

	"github.com/MeshBench/meshbench/internal/dsp"
	"github.com/MeshBench/meshbench/internal/rf"
)

// SetNodePosition moves a node live, physics included: the position changes
// and every cached path loss that involved it is forgotten, so the next
// window prices the new geometry. This is what makes walking an observer
// around the map change what it hears while a client is attached.
func (e *Engine) SetNodePosition(idx int, lat, lon float64) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if idx < 0 || idx >= len(e.nodes) {
		return
	}
	e.nodes[idx].Spec.Position.Lat = lat
	e.nodes[idx].Spec.Position.Lon = lon
	for k := range e.linkCache {
		if k[0] == idx || k[1] == idx {
			delete(e.linkCache, k)
		}
	}
}

// ObserveSpan renders what one receiver's antenna carried over a span of
// simulated time: every transmission that overlapped it, summed coherently,
// plus that receiver's own noise floor. n samples at the receiver's
// bandwidth rate, starting at fromMs.
//
// Deterministic like everything else: the noise stream is keyed by the span
// and the receiver, so replaying a stretch of time replays its samples.
func (e *Engine) ObserveSpan(rxIdx int, fromMs uint32, n int) []complex128 {
	e.mu.Lock()
	nodes := make([]*Node, len(e.nodes))
	copy(nodes, e.nodes)
	air := make([]transmission, 0, len(e.inFlight)+len(e.recent))
	air = append(air, e.inFlight...)
	air = append(air, e.recent...)
	seed := e.Config.Seed
	e.mu.Unlock()
	if rxIdx < 0 || rxIdx >= len(nodes) {
		return nil
	}

	rxPHY := e.phyOf(nodes[rxIdx].Spec)
	spms := rxPHY.bandwidthHz / 1000
	spanMs := float64(n) / spms
	cache := modCache{}
	var txs []rf.Transmission
	for _, t := range air {
		if float64(t.endMs) <= float64(fromMs) || float64(t.startMs) >= float64(fromMs)+spanMs {
			continue
		}
		if t.from == rxIdx {
			continue
		}
		// The observer is wideband by design - the same exemption delivery
		// gives it - so no sameChannel gate here; what is on the air is what
		// it sees.
		if tx, ok := e.rxTransmission(t, rxIdx, fromMs, nodes, cache); ok {
			txs = append(txs, tx)
		}
	}
	noiseDBm := dsp.NoiseFloorDBm(rxPHY.bandwidthHz, e.noiseFigOf(nodes[rxIdx].Spec))
	return rf.Observe(txs, rf.Receiver{
		NoisePowerLinear: math.Pow(10, noiseDBm/10),
		Seed:             seed,
		Offset:           uint64(fromMs)*0xA24BAED4963EE407 + uint64(rxIdx)<<44,
	}, n)
}
