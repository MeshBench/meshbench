package planning_test

import (
	"strings"
	"testing"

	"github.com/MeshBench/meshbench/internal/study/planning"
)

// A degree square of Scotland at a metre of step: the sample grid alone is
// twelve billion points, and the greedy search walks it once per candidate per
// site it places. Nothing about the request looks wrong until the machine
// stops, so the refusal has to name the step that would work.
func bigArea() []planning.LatLon {
	return []planning.LatLon{{Lat: 56, Lon: -4}, {Lat: 57, Lon: -4},
		{Lat: 57, Lon: -3}, {Lat: 56, Lon: -3}}
}

func TestAFineStepOverALargeAreaIsRefused(t *testing.T) {
	_, err := planning.CoverArea(bigArea(), hills{}, rangeChecker{km: 8},
		planning.CoverOptions{MaxNew: 2, SampleStep: 0.00001, CandidateStep: 0.05})
	if err == nil {
		t.Fatal("a twelve-billion-point sample grid was attempted")
	}
	if !strings.Contains(err.Error(), "limit") || !strings.Contains(err.Error(), "at least") {
		t.Errorf("the refusal names neither the limit nor a step that fits: %v", err)
	}

	// The candidate scan holds every point it visits in order to sort them by
	// height, so capping the sixty it returns caps nothing that matters.
	if _, err := planning.CoverArea(bigArea(), hills{}, rangeChecker{km: 8},
		planning.CoverOptions{MaxNew: 2, SampleStep: 0.05, CandidateStep: 0.00001}); err == nil {
		t.Error("a twelve-billion-point candidate scan was attempted")
	}

	// The same question of the baseline, which samples the area for itself.
	if _, err := planning.BaselineCoverage(bigArea(), hills{}, rangeChecker{km: 8},
		planning.CoverOptions{SampleStep: 0.00001}); err == nil {
		t.Error("BaselineCoverage attempted the same grid")
	}
}

// The bridge search scans a corridor the same way, and an unbounded scan there
// is the same afternoon.
func TestAFineStepOverALongCorridorIsRefused(t *testing.T) {
	_, err := planning.Bridge(
		planning.Site{Name: "a", Lat: 56, Lon: -4, HeightAGLm: 10},
		planning.Site{Name: "b", Lat: 57, Lon: -3, HeightAGLm: 10},
		hills{}, rangeChecker{km: 8},
		planning.BridgeOptions{MaxNew: 2, CandidateStep: 0.00001})
	if err == nil {
		t.Fatal("an unbounded corridor scan was attempted")
	}
	if !strings.Contains(err.Error(), "limit") {
		t.Errorf("the refusal does not name a limit: %v", err)
	}
}

// A step that a real plan uses is not caught by the cap: the point is to
// refuse the request nobody meant, not to narrow the ones they did.
func TestAWorkableStepIsNotRefused(t *testing.T) {
	placements, err := planning.CoverArea(bigArea(), hills{}, rangeChecker{km: 12},
		planning.CoverOptions{MaxNew: 2, SampleStep: 0.05, CandidateStep: 0.1})
	if err != nil {
		t.Fatalf("a 0.05 degree step over a degree square was refused: %v", err)
	}
	if len(placements) == 0 {
		t.Error("no sites proposed over an area with room for them")
	}
}
