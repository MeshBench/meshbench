// The terms panel: what a resource arrived under, shown against the row that
// asked for it.
//
// In place rather than in a window. Somebody looking at 7 GB of terrain and
// wondering whether they may republish it should not have to lose the row to
// find out, and a licence is read beside the thing it licenses or not at all.
package workbench

import (
	"gioui.org/layout"
	"gioui.org/op"

	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/ui/comp"
	"github.com/MeshBench/meshbench/internal/ui/theme"
)

// licenceShown is whether the terms the world is holding are this row's.
//
// Matched on kind and name rather than on a flag the button sets, so the state
// is reachable from the control socket and can therefore be captured. Terms
// shown under the wrong resource would be worse than none, hence both halves.
func (p *resourcesPanel) licenceShown(r state.ResourceRow) bool {
	return p.licence.Text != "" &&
		p.licence.Kind == r.Kind && p.licence.Name == r.Name
}

// licenceBox draws the terms under the row that asked for them.
func (p *resourcesPanel) licenceBox(t *theme.Theme, gtx layout.Context,
	r state.ResourceRow) layout.Dimensions {
	if !p.licenceShown(r) {
		return layout.Dimensions{}
	}
	return layout.Inset{Top: t.Sp.S}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		// Recorded then painted behind, the card's own trick: the surface has
		// to be the size of the text, and the text has to be measured first.
		macro := op.Record(gtx.Ops)
		dims := layout.UniformInset(t.Sp.M).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(comp.Text(t, t.Sz.Label, t.P.Dim, "Terms")),
				layout.Rigid(layout.Spacer{Height: t.Sp.XS}.Layout),
				layout.Rigid(comp.Text(t, t.Sz.Caption, t.P.Dim, p.licence.Text)),
			)
		})
		call := macro.Stop()
		comp.RoundRect(gtx, dims.Size, t.Sp.S, t.P.Sunk)
		call.Add(gtx.Ops)
		return dims
	})
}
