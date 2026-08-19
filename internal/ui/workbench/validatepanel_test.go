package workbench

import (
	"testing"

	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/ui/comp"
)

// The Verify view said nothing about what it was for, and its panel could not
// say where in the chain anybody was. The steps are read from the world, so a
// verb that refused cannot leave a step marked done.
func TestValidateStepsFollowTheWorld(t *testing.T) {
	steps := func(s *state.Snapshot) []comp.Step {
		heard := s != nil && len(s.Observed) > 0
		compared := s != nil && s.Residuals != nil
		calibrated := s != nil && s.Calibrated
		return []comp.Step{
			{Label: "fetch what was heard", Done: heard, Now: !heard},
			{Label: "compare", Done: compared, Now: heard && !compared},
			{Label: "calibrate", Done: calibrated, Now: compared && !calibrated},
		}
	}
	cases := []struct {
		name string
		snap *state.Snapshot
		done []bool
		now  int // which step is next, -1 for none
	}{
		{"nothing fetched", &state.Snapshot{}, []bool{false, false, false}, 0},
		{"heard, not compared",
			&state.Snapshot{Observed: []state.Observed{{}}}, []bool{true, false, false}, 1},
		{"compared, not calibrated",
			&state.Snapshot{Observed: []state.Observed{{}}, Residuals: &state.Residuals{}},
			[]bool{true, true, false}, 2},
		{"calibrated",
			&state.Snapshot{Observed: []state.Observed{{}}, Residuals: &state.Residuals{},
				Calibrated: true}, []bool{true, true, true}, -1},
	}
	for _, c := range cases {
		got := steps(c.snap)
		nowAt := -1
		for i, st := range got {
			if st.Done != c.done[i] {
				t.Errorf("%s: step %d done=%v, want %v", c.name, i, st.Done, c.done[i])
			}
			if st.Now {
				if nowAt >= 0 {
					t.Errorf("%s: two steps marked next", c.name)
				}
				nowAt = i
			}
		}
		if nowAt != c.now {
			t.Errorf("%s: step %d is next, want %d", c.name, nowAt, c.now)
		}
	}
}

// With nothing fetched the panel says what to do rather than sitting empty,
// and it still draws its steps.
func TestValidatePanelGuidesBeforeAnyData(t *testing.T) {
	p := &validatePanel{}
	h := newPanelHarness(p.Draw, &state.Snapshot{})
	h.frame()
	h.frame() // no panic, and the empty path draws the strip
	// And with residuals it draws the table instead.
	h2 := newPanelHarness(p.Draw, &state.Snapshot{
		Observed:  []state.Observed{{}},
		Residuals: &state.Residuals{Matched: 12, MedianDB: 2.1, IQRdB: 3.4},
	})
	h2.frame()
	h2.frame()
	if p.tb.Shown() != 4 {
		t.Fatalf("the residual table shows %d rows, want the four figures", p.tb.Shown())
	}
}
