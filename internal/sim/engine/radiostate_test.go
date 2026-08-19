package engine

import (
	"testing"

	"github.com/MeshBench/meshbench/internal/mesh/firmware"
	"github.com/MeshBench/meshbench/internal/world/scenario"
)

// board is a node as imported: the datasheet figures, before any firmware has
// said what it actually did with the radio.
func board(nfDB, txDBm float64, fem *scenario.FEM) *Node {
	return &Node{
		Spec:           scenario.Node{NoiseFigureDB: nfDB, TxPowerDBm: txDBm, FEM: fem},
		baseNoiseFigDB: nfDB,
		baseTxPowerDBm: txDBm,
	}
}

// configured is what a radio that has come up and been set up reports.
func configured(gain uint8, txDBm int8, fem firmware.FemState) firmware.RadioStats {
	return firmware.RadioStats{
		Configured: true, RxGainReg: gain, TxPowerDBm: txDBm, FemAtTx: fem,
	}
}

// A radio that has not reported must not be read as one reporting zeroes. The
// distinction is the whole reason RadioStats carries Configured: a node that has
// not come up yet would otherwise be given 0 dBm and a 0 dB noise figure, which
// is a silent, extremely optimistic receiver.
func TestAnUnreportedRadioLeavesTheBoardFiguresAlone(t *testing.T) {
	n := board(6, 22, nil)
	if _, _, ok := effectiveRF(n, firmware.RadioStats{}); ok {
		t.Fatal("an unconfigured radio was treated as having reported")
	}
}

// The board's noise figure is the power-saving case, because that is the gain
// register's reset default. Boosted gain is an improvement on it.
func TestBoostedGainImprovesTheNoiseFigure(t *testing.T) {
	n := board(6, 22, nil)

	_, nf, ok := effectiveRF(n, configured(firmware.RxGainPowerSaving, 22, firmware.FemUnknown))
	if !ok || nf != 6 {
		t.Fatalf("power saving: noise figure = %v, want the board's 6", nf)
	}

	_, nf, ok = effectiveRF(n, configured(firmware.RxGainBoosted, 22, firmware.FemUnknown))
	if !ok {
		t.Fatal("boosted: not reported")
	}
	if want := 6 - RxBoostedGainImprovementDB; nf != want {
		t.Fatalf("boosted: noise figure = %v, want %v", nf, want)
	}
}

// The fault MeshCore 1.17.1 fixed, seen from the engine.
//
// A node comes up in boosted gain, then an AGC reset runs a full calibration.
// On a variant that does not define SX126X_RX_BOOSTED_GAIN - generic-e22 among
// them - sx126xResetAGC re-applies nothing, so the chip is left in power saving
// and the node quietly loses sensitivity. The firmware's own prefs and its CLI
// go on reporting the operator's setting, so this is the only place it shows.
func TestGainLostToAnAGCResetCostsSensitivity(t *testing.T) {
	n := board(6, 22, nil)

	_, before, _ := effectiveRF(n, configured(firmware.RxGainBoosted, 22, firmware.FemUnknown))
	_, after, _ := effectiveRF(n, configured(firmware.RxGainPowerSaving, 22, firmware.FemUnknown))

	if after <= before {
		t.Fatalf("losing boosted gain did not worsen the receiver: %v then %v",
			before, after)
	}
	if got := after - before; got != RxBoostedGainImprovementDB {
		t.Fatalf("sensitivity moved %v dB, want %v", got, RxBoostedGainImprovementDB)
	}
}

// A board with no front-end module is unaffected by the transmit-enable line,
// because it has no such line and whether it was asserted means nothing.
func TestABoardWithNoModuleIgnoresTheEnableLine(t *testing.T) {
	n := board(6, 22, nil)
	in, _, _ := effectiveRF(n, configured(firmware.RxGainBoosted, 14, firmware.FemIn))
	out, _, _ := effectiveRF(n, configured(firmware.RxGainBoosted, 14, firmware.FemOut))
	if in != out || in != 14 {
		t.Fatalf("no module: in %v, out %v, want 14 either way", in, out)
	}
}

// A node that has not transmitted has not answered the question, and must not
// be charged the loss for it. Observed on a live run: reading the line's current
// level docked a node 25 dB for the ordinary state of listening.
func TestANodeThatHasNotTransmittedIsNotChargedForIt(t *testing.T) {
	n := board(6, 22, &scenario.FEM{TxGainDB: 0, TxLossDB: 25})
	tx, _, ok := effectiveRF(n, configured(firmware.RxGainBoosted, 22, firmware.FemUnknown))
	if !ok || tx != 22 {
		t.Fatalf("never transmitted: %v dBm, want the board's 22 untouched", tx)
	}
}

