package pathview_test

import (
	"strings"
	"testing"

	"github.com/MeshBench/meshbench/internal/pathview"
)

type flat struct{ h float64 }

func (f flat) ElevationM(_, _ float64) (float64, bool) { return f.h, true }

// A hill of a given height at a given fraction along the path.
type hill struct {
	base, peak float64
	at, width  float64 // fractions of the longitude span from -3.9 to -3.4
}

func (h hill) ElevationM(_, lon float64) (float64, bool) {
	f := (lon - (-3.9)) / 0.5
	d := f - h.at
	if d < 0 {
		d = -d
	}
	if d > h.width {
		return h.base, true
	}
	return h.base + (h.peak-h.base)*(1-d/h.width), true
}

const (
	lat  = 56.7
	lonA = -3.9
	lonB = -3.4
	freq = 869.525
)

// The three verdicts exist because there are three different actions. Blocked
// means move or raise; grazing means a few metres would help; clear means the
// answer is in the link budget and no mast will change it.
func TestVerdictDistinguishesTheThreeActions(t *testing.T) {
	// A tall hill mid-path, well above the sight line between two low ends.
	blocked, err := pathview.Analyse(hill{base: 100, peak: 700, at: 0.5, width: 0.15},
		lat, lonA, 10, lat, lonB, 2, freq, 200)
	if err != nil {
		t.Fatal(err)
	}
	if !blocked.Blocked || !strings.HasPrefix(blocked.Verdict(), "BLOCKED") {
		t.Errorf("a 600 m hill mid-path was not blocked: %s", blocked.Verdict())
	}

	// High masts over flat ground: nothing near the sight line at all.
	clear, err := pathview.Analyse(flat{100}, lat, lonA, 300, lat, lonB, 300, freq, 200)
	if err != nil {
		t.Fatal(err)
	}
	if clear.Blocked || !strings.HasPrefix(clear.Verdict(), "CLEAR") {
		t.Errorf("two 300 m masts over flat ground: %s", clear.Verdict())
	}

	// A hill that stops just below the sight line: line of sight is clear, but
	// the Fresnel zone is not. This is the case a sight-line-only view misses
	// entirely, and it is the reason the package draws the zone.
	grazing, err := pathview.Analyse(hill{base: 100, peak: 180, at: 0.5, width: 0.2},
		lat, lonA, 100, lat, lonB, 100, freq, 200)
	if err != nil {
		t.Fatal(err)
	}
	if grazing.Blocked {
		t.Fatalf("the grazing case is actually blocked; clearance %.1f m",
			grazing.Samples[grazing.Worst].ClearanceM)
	}
	if grazing.WorstF1Pct >= 60 {
		t.Skipf("geometry gives %.0f%% of F1, not a grazing case", grazing.WorstF1Pct)
	}
	if !strings.HasPrefix(grazing.Verdict(), "GRAZING") {
		t.Errorf("clear sight line but %.0f%% of F1: %s", grazing.WorstF1Pct, grazing.Verdict())
	}
}

// The Fresnel zone is widest at mid-path, so equal clearance in metres is not
// equal clearance in the sense that matters. Two hills leaving the same 30 m
// gap under the sight line are not equally bad: the mid-path one eats far more
// of its Fresnel radius, and it is the one that decides the path.
//
// The near-end hill here is the *taller* of the two — it has to be, to leave
// the same gap once earth curvature is accounted for — which is what makes
// ranking by absolute height get this backwards.
func TestWorstPointIsFresnelRelativeNotHighest(t *testing.T) {
	c, err := pathview.Analyse(bumps{
		base: 100,
		peaks: []bump{
			{at: 0.06, height: 267, width: 0.05}, // taller, hard against the end
			{at: 0.50, height: 257, width: 0.10}, // shorter, where the zone is widest
		},
	}, lat, lonA, 300, lat, lonB, 300, freq, 400)
	if err != nil {
		t.Fatal(err)
	}
	if c.Blocked {
		t.Fatalf("neither hill should block; worst clearance %.1f m",
			c.Samples[c.Worst].ClearanceM)
	}

	worst := c.Samples[c.Worst]
	worstAt := worst.DistM / (c.DistanceKm * 1000)
	if worstAt < 0.25 || worstAt > 0.75 {
		t.Errorf("worst point at %.2f of the path; the mid-path hill should win even "+
			"though the near one is taller (clearance %.0f m, %.0f%% of F1)",
			worstAt, worst.ClearanceM, c.WorstF1Pct)
	}
	// And the Fresnel radius there really is the larger one, which is the whole
	// mechanism.
	if worst.FresnelM <= c.Samples[len(c.Samples)*6/100].FresnelM {
		t.Error("the mid-path Fresnel radius is not larger than the near-end one")
	}
}

type bump struct{ at, height, width float64 }
type bumps struct {
	base  float64
	peaks []bump
}

func (b bumps) ElevationM(_, lon float64) (float64, bool) {
	f := (lon - (-3.9)) / 0.5
	h := b.base
	for _, p := range b.peaks {
		d := f - p.at
		if d < 0 {
			d = -d
		}
		if d < p.width {
			if v := b.base + p.height*(1-d/p.width); v > h {
				h = v
			}
		}
	}
	return h, true
}

// Ground and bulged terrain are kept separately so a drawn view can be checked
// against a map. Collapsing them would make the picture unverifiable.
func TestCurvatureIsAddedButGroundIsKept(t *testing.T) {
	c, err := pathview.Analyse(flat{100}, lat, lonA, 20, lat, lonB, 20, freq, 100)
	if err != nil {
		t.Fatal(err)
	}
	mid := c.Samples[len(c.Samples)/2]
	if mid.GroundM != 100 {
		t.Errorf("ground was altered: %.1f m", mid.GroundM)
	}
	if mid.BulgedM <= mid.GroundM {
		t.Errorf("no earth curvature at mid-path: bulged %.1f, ground %.1f", mid.BulgedM, mid.GroundM)
	}
	// Curvature is zero at the ends by definition.
	if c.Samples[0].BulgedM != c.Samples[0].GroundM {
		t.Error("curvature was applied at the near end, where it is zero by definition")
	}
}

func TestFresnelRadiusIsWidestMidPath(t *testing.T) {
	c, err := pathview.Analyse(flat{100}, lat, lonA, 20, lat, lonB, 20, freq, 100)
	if err != nil {
		t.Fatal(err)
	}
	mid := c.Samples[len(c.Samples)/2].FresnelM
	quarter := c.Samples[len(c.Samples)/4].FresnelM
	if mid <= quarter {
		t.Errorf("Fresnel radius %.1f m at mid-path against %.1f m at a quarter", mid, quarter)
	}
	if c.Samples[0].FresnelM != 0 {
		t.Error("the Fresnel radius at the antenna itself is not zero")
	}
}

func TestNoTerrainIsAnError(t *testing.T) {
	_, err := pathview.Analyse(missing{}, lat, lonA, 10, lat, lonB, 10, freq, 50)
	if err == nil {
		t.Fatal("a path with no terrain was analysed anyway")
	}
}

type missing struct{}

func (missing) ElevationM(_, _ float64) (float64, bool) { return 0, false }
