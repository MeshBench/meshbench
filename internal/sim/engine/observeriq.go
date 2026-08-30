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

	"github.com/MeshBench/meshbench/internal/rf/channel"
	"github.com/MeshBench/meshbench/internal/rf/dsp"
	"github.com/MeshBench/meshbench/internal/world/scenario"
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
	e.nodes[idx].changeSpec(func(s *scenario.Node) {
		s.Position.Lat, s.Position.Lon = lat, lon
	})
	for k := range e.linkCache {
		if k[0] == idx || k[1] == idx {
			delete(e.linkCache, k)
			delete(e.culled, k)
		}
	}
	// The ground between the pairs changed, so the profiles go with the
	// losses - this is the one invalidation that reaches them.
	for k := range e.profCache {
		if k[0] == idx || k[1] == idx {
			delete(e.profCache, k)
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
	return e.observeSpan(rxIdx, float64(fromMs), n, true)
}

// ObserveSignalAt is the observers' entry: signal only - the presentation
// layer supplies the front-end floor itself - and addressed by an absolute
// sample index rather than milliseconds, because a continuous stream lives
// on a sample clock. A millisecond is not a whole number of samples at
// every bandwidth, and rounding the difference tore the stream's phase at
// every window seam. The verdicts never use this - a receiver without
// noise is not a receiver.
func (e *Engine) ObserveSignalAt(rxIdx int, fromSample uint64, n int) []complex128 {
	e.mu.Lock()
	ok := rxIdx >= 0 && rxIdx < len(e.nodes)
	var spec scenario.Node
	if ok {
		spec = e.nodes[rxIdx].Spec()
	}
	e.mu.Unlock()
	if !ok {
		return make([]complex128, n)
	}
	spms := e.phyOf(spec).bandwidthHz / 1000
	return e.observeSpan(rxIdx, float64(fromSample)/spms, n, false)
}

// ObserverNoisePSD is the receiver's noise density in linear watts per
// hertz - the level a presentation layer paints its own wideband floor at,
// so the floor it shows and the floor the verdicts hear are the same claim.
func (e *Engine) ObserverNoisePSD(rxIdx int) float64 {
	e.mu.Lock()
	if rxIdx < 0 || rxIdx >= len(e.nodes) {
		e.mu.Unlock()
		return 0
	}
	spec := e.nodes[rxIdx].Spec()
	e.mu.Unlock()
	rxPHY := e.phyOf(spec)
	noiseDBm := dsp.NoiseFloorDBm(rxPHY.bandwidthHz, e.noiseFigOf(spec))
	return math.Pow(10, noiseDBm/10) / rxPHY.bandwidthHz
}

func (e *Engine) observeSpan(rxIdx int, fromMs float64, n int, withNoise bool) []complex128 {
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

	rxPHY := e.phyOf(nodes[rxIdx].Spec())
	spms := rxPHY.bandwidthHz / 1000
	spanMs := float64(n) / spms
	// The cache survives between windows and is pruned to the air: without
	// it every 50 ms chunk re-encoded and re-modulated every overlapping
	// frame from scratch, the stream fell behind the ticker the moment
	// anything transmitted, and a starved SDR++ drew garbage.
	e.obsMu.Lock()
	defer e.obsMu.Unlock()
	if e.obsCache == nil {
		e.obsCache = modCache{}
	}
	alive := map[uint64]bool{}
	for _, t := range air {
		alive[t.packetID] = true
	}
	for id := range e.obsCache {
		if !alive[id] {
			delete(e.obsCache, id)
		}
	}
	cache := e.obsCache
	var txs []channel.Transmission
	for _, t := range air {
		if float64(t.endMs) <= fromMs || float64(t.startMs) >= fromMs+spanMs {
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
	noisePower := 0.0
	if withNoise {
		noiseDBm := dsp.NoiseFloorDBm(rxPHY.bandwidthHz, e.noiseFigOf(nodes[rxIdx].Spec()))
		noisePower = math.Pow(10, noiseDBm/10)
	}
	return channel.Observe(txs, channel.Receiver{
		NoisePowerLinear: noisePower,
		Seed:             seed,
		Offset:           uint64(fromMs*64)*0xA24BAED4963EE407 + uint64(rxIdx)<<44,
	}, n)
}
