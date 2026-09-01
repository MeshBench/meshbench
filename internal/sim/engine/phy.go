// The radio arithmetic: what a node transmits with, what it can hear, and
// whether two of them are even on the same channel.
//
// Small, dull functions that everything else leans on. They are together
// because they are the layer where a decibel is still a decibel - no packets,
// no scheduling, nothing that knows a mesh exists.
package engine

import (
	"math"

	"github.com/MeshBench/meshbench/internal/mesh/packet"
	"github.com/MeshBench/meshbench/internal/rf/antenna"
	"github.com/MeshBench/meshbench/internal/rf/dsp"
	"github.com/MeshBench/meshbench/internal/rf/geo"
	"github.com/MeshBench/meshbench/internal/world/scenario"
)

// bestGainDBi is what an antenna could manage if the far end were straight
// down its boresight, feedline already deducted.
//
// Only the path-loss cull may use it. The cull asks whether a pair could
// matter at all, so it wants each end's best case: charging a real look angle
// there would discard links that close. Everything that prices an actual
// reception goes through linkGainsDBi instead, which evaluates the pattern in
// the direction the far end is really in.
func bestGainDBi(n scenario.Node) float64 {
	if n.Antenna.Pattern == nil {
		return 0
	}
	return n.Antenna.Pattern.PeakDBi() - n.Antenna.FeedlineDB
}

// linkGainsDBi is each end's antenna gain in the true direction of the other:
// the transmitter's first, the receiver's second.
//
// Cached, because a pair is asked several times per transmission - once for
// the wanted signal, again for the demodulator contest, again for every
// interferer that lands on it - and the answer is trigonometry that only a
// node moving can change.
func (e *Engine) linkGainsDBi(nodes []*Node, from, to int) (txDBi, rxDBi float64) {
	k := [2]int{from, to}
	e.gainMu.RLock()
	g, ok := e.gainCache[k]
	e.gainMu.RUnlock()
	if ok {
		return g[0], g[1]
	}
	txDBi, rxDBi = e.measureGains(nodes, from, to)
	e.gainMu.Lock()
	if e.gainCache == nil {
		e.gainCache = map[[2]int][2]float64{}
	}
	e.gainCache[k] = [2]float64{txDBi, rxDBi}
	e.gainMu.Unlock()
	return txDBi, rxDBi
}

// measureGains evaluates both patterns in the direction each end is really
// in: bearing to the far end, and the elevation angle from both altitudes.
//
// Two evaluations, never one shared figure. The bearing from a to b is not
// the reverse of the bearing from b to a, the elevation angle changes sign
// between them, and the two ends are rarely the same antenna - so a single
// scalar cannot be right for both directions of a link that is asymmetric by
// nature.
//
// This was peak gain until it was not, and the error was not academic. Every
// imported node is given a collinear, whose gain falls three decibels within
// about four degrees of the horizon at 10 dBi: a repeater 500 m above a node
// 5 km away is looking almost six degrees down, and the engine was crediting
// it roughly six decibels it does not have. Hill paths are the ordinary case
// for this tool, and the coverage raster - which has always evaluated the
// pattern properly - therefore disagreed with the packets.
func (e *Engine) measureGains(nodes []*Node, from, to int) (txDBi, rxDBi float64) {
	a, b := nodes[from].specRef(), nodes[to].specRef()
	if a.Antenna.Pattern == nil && b.Antenna.Pattern == nil {
		// Nothing to point. Common enough in scenarios built by hand, and
		// worth the branch: it skips the trigonometry as well as the patterns.
		return 0, 0
	}
	distKm := geo.DistanceKm(a.Position.Lat, a.Position.Lon, b.Position.Lat, b.Position.Lon)
	if distKm <= 0 {
		// Two nodes at one point have no direction between them. Nothing is
		// delivered over such a pair anyway - pathLoss refuses it - so the
		// best case is the least surprising answer.
		return bestGainDBi(*a), bestGainDBi(*b)
	}
	up := geo.ElevationDeg(
		e.groundAt(nodes[from])+a.HeightAGLm,
		e.groundAt(nodes[to])+b.HeightAGLm, distKm)
	txDBi = gainTowards(a, geo.BearingDeg(a.Position.Lat, a.Position.Lon,
		b.Position.Lat, b.Position.Lon), up)
	rxDBi = gainTowards(b, geo.BearingDeg(b.Position.Lat, b.Position.Lon,
		a.Position.Lat, a.Position.Lon), -up)
	return txDBi, rxDBi
}

