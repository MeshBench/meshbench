package experiment

import (
	"strings"
	"testing"

	"github.com/MeshBench/meshbench/internal/app/session"
)

// The report told somebody to add seeds when every seed had returned the same
// numbers. A cell runs the calculated model - engine.Config's zero RFMode -
// and nothing on that path is drawn per reception: the noise floor is the
// thermal figure for a bandwidth and a noise figure, and MeshCore's own relay
// delay is a function of the score and the airtime with no RNG in it. So the
// seed reaches every node correctly and has nothing to perturb, and the advice
// sent people to spend machine time on a number that cannot move.
func TestTheAdviceDoesNotSendSomebodyToAddSeeds(t *testing.T) {
	if strings.Contains(seedsCannotSeparate, "Add senders or seeds") {
		t.Error("the advice still offers seeds as a remedy")
	}
	if !strings.Contains(seedsCannotSeparate, "Add senders") {
		t.Errorf("the advice does not name what does help: %q", seedsCannotSeparate)
	}
	if !strings.Contains(seedsCannotSeparate, "calculated model") {
		t.Errorf("the advice does not say why seeds cannot help: %q", seedsCannotSeparate)
	}
}

// One draw is reported as one draw, ahead of anything about spreads.
func TestOneSeedIsReportedAsOneDraw(t *testing.T) {
	e := &experiment{
		Seeds:   []uint64{9001},
		Arms:    []session.ExpArm{{Label: "a"}, {Label: "b"}},
		results: []Result{{Arm: "a"}, {Arm: "b"}},
	}
	if got := e.notAResultYet(); !strings.Contains(got, "one draw") {
		t.Errorf("one seed reported as %q", got)
	}
}
