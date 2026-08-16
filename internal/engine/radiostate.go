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
// **This figure is not traceable to a source, and may not be documented at
// all.** Neither RadioLib nor MeshCore states a decibel number for it anywhere.
// RadioLib's own maintainer left the feature unimplemented for exactly this
// reason, writing that "what exactly it means to 'consume more power to improve
// the sensitivity' seems to be undocumented. Is it an improvement of 10% or
// 0.1%?" (jgromes/RadioLib discussion 370). Semtech's datasheet points at the
// setting without quantifying it.
//
// So 2.0 is a placeholder of the right order, not a measurement, and it is the
// one number in this package that cannot be defended. Anything that turns on it
// must say so. The honest ways out are a bench measurement against real
// hardware, or expressing results as a range across plausible values - not a
// more confident constant.
//
// CLAUDE.md's rule for internal/scenario applies here too: anything uncertain
// belongs in a note rather than smoothed over.
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
	// Judged at the instant the node last transmitted, not on the line's
	// current level: the line is meant to be low while it listens, and a node
	// that has not transmitted at all has not answered the question.
	if fem := n.Spec.FEM; fem != nil {
		switch st.FemAtTx {
		case firmware.FemIn:
			txDBm += fem.TxGainDB
		case firmware.FemOut:
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
	// Both figures feed the path-loss cull, so a cached pair cannot outlive
	// them - but only pairs this node is party to are affected, not the whole
	// mesh. Wiping the entire cache here was the bug: the FEM-at-TX and
	// AGC-boost fields this reads change right after a node transmits, so a
	// blanket e.linkCache = map[[2]int]float64{} turned every transmission on
	// a many-node scenario into the frozen-minute DEM walk pathLoss's own
	// comment describes, instead of the once-per-import cost it was meant to
	// be. Scoped deletion keeps every other pair's cached figure intact.
	for k := range e.linkCache {
		if k[0] == i || k[1] == i {
			delete(e.linkCache, k)
		}
	}
	delete(e.emitterNoise, i)
}
