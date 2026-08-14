package engine

import "github.com/MeshBench/meshbench/internal/firmware"

// What a node's radio is worth, given what its firmware has actually configured
// it to be rather than what its board profile claims it can do.
//
// The board figures are datasheet claims about hardware, and they are the right
// answer for any node whose firmware set its radio up correctly. For one that
// did not they are simply wrong, and until the chip began reporting its own
// state there was no way here to tell the two apart. MeshCore 1.17.1 fixed a
// fault of each kind - receive gain reverting on an AGC reset, and a
// transmit-enable line that never went high - and this simulator would have
// reported the datasheet figures through both.

// RxBoostedGainImprovementDB is how much better the receiver is in boosted gain
// than in power saving.
//
// A chosen figure rather than a cited one: neither RadioLib nor MeshCore states
// a decibel number for this anywhere in their sources, so it cannot be traced
// the way the rest of internal/scenario's figures can. Section 9.6 of the SX126x
// datasheet is the authority. Named so that reconciling it is one edit.
const RxBoostedGainImprovementDB = 2.0

// txPowerUnset is what the chip reports before the firmware has called
// SetTxParams. Zero cannot carry that meaning, because 0 dBm is a level a radio
// can legitimately be set to.
const txPowerUnset = -128

// effectiveRF is the transmit power and receive noise figure a node's radio
// actually has, from the board's baseline and what the firmware programmed.
//
// ok is false when the radio has not reported - a node that has not come up, or
// one whose build predates the chip carrying its configuration - and the caller
// then leaves the board's figures alone rather than replacing them with zeroes.
func effectiveRF(n *Node, st firmware.RadioStats) (txDBm, nfDB float64, ok bool) {
	if !st.Configured {
		return 0, 0, false
	}

	// Receive. The board's figure is the power-saving case, because that is the
	// gain register's reset default and what the chip holds until it is asked
	// for anything else.
	nfDB = n.baseNoiseFigDB
	if nfDB > 0 && st.RxBoosted() {
		nfDB -= RxBoostedGainImprovementDB
	}

	// Transmit. What the PA was asked for, then what the board's front end does
	// with it - and the front end contributes only if the firmware switched it
	// in. A board with no module is unaffected either way.
	txDBm = n.baseTxPowerDBm
	if st.TxPowerDBm != txPowerUnset {
		txDBm = float64(st.TxPowerDBm)
	}
	if fem := n.Spec.FEM; fem != nil {
		if st.FemEnabled {
			txDBm += fem.TxGainDB
		} else {
			txDBm -= fem.TxLossDB
		}
	}
	return txDBm, nfDB, true
}

// ApplyRadioState updates one node's effective radio parameters from what its
// firmware has configured, and drops the caches that depended on the old ones.
//
// Called every tick, so it returns early when nothing moved: the firmware
// reports its configuration whether or not it touched it, and refilling the link
// cache on a country-sized import is the expensive thing this engine does.
func (e *Engine) ApplyRadioState(i int, st firmware.RadioStats) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if i < 0 || i >= len(e.nodes) {
		return
	}
	n := e.nodes[i]
	txDBm, nfDB, ok := effectiveRF(n, st)
	if !ok || (n.Spec.TxPowerDBm == txDBm && n.Spec.NoiseFigureDB == nfDB) {
		return
	}
	n.Spec.TxPowerDBm = txDBm
	n.Spec.NoiseFigureDB = nfDB
	// Both figures feed the path-loss cull, so the link cache cannot outlive
	// them. Inline rather than calling InvalidateLinks, which takes this lock.
	e.linkCache = map[[2]int]float64{}
	e.emitterNoise = map[int]float64{}
}