// gainTowards is Mounted.GainTowardsDBi with the missing-pattern case, which
// a scenario built by hand or imported without an antenna still produces.
func gainTowards(n *scenario.Node, bearingDeg, elevationDeg float64) float64 {
	if n.Antenna.Pattern == nil {
		return 0
	}
	return n.Antenna.GainTowardsDBi(bearingDeg, elevationDeg)
}

// rxPowerDBm is what one node's transmission delivers at another's antenna:
// transmit power, both patterns evaluated towards each other, the path loss
// between them, and what the two ends' polarisations cost each other. Every
// judgement in the package is built on this one line, so there is one place a
// decibel can go missing rather than eleven.
//
// The polarisation term is charged here rather than folded into either end's
// gain because it belongs to neither: it is what the pair loses, and a handheld
// held sideways is only mismatched with respect to the mast it is talking to.
func (e *Engine) rxPowerDBm(nodes []*Node, from, to int, lossDB float64) float64 {
	txDBi, rxDBi := e.linkGainsDBi(nodes, from, to)
	crossPol := antenna.MismatchLossDB(
		nodes[from].specRef().Antenna, nodes[to].specRef().Antenna)
	return nodes[from].specRef().TxPowerDBm + txDBi - lossDB + rxDBi - crossPol
}

// groundAt is the terrain elevation under a node, measured once and kept on
// the node's own state.
//
// A look angle needs both ends' altitude, and the DEM lookup behind it can
// reach the network - which the delivery path must never do, and which would
// otherwise happen once per receiver per transmission. It is not behind the
// engine's lock because this is asked from the parallel judges, where four
// lock acquisitions per pair cost more than the arithmetic they guard.
//
// Where the terrain has no answer both ends read as sea level, so the angle
// falls back to the difference in mast heights: flat earth, which is what a
// scenario with no DEM under it is.
func (e *Engine) groundAt(n *Node) float64 {
	st := n.state.Load()
	if st.groundKnown {
		return st.groundM
	}
	h := 0.0
	if e.Terrain != nil {
		if g, found := e.Terrain.ElevationM(st.spec.Position.Lat, st.spec.Position.Lon); found {
			h = g
		}
	}
	next := *st
	next.groundM, next.groundKnown = h, true
	// Losing the swap means somebody measured the same ground first, or the
	// node moved and this answer is already stale. Both are answers to throw
	// away, never to force.
	n.state.CompareAndSwap(st, &next)
	return h
}

// dropGains forgets the look angles a node is party to, or all of them when
// idx is negative. The bearing and the elevation are geometry, so this is
// the one thing that invalidates them: a node that moved.
func (e *Engine) dropGains(idx int) {
	e.gainMu.Lock()
	defer e.gainMu.Unlock()
	if idx < 0 {
		e.gainCache = nil
		return
	}
	for k := range e.gainCache {
		if k[0] == idx || k[1] == idx {
			delete(e.gainCache, k)
		}
	}
}

// LinkGainsDBiForTest exposes the pair's directional gains, so a test can
// hold the engine and a coverage raster to the same arithmetic.
func (e *Engine) LinkGainsDBiForTest(from, to int) (txDBi, rxDBi float64) {
	return e.linkGainsDBi(e.Nodes(), from, to)
}

