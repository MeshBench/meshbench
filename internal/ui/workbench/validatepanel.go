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
	if s == nil {
		return layout.Dimensions{}
	}
	if s.Residuals == nil {
		// What to do next, and why it is not simply empty. A panel that says
		// only "no data" leaves somebody to guess whether it is broken.
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(validateSteps(t, s)),
			layout.Rigid(layout.Spacer{Height: t.Sp.M}.Layout),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				// Wrapped, not clipped: this panel lives in a 340dp rail, and
				// a sentence that says what to do next is worth nothing
				// ending in an ellipsis.
				return layout.Center.Layout(gtx, comp.Text(t, t.Sz.Body, t.P.Dim,
					"nothing to compare against yet - fetch what the real network "+
						"heard, and the model is measured against it"))
			}),
		)
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
		{Key: "censored", Cells: []string{"predicted past the modem's ceiling",
			fmt.Sprintf("%d", r.Censored),
			"these can only say 'at least this optimistic', so they are counted but do not vote in the fit"}},
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
		layout.Rigid(layout.Spacer{Height: t.Sp.XS}.Layout),
		layout.Rigid(validateSteps(t, s)),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return p.tb.Layout(t, gtx, nil)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			// What the model is running with right now, said where the
			// residuals are read: a residual against an uncalibrated model
			// and one against a fitted model are different numbers.
			line := fmt.Sprintf("running with %.1f dB excess loss, the default",
				s.ExcessLossDB)
			if s.Calibrated {
				line = fmt.Sprintf("running with %.1f dB excess loss, fitted against "+
					"what was heard", s.ExcessLossDB)
			}
			return comp.OneLine(t, t.Sz.Caption, t.P.Faint, line, false)(gtx)
		}),
	)
}

// validateSteps is where the operator is in the chain, read from the world
// rather than from what was last pressed.
func validateSteps(t *theme.Theme, s *state.Snapshot) layout.Widget {
	heard := s != nil && len(s.Observed) > 0
	compared := s != nil && s.Residuals != nil
	calibrated := s != nil && s.Calibrated
	return comp.Steps(t, []comp.Step{
		{Label: "fetch what was heard", Done: heard, Now: !heard},
		{Label: "compare", Done: compared, Now: heard && !compared},
		{Label: "calibrate", Done: calibrated, Now: compared && !calibrated},
	})
}
