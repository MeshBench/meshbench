package experiment

import (
	"testing"

	"github.com/MeshBench/meshbench/internal/app/session"
)

func armNamed(sums []map[string]any, name string) map[string]any {
	for _, s := range sums {
		if s["arm"] == name {
			return s
		}
	}
	return nil
}

// A seed that failed counted nothing, and is left out of its arm's mean.
//
// It used to be averaged in as a zero, so one failure of three pulled an arm's
// receptions down by a third and the matrix drew that as a regression the
// firmware had caused.
func TestAFailedSeedIsLeftOutOfItsArmsMean(t *testing.T) {
	e := &experiment{results: []Result{
		{Arm: "1.7.0", Seed: 1, TX: 90, RX: 300, Delivered: 30},
		{Arm: "1.7.0", Seed: 2, TX: 90, RX: 300, Delivered: 30},
		{Arm: "1.7.0", Seed: 3, Err: "the node never came up"},
	}}
	got := armNamed(e.summarise(), "1.7.0")
	if got == nil {
		t.Fatal("the arm is not in the summary at all")
	}
	if n, _ := got["runs"].(int); n != 2 {
		t.Errorf("the arm claims %d runs, having measured 2", n)
	}
	if n, _ := got["failed"].(int); n != 1 {
		t.Errorf("the arm reports %d failures, having had 1", n)
	}
	if rx, _ := got["rx"].(float64); rx != 300 {
		t.Errorf("receptions average %.0f, and both cells that ran heard 300", rx)
	}
}

// An arm no seed got through has no numbers, rather than zero of everything.
//
// Zero is a measurement. Drawn as one it reads as the worst result the sweep
// has ever produced, which is the opposite of what happened.
func TestAnArmNoSeedGotThroughHasNoNumbers(t *testing.T) {
	e := &experiment{results: []Result{
		{Arm: "1.7.0", Seed: 1, TX: 90, RX: 300, Delivered: 30},
		{Arm: "1.7.1", Seed: 1, Err: "the firmware would not build"},
		{Arm: "1.7.1", Seed: 2, Err: "the firmware would not build"},
	}}
	got := armNamed(e.summarise(), "1.7.1")
	if got == nil {
		t.Fatal("an arm that failed outright vanished from the summary")
	}
	if n, _ := got["runs"].(int); n != 0 {
		t.Errorf("the arm claims %d runs", n)
	}
	if n, _ := got["failed"].(int); n != 2 {
		t.Errorf("the arm reports %d failures, having had 2", n)
	}
	for _, k := range []string{"tx", "rx", "delivered", "redundant", "collisions",
		"airtime_ms", "rx_spread"} {
		if _, present := got[k]; present {
			t.Errorf("%q is reported for an arm that measured nothing", k)
		}
	}
}

// And the sentence on the front of it does not announce the failure as the
// sweep's finding: a -100% on an arm that never ran was the largest change in
// the table, so it won.
func TestTheVerdictDoesNotReportAnArmThatDidNotRun(t *testing.T) {
	e := &experiment{
		Arms:  []session.ExpArm{{Label: "1.7.0"}, {Label: "1.7.1"}},
		Seeds: []uint64{1, 2},
		results: []Result{
			{Arm: "1.7.0", Seed: 1, TX: 90, RX: 300, Delivered: 30},
			{Arm: "1.7.0", Seed: 2, TX: 92, RX: 306, Delivered: 30},
			{Arm: "1.7.1", Seed: 1, Err: "the firmware would not build"},
			{Arm: "1.7.1", Seed: 2, Err: "the firmware would not build"},
		},
	}
	if v := e.verdict(); v != "No difference. Every arm produced the same numbers." {
		t.Errorf("the verdict reads %q, and the only arm it could compare did not run", v)
	}
	// The failure is still said, in the place that says what is wrong with the
	// numbers rather than in the numbers themselves.
	if w := e.notAResultYet(); w == "" {
		t.Error("half the sweep failed and nothing warned about it")
	}
}
