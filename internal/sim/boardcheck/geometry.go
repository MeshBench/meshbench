// The three nodes a probe measures a board with, and how long it gives them.
package boardcheck

import (
	"time"

	"github.com/MeshBench/meshbench/internal/rf/antenna"
	"github.com/MeshBench/meshbench/internal/world/scenario"
)

// probeGeometry is three nodes: a native sender, the board under test in the
// middle, and a native listener out of the sender's own direct reach - so a
// message the listener receives proves the middle node relayed it, not that
// the sender happened to reach both.
func probeGeometry(board, version string) []scenario.Node {
	mast := antenna.Mounted{Pattern: antenna.Collinear{GainDBiPeak: 6}, Polarisation: "vertical"}
	radio := scenario.RadioConfig{CentreHz: 869.618e6, BandwidthHz: 62_500, SpreadFactor: 8, CodingRate: 4}
	node := func(name string, lat, lon float64, txDBm float64, fw scenario.FirmwareRef) scenario.Node {
		return scenario.Node{
			Name: name, Kind: scenario.SimpleRepeater,
			Position: scenario.LatLon{Lat: lat, Lon: lon}, HeightAGLm: 10,
			Antenna: mast, TxPowerDBm: txDBm, NoiseFigureDB: 6, Radio: radio,
			Firmware: fw,
		}
	}
	// The listener must not hear the sender, and now does not.
	//
	// It used to, which quietly broke the flood phase: three nodes 0.6 degrees
	// of longitude apart put the far pair 73 km from each other, which 20 dBm
	// from a 6 dBi mast covers easily over flat bare earth. So the listener
	// heard every packet directly, relayed it before the board had finished
	// waiting, and the board dropped its own copy - which is what a repeater
	// should do when somebody else has already relayed. The board was behaving
	// correctly and being marked down for it.
	//
	// Distance alone cannot separate the two links: free space costs 6 dB per
	// doubling, so a line of three nodes never puts much between the near hop
	// and the far one. A weak sender does. Measured on the channel rather than
	// estimated: at 2 dBm the sender reached the listener at -1.6 dB SNR, some
	// 8 dB above what SF8 needs, so it comes down another 14 dB and the board
	// moves in to 3 km to keep its own hop strong.
	//
	//	sender -> board       about +12 dB SNR   must work
	//	sender -> listener    about -16 dB SNR   must not
	//	board  -> listener    about  +1 dB SNR   must work
	//
	// A sender this quiet is not a realistic node, and it is not pretending to
	// be one: it is a fixture that puts the board on the only path between two
	// others, which is the whole of what this phase measures.
	return []scenario.Node{
		node("bc-sender", 56.70, -3.90, -12,
			scenario.FirmwareRef{Role: "simple_repeater", Version: nativePeerVersion}),
		node("bc-under-test", 56.70, -3.85, 20,
			scenario.FirmwareRef{Role: "simple_repeater", Version: version, Board: board}),
		node("bc-listener", 56.70, -2.90, 20,
			scenario.FirmwareRef{Role: "simple_repeater", Version: nativePeerVersion}),
	}
}

// nativePeerVersion is the native build the probe's sender and listener run.
const nativePeerVersion = "repeater-v1.17.1"

// advertBudgetMs is how long any phase waits for something to reach the air,
// and it is one number on purpose.
//
// The phases used to differ - 90 s for the first advert, 60 s after an idle -
// and an emulated ESP32 took 68.5 s to produce its first. So the board passed
// the phase with the generous budget and failed the identical act under the
// tighter one, and the matrix recorded "no response after the idle period"
// for a board that was answering perfectly well. A board's second advert
// cannot be held to a shorter deadline than its first, and a relay - which is
// an advert plus a hop - cannot be held to a shorter one than either.
//
// Four minutes, not ninety seconds, since an emulated board got a filesystem
// that works. It formats it on first boot, which is 1.4 MB of flash through an
// emulated SPI controller and takes most of ninety seconds by itself - so the
// old budget was spent before the board had finished starting. The board was
// relaying the whole time and being recorded as a board that would not.
//
// The cost is real: a probe takes minutes rather than one. Measuring the wrong
// thing faster is not a saving.
// A debug build of the firmware is the case this has to bend for. Every
// MESH_DEBUG line goes out of a console on a guest already running several
// times slower than real time, and a board that would advert in ninety seconds
// takes longer than this to reach its first one - so the probe records a board
// that never transmitted, and every row after it goes untested. That is a
// measurement of the budget rather than of the board, and the whole point of
// running a debug build is to be told why a later row fails.
var advertBudgetMs = envMs("MESHBENCH_ADVERT_BUDGET_MS", 240_000)

// floodQuietMs is how long the board must have been off the air before it is
// handed the packet the flood row judges it on.
//
// A repeater's own retransmit delay is a random fraction of the airtime it is
// forwarding - under three seconds here - and its transmission takes about a
// second more. Ten covers both with room to spare, and costs a tenth of the
// budget the row already has.
const floodQuietMs = 10_000

// probeWaitPhases is how many of Probe's phases can each spend the full
// advertBudgetMs before giving up: TX/Radio, RX, the flood-quiet wait, the
// flood relay itself, and the power phase's relay fallback. Kept as a count
// rather than as a second sum written out by hand, so ProbeBudget cannot
// drift from Probe the way the caller's hardcoded timeout already had.
const probeWaitPhases = 5

// probeFixedOverhead covers what Probe spends outside those five waits: the
// image fetch, the boot settle, the fifteen-second idle before the power
// check, and the console round trips - none of them bounded by
// advertBudgetMs, all of them real time a caller must also grant.
const probeFixedOverhead = 5 * time.Minute

// ProbeBudget is the shortest context a caller may hand to Probe.
//
// internal/app/session/board's board.probe verb used to give Probe a flat
// five minutes while a board that failed a phase honestly could need up to
// probeWaitPhases times advertBudgetMs on top of the fixed overhead - so a
// board that was still starting got cut off mid-phase and its capabilities
// recorded as failed for a budget the caller chose, not for anything the
// board did. Deriving it from the same numbers Probe waits on means the two
// cannot disagree again.
func ProbeBudget() time.Duration {
	return time.Duration(probeWaitPhases)*time.Duration(advertBudgetMs)*time.Millisecond +
		probeFixedOverhead
}
