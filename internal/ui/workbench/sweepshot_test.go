package workbench

import (
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"gioui.org/layout"

	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/ui/theme"
)

// Draw the sweep results with an arm that did not run, and look at it.
//
// Asserting that a cell reads "-" says nothing about whether a reader can tell
// the difference between an arm that failed and one that measured nothing,
// which is the whole question. A firmware that will not build is also not a
// state a live capture can be driven into on demand, so the picture is made
// here.
func TestDrawTheSweepResults(t *testing.T) {
	if os.Getenv("MESHBENCH_SHOTS") == "" {
		t.Skip("set MESHBENCH_SHOTS=<dir> to write the pictures")
	}
	dir := os.Getenv("MESHBENCH_SHOTS")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name string
		snap *state.Snapshot
	}{
		{"every-arm-ran", &state.Snapshot{
			ExperimentVerdict: "No difference worth reporting: the largest change, " +
				"receptions on 1.7.1 by +1.4%, is inside the ±2.1% the seeds vary by " +
				"on their own.",
			Experiment: []state.ArmSummary{
				{Arm: "1.7.0", Runs: 3, TX: 93, RX: 302, Delivered: 31,
					Redundant: 271, Collided: 4, AirtimeMs: 8100, RXSpread: 0.021},
				{Arm: "1.7.1", Runs: 3, TX: 93, RX: 306, Delivered: 31,
					Redundant: 275, Collided: 5, AirtimeMs: 8140, RXSpread: 0.019},
			},
		}},
		{"one-seed-failed", &state.Snapshot{
			ExperimentWarning: "at least one run failed: no published image for " +
				"repeater 1.7.1 on this board",
			Experiment: []state.ArmSummary{
				{Arm: "1.7.0", Runs: 3, TX: 93, RX: 302, Delivered: 31,
					Redundant: 271, Collided: 4, AirtimeMs: 8100, RXSpread: 0.021},
				{Arm: "1.7.1", Runs: 2, Failed: 1, TX: 93, RX: 306, Delivered: 31,
					Redundant: 275, Collided: 5, AirtimeMs: 8140, RXSpread: 0.019},
			},
		}},
		// The one the fault was reported on: an arm whose every seed failed
		// used to draw zeros throughout and read as the worst result the
		// sweep had ever produced.
		{"an-arm-did-not-run", &state.Snapshot{
			ExperimentWarning: "at least one run failed: no published image for " +
				"repeater 9.9.9 on this board",
			Experiment: []state.ArmSummary{
				{Arm: "1.7.0", Runs: 3, TX: 93, RX: 302, Delivered: 31,
					Redundant: 271, Collided: 4, AirtimeMs: 8100, RXSpread: 0.021},
				{Arm: "9.9.9", Runs: 0, Failed: 3},
			},
		}},
	}

	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			p := &sweepResults{}
			img := renderWidget(t, 1100, 260, func(gtx layout.Context, th *theme.Theme) layout.Dimensions {
				return p.Draw(th, gtx, c.snap)
			})
			out := filepath.Join(dir, "sweep-"+c.name+".png")
			f, err := os.Create(out)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = f.Close() }()
			if err := png.Encode(f, img); err != nil {
				t.Fatal(err)
			}
			t.Logf("wrote %s", out)
		})
	}
}