// The T096 case. The board is compiled for 9 dBm at the chip and reaches about
// 22 at the antenna through a KCT8103L, so a firmware that never switches the
// module in transmits 13 dB down - which is the difference between a link and
// no link, and which the board profile's MaxTxDBm cannot express.
func TestAnUnswitchedFrontEndCostsItsGain(t *testing.T) {
	n := board(6, 22, &scenario.FEM{TxGainDB: 13, TxLossDB: 0})

	in, _, _ := effectiveRF(n, configured(firmware.RxGainBoosted, 9, firmware.FemIn))
	if in != 22 {
		t.Fatalf("module switched in: %v dBm, want 9 + 13", in)
	}
	out, _, _ := effectiveRF(n, configured(firmware.RxGainBoosted, 9, firmware.FemOut))
	if out != 9 {
		t.Fatalf("module not switched in: %v dBm, want the chip's 9", out)
	}
}

// Where the module is also the antenna switch, failing to assert the line does
// not merely cost gain - the path is not connected, and what leaks past is far
// below anything that closes a link.
func TestAnUnswitchedAntennaPathLosesFarMoreThanGain(t *testing.T) {
	n := board(6, 22, &scenario.FEM{TxGainDB: 0, TxLossDB: 25})

	out, _, _ := effectiveRF(n, configured(firmware.RxGainBoosted, 22, firmware.FemOut))
	if out != -3 {
		t.Fatalf("path not switched: %v dBm, want 22 - 25", out)
	}
}

// The frozen-transmission regression: a node's own radio change must not
// evict every other pair's cached path loss.
//
// pathLoss's cache exists because profiling a pair against terrain is the
// expensive part of this engine - "the lazy cache fill walked forty-five
// thousand pairs of DEM samples on the frame thread" is its own comment about
// what skipping it costs. ApplyRadioState used to drop the whole cache on any
// change, and FemAtTx/the AGC-boost register change right after a node
// transmits, so on a many-node scenario that turned every transmission into
// that same frozen walk. Only pairs the changed node is party to may be
// dropped.
func TestApplyRadioStateOnlyInvalidatesThisNodesPairs(t *testing.T) {
	e := New(flatEarth{}, Config{StepMs: 10})
	defer func() { _ = e.Close() }()
	e.Add(scenario.Node{Name: "a", TxPowerDBm: 22, NoiseFigureDB: 6}, nil)
	e.Add(scenario.Node{Name: "b", TxPowerDBm: 22, NoiseFigureDB: 6}, nil)
	e.Add(scenario.Node{Name: "c", TxPowerDBm: 22, NoiseFigureDB: 6}, nil)

	// A full loss and a culled underestimate for node 0, and an untouched
	// pair elsewhere: a radio report may only re-price what read its
	// figures, which is the culled entry alone. Dropping full losses too is
	// how 310 booting nodes stuttered the first minute of every run.
	e.linkCache[[2]int{0, 1}] = 111
	e.linkCache[[2]int{0, 2}] = 222
	e.culled[[2]int{0, 2}] = true
	e.linkCache[[2]int{1, 2}] = 333
	e.emitterNoise[0] = -50
	e.emitterNoise[1] = -60
	e.emitterNoise[2] = -70

	// Boosted gain moves node 0's effective noise figure off its board value,
	// which is what makes ApplyRadioState take the invalidating branch at all.
	e.ApplyRadioState(0, configured(firmware.RxGainBoosted, 22, firmware.FemUnknown))

	if _, ok := e.linkCache[[2]int{1, 2}]; !ok {
		t.Fatal("a pair not touching the changed node was evicted")
	}
	if _, ok := e.linkCache[[2]int{0, 1}]; !ok {
		t.Fatal("the changed node's full loss was evicted; propagation does not read a FEM bit")
	}
	if _, ok := e.linkCache[[2]int{0, 2}]; ok {
		t.Fatal("the changed node's culled pair survived; its verdict reads the figures")
	}
	if _, ok := e.emitterNoise[1]; !ok {
		t.Fatal("another node's emitter-noise figure was evicted")
	}
	if _, ok := e.emitterNoise[2]; !ok {
		t.Fatal("another node's emitter-noise figure was evicted")
	}
	if _, ok := e.emitterNoise[0]; ok {
		t.Fatal("the changed node's own emitter-noise figure survived")
	}
}

// 0 dBm is a level a radio can be set to, so it cannot also mean "has not said".
// A chip that has not been given SetTxParams yet must fall back to the board.
func TestATransmitPowerNobodySetFallsBackToTheBoard(t *testing.T) {
	n := board(6, 22, nil)
	tx, _, ok := effectiveRF(n, configured(firmware.RxGainBoosted, txPowerUnset, firmware.FemUnknown))
	if !ok || tx != 22 {
		t.Fatalf("unset transmit power gave %v dBm, want the board's 22", tx)
	}

	tx, _, _ = effectiveRF(n, configured(firmware.RxGainBoosted, 0, firmware.FemUnknown))
	if tx != 0 {
		t.Fatalf("a radio set to 0 dBm reported %v; 0 is a real setting", tx)
	}
}
