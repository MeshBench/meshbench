// The Validate panel: the model against reality.
//
// It says which way the model is wrong and by how much, because that is
// something somebody can act on; "validation failed" is not.
package workbench

import (
	"fmt"

	"gioui.org/layout"

	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/ui/comp"
	"github.com/MeshBench/meshbench/internal/ui/theme"
)

// validatePanel is the model against reality.
//
// Residuals, not a verdict. "The model is 3 dB optimistic on this network" is
// something somebody can act on; "validation failed" is not.
type validatePanel struct {
	tb   comp.Table
	init bool
}

func (p *validatePanel) Draw(t *theme.Theme, gtx layout.Context, s *state.Snapshot) layout.Dimensions {
	if !p.init {
		p.tb.Cols = []comp.Column{
			{Title: "", Width: 260},
			{Title: "value", Width: 140, Right: true, Mono: true},
			{Title: "what it says"},
		}
		p.init = true
	}
	if s == nil || s.Residuals == nil {
		return layout.Center.Layout(gtx, comp.Text(t, t.Sz.Body, t.P.Dim,
			"pull the live feed first - residuals need something real to be residuals against"))
	}
	r := s.Residuals
	rows := []comp.Row{
		{Key: "pairs", Cells: []string{"observed pairs matched", fmt.Sprintf("%d", r.Matched),
			"receptions between two nodes this scenario also has"}},
		// The two ways an observation fails to match are different problems
		// with different fixes, and their sum shown as one number is how a
		// total matching failure once went undiagnosed for weeks.
		{Key: "offscenario", Cells: []string{"named a node not in this scenario",
			fmt.Sprintf("%d", r.OffScenario),
			"the real network is bigger than this import - widen the region if these matter"}},
		{Key: "nolink", Cells: []string{"no measured link for the pair",
			fmt.Sprintf("%d", r.NoLink),
			"both nodes are here but the engine has not priced the path - let the links finish warming"}},
		{Key: "median", Cells: []string{"median residual", fmt.Sprintf("%+.1f dB", r.MedianDB),
			"positive means the model predicts more margin than was observed"}},
		{Key: "spread", Cells: []string{"half the residuals within",
			fmt.Sprintf("%.1f dB", r.IQRdB),
			"the interquartile range; a wide one means the model is inconsistent, not merely wrong"}},
	}
	p.tb.SetRows(rows)
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(comp.SectionTitle(t, "the model against reality")),
		layout.Rigid(comp.Text(t, t.Sz.Caption, t.P.Dim,
			"residuals, not a verdict: a bias somebody can correct for beats a pass or a fail")),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return p.tb.Layout(t, gtx, nil)
		}),
	)
}
