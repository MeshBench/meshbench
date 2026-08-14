package engine

import (
	"testing"

	"github.com/MeshBench/meshbench/internal/firmware"
	"github.com/MeshBench/meshbench/internal/scenario"
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
func configured(gain uint8, txDBm int8, fem bool) firmware.RadioStats {
	return firmware.RadioStats{
		Configured: true, RxGainReg: gain, TxPowerDBm: txDBm, FemEnabled: fem,
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

	_, nf, ok := effectiveRF(n, configured(firmware.RxGainPowerSaving, 22, false))
	if !ok || nf != 6 {
		t.Fatalf("power saving: noise figure = %v, want the board's 6", nf)
	}

	_, nf, ok = effectiveRF(n, configured(firmware.RxGainBoosted, 22, false))
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

	_, before, _ := effectiveRF(n, configured(firmware.RxGainBoosted, 22, false))
	_, after, _ := effectiveRF(n, configured(firmware.RxGainPowerSaving, 22, false))

	if after <= before {
		t.Fatalf("losing boosted gain did not worsen the receiver: %v then %v",
			before, after)
	}
	if got := after - before; got != RxBoostedGainImprovementDB {
		t.Fatalf("sensitivity moved %v dB, want %v", got, RxBoostedGainImprovementDB)
	}
}

// A board with no front-end module is unaffected by the transmit-enable line,
// because it has no such line and whether it is asserted means nothing.
func TestABoardWithNoModuleIgnoresTheEnableLine(t *testing.T) {
	n := board(6, 22, nil)
	on, _, _ := effectiveRF(n, configured(firmware.RxGainBoosted, 14, true))
	off, _, _ := effectiveRF(n, configured(firmware.RxGainBoosted, 14, false))
	if on != off || on != 14 {
		t.Fatalf("no module: enabled %v, disabled %v, want 14 either way", on, off)
	}
}

// The T096 case. The board is compiled for 9 dBm at the chip and reaches about
// 22 at the antenna through a KCT8103L, so a firmware that never switches the
// module in transmits 13 dB down - which is the difference between a link and
// no link, and which the board profile's MaxTxDBm cannot express.
func TestAnUnswitchedFrontEndCostsItsGain(t *testing.T) {
	fem := &scenario.FEM{TxGainDB: 13, TxLossDB: 0}
	n := board(6, 22, fem)

	on, _, _ := effectiveRF(n, configured(firmware.RxGainBoosted, 9, true))
	if on != 22 {
		t.Fatalf("module switched in: %v dBm, want 9 + 13", on)
	}
	off, _, _ := effectiveRF(n, configured(firmware.RxGainBoosted, 9, false))
	if off != 9 {
		t.Fatalf("module not switched in: %v dBm, want the chip's 9", off)
	}
}

// Where the module is also the antenna switch, failing to assert the line does
// not merely cost gain - the path is not connected, and what leaks past is far
// below anything that closes a link.
func TestAnUnswitchedAntennaPathLosesFarMoreThanGain(t *testing.T) {
	fem := &scenario.FEM{TxGainDB: 0, TxLossDB: 25}
	n := board(6, 22, fem)

	off, _, _ := effectiveRF(n, configured(firmware.RxGainBoosted, 22, false))
	if off != -3 {
		t.Fatalf("path not switched: %v dBm, want 22 - 25", off)
	}
}

// 0 dBm is a level a radio can be set to, so it cannot also mean "has not said".
// A chip that has not been given SetTxParams yet must fall back to the board.
func TestATransmitPowerNobodySetFallsBackToTheBoard(t *testing.T) {
	n := board(6, 22, nil)
	tx, _, ok := effectiveRF(n, configured(firmware.RxGainBoosted, txPowerUnset, false))
	if !ok || tx != 22 {
		t.Fatalf("unset transmit power gave %v dBm, want the board's 22", tx)
	}

	tx, _, _ = effectiveRF(n, configured(firmware.RxGainBoosted, 0, false))
	if tx != 0 {
		t.Fatalf("a radio set to 0 dBm reported %v; 0 is a real setting", tx)
	}
}
