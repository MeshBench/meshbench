package workbench

import (
	"strings"
	"testing"

	"github.com/MeshBench/meshbench/internal/app/state"
)

func colNamed(title string) armCol {
	for _, c := range armCols {
		if c.title == title {
			return c
		}
	}
	panic("no column called " + title)
}

// An arm that did not run draws as a dash, not as a hundred per cent down.
//
// Every counter of a failed cell is zero because it never got as far as
// counting, and the matrix drew those zeros against the baseline: an arm whose
// firmware would not build came out as the largest regression in the table.
func TestAnArmThatDidNotRunDrawsAsADash(t *testing.T) {
	th := linkTestTheme()
	base := state.ArmSummary{Arm: "1.7.0", Runs: 2, TX: 90, RX: 300, Delivered: 30}
	dead := state.ArmSummary{Arm: "1.7.1", Runs: 0, Failed: 2}

	for _, title := range []string{"tx", "rx", "delivered", "collisions", "seed spread"} {
		got, colour := armCell(th, colNamed(title), dead, base, false)
		if got != "-" {
			t.Errorf("the %s of an arm that did not run reads %q", title, got)
		}
		if colour != th.P.Faint {
			t.Errorf("the %s of an arm that did not run is not drawn as absent", title)
		}
	}

	// And the row says what happened, in the column where a bare count was.
	said, colour := armCell(th, colNamed("seeds"), dead, base, false)
	if !strings.Contains(said, "2 failed") {
		t.Errorf("the seeds column reads %q and does not name the failures", said)
	}
	if colour != th.P.Warn {
		t.Error("an arm with failures is not marked as one")
	}
}

// An arm that partly ran keeps its numbers, and still says what it lost.
func TestAnArmThatPartlyRanKeepsItsNumbers(t *testing.T) {
	th := linkTestTheme()
	base := state.ArmSummary{Arm: "1.7.0", Runs: 3, TX: 90, RX: 300, Delivered: 30}
	part := state.ArmSummary{Arm: "1.7.1", Runs: 2, Failed: 1, TX: 90, RX: 300, Delivered: 30}

	got, _ := armCell(th, colNamed("rx"), part, base, false)
	if got != "+0.0%" {
		t.Errorf("an arm that heard exactly what the baseline heard reads %q", got)
	}
	said, _ := armCell(th, colNamed("seeds"), part, base, false)
	if !strings.Contains(said, "2 ran") || !strings.Contains(said, "1 failed") {
		t.Errorf("the seeds column reads %q", said)
	}
}

// A baseline that did not run is nothing to be a percentage of, so the arms
// after it show what they measured rather than a delta against absent zeros.
func TestABaselineThatDidNotRunLeavesTheRestAbsolute(t *testing.T) {
	th := linkTestTheme()
	base := state.ArmSummary{Arm: "1.7.0", Runs: 0, Failed: 2}
	ran := state.ArmSummary{Arm: "1.7.1", Runs: 2, TX: 90, RX: 300, Delivered: 30}
	if got, _ := armCell(th, colNamed("rx"), ran, base, false); got != "300" {
		t.Errorf("against a baseline that did not run the arm reads %q", got)
	}
}

// Red is worse in both directions, which means the two columns where more is
// better have to be painted the other way round.
func TestMoreIsBetterIsPaintedTheOtherWayRound(t *testing.T) {
	th := linkTestTheme()
	if c := deltaColour(th, +5, +1); c != th.P.Good {
		t.Error("delivering more was painted as a regression")
	}
	if c := deltaColour(th, +5, -1); c != th.P.Bad {
		t.Error("spending more airtime was painted as an improvement")
	}
	if c := deltaColour(th, 0.1, -1); c != th.P.Dim {
		t.Error("a change smaller than a rounding error was painted as one")
	}
}