// addDBm sums two powers expressed in dBm. Adding decibels instead is a mistake
// that produces a plausible number and is wrong by tens of dB.
func addDBm(a, b float64) float64 {
	if math.IsInf(a, -1) {
		return b
	}
	if math.IsInf(b, -1) {
		return a
	}
	return 10 * math.Log10(math.Pow(10, a/10)+math.Pow(10, b/10))
}

// phy is one node's modem settings, resolved.
type phy struct {
	freqMHz     float64
	bandwidthHz float64
	sf          int
	codingRate  int
}

// sameChannel reports whether two radios can hear each other at all.
//
// Frequency and bandwidth must match; spreading factor too, because LoRa's
// orthogonality across SFs is the property the whole modulation is sold on — a
// receiver at SF10 does not demodulate SF7, it sees noise. Coding rate is in
// the explicit header, so it does not have to match.
func (a phy) sameChannel(b phy) bool {
	return math.Abs(a.freqMHz-b.freqMHz) < 0.001 &&
		a.bandwidthHz == b.bandwidthHz && a.sf == b.sf
}

// noiseFigOf resolves a node's receive noise figure, falling back to the
// scenario's default for nodes imported without one.
//
// Per node rather than per run because a repeater with a masthead preamp and a
// handheld in a pocket do not have the same one. scenario.Node has carried the
// field since import and the engine simply never read it, so every node in
// every result so far has been given the run-wide figure regardless of what its
// board profile said.
func (e *Engine) noiseFigOf(n *scenario.Node) float64 {
	if n.NoiseFigureDB > 0 {
		return n.NoiseFigureDB
	}
	return e.Config.NoiseFigDB
}

// phyOf resolves a node's radio, falling back to the scenario's defaults for
// nodes imported without one.
func (e *Engine) phyOf(n *scenario.Node) phy {
	p := phy{
		freqMHz:     n.Radio.CentreHz / 1e6,
		bandwidthHz: n.Radio.BandwidthHz,
		sf:          n.Radio.SpreadFactor,
		codingRate:  n.Radio.CodingRate,
	}
	if p.freqMHz <= 0 {
		p.freqMHz = e.Config.FreqMHz
	}
	if p.bandwidthHz <= 0 {
		p.bandwidthHz = e.Config.BandwidthHz
	}
	if p.sf <= 0 {
		p.sf = e.Config.SF
	}
	if p.codingRate <= 0 {
		p.codingRate = e.Config.CodingRate
	}
	return p
}

// requiredSNRdB is Semtech's published demodulator floor, which the modem has
// been measured against to within 1.6 dB (docs/shortcomings.md).
func requiredSNRdB(sf int) float64 {
	if v, ok := dsp.RequiredSNRdB[sf]; ok {
		return v
	}
	return -20
}

// payloadID identifies a message by its content, across every hop it takes.
//
// The header and the payload; deliberately *not* the path. A flood packet grows
// a hop hash at every relay, so hashing the whole frame gave the same message a
// new identity at each hop — every relay looked like a brand new message, no
// message could be followed across the mesh, and the unique-versus-redundant
// count was measuring nothing.
//
// The route bits are masked out of the header for the same reason: a node may
// re-route a packet it forwards, and a message that changed identity when it
// switched from flood to direct would break at exactly the interesting moment.
func payloadID(frame []byte) uint64 {
	d := packet.Dissect(frame)
	h := uint64(14695981039346656037)
	mix := func(b byte) {
		h ^= uint64(b)
		h *= 1099511628211
	}
	if d.Truncated {
		// Unparseable: fall back to the whole frame. It cannot be followed, but
		// two identical malformed frames should still be one thing.
		for _, b := range frame {
			mix(b)
		}
		return h
	}
	// Payload type only, not the route bits or the version.
	mix(d.PayloadType)
	for _, b := range d.Payload {
		mix(b)
	}
	return h
}

// PayloadIDForTest exposes the message identity for tests.
func PayloadIDForTest(frame []byte) uint64 { return payloadID(frame) }
