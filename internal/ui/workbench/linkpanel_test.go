package workbench

import (
	"image"
	"testing"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/text"
	"gioui.org/unit"

	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/ui/theme"
	"github.com/MeshBench/meshbench/internal/ui/theme/brandfont"
)

// A link between two nodes whose names are long enough to be the problem.
func longNamedProfile() *state.Profile {
	p := &state.Profile{
		From: "sco-Avon Heritage Trail", To: "sco-tay-moncreiffe",
		DistanceKm: 44.4, AtoB: -3.5, BtoA: -4.1,
		LowM: 0, HighM: 410,
		Verdict: "BLOCKED. Terrain at 15.9 km sits 266 m above the line of " +
			"sight once earth curvature is included. Raising an antenna by about " +
			"that much, or moving to clear it, is what changes this - more power " +
			"will not.",
		Assumed: "ITU-R P.526 knife edges over bare earth",
		Worst:   state.ProfileWorst{DistM: 15900, FresnelPct: -449, Blocked: true},
		Edges: []state.ProfileEdge{
			{DistM: 1200, LossDB: 25.8}, {DistM: 10400, LossDB: 11.3},
			{DistM: 15900, LossDB: 28.9}, {DistM: 18700, LossDB: 27.0},
			{DistM: 22200, LossDB: 17.0}, {DistM: 26000, LossDB: 12.6},
		},
	}
	for i := 0; i <= 100; i++ {
		d := float64(i) * 444
		p.Samples = append(p.Samples, state.ProfileSample{
			DistM: d, GroundM: 100, BulgedM: 100 + float64(i%7)*20,
			LOSm: 120 + d/1000, FresnelM: 40,
		})
	}
	return p
}

func linkTestTheme() *theme.Theme {
	return theme.New(theme.Dark, theme.Default,
		text.NewShaper(text.WithCollection(brandfont.Collection())))
}

func linkTestContext(ops *op.Ops, sz image.Point) layout.Context {
	return layout.Context{
		Ops:         ops,
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Exact(sz),
	}
}

// Docked in a rail, the Link panel drew no chart at all.
//
// The header was one horizontal row, so the pair of names took the width as a
// rigid and the two direction margins wrapped one character per line into a
// header taller than the panel. The chart was the only flexed child and got
// what was left, which was nothing.
func TestTheLinkPanelKeepsItsChartInARail(t *testing.T) {
	th := linkTestTheme()
	p := longNamedProfile()
	for _, sz := range []image.Point{{X: 340, Y: 520}, {X: 1400, Y: 900}} {
		var ops op.Ops
		gtx := linkTestContext(&ops, sz)
		lp := &linkPanel{}
		if h := lp.chartHeight(th, gtx, p); h < gtx.Dp(minChart) {
			t.Errorf("at %dx%d the cut-through gets %d px, under the %v floor",
				sz.X, sz.Y, h, minChart)
		}
		// And the header stays a header: the fault showed as one that had
		// grown taller than the panel it was in.
		if h := heightOf(gtx, lp.header(th, p)); h > sz.Y/3 {
			t.Errorf("at %dx%d the header is %d px of a %d px panel",
				sz.X, sz.Y, h, sz.Y)
		}
	}
}

// A panel with almost no height still draws, and scrolls rather than dropping
// the picture: the whole fault was a chart silently reduced to nothing.
func TestTheLinkPanelSurvivesBeingSquashed(t *testing.T) {
	th := linkTestTheme()
	p := longNamedProfile()
	var ops op.Ops
	gtx := linkTestContext(&ops, image.Pt(300, 120))
	lp := &linkPanel{}
	if h := lp.chartHeight(th, gtx, p); h < gtx.Dp(minChart) {
		t.Errorf("squashed to 120 px the chart takes %d px, under the %v floor",
			h, minChart)
	}
	if d := lp.Draw(th, gtx, &state.Snapshot{LinkProfile: p}); d.Size.X == 0 {
		t.Error("the panel drew nothing at all")
	}
}
