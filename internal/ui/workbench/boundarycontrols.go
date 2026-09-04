// The Boundary controls: choosing the study area.
package workbench

import (
	"gioui.org/layout"

	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/ui/comp"
	"github.com/MeshBench/meshbench/internal/ui/theme"
)

// boundaryControls chooses the study area.
type boundaryControls struct {
	bar    comp.ActionBar
	margin comp.Field
	prune  comp.Button
	do     Do
	built  bool
}

func (c *boundaryControls) Draw(t *theme.Theme, gtx layout.Context, s *state.Snapshot) layout.Dimensions {
	if !c.built {
		c.margin.Hint = "margin km"
		c.margin.Editor.SingleLine = true
		c.prune.Label, c.prune.Kind = "delete what is outside", comp.Destructive
		c.bar.Fields = []*comp.Field{&c.margin}
		c.bar.Buttons = []*comp.Button{&c.prune}
		// Choosing the areas happens where it is used, in the Import panel:
		// this is what the study area turned out to be, and the one action
		// that is about nodes already loaded from somewhere else.
		c.bar.Note = "the areas themselves are chosen in Import, where they decide " +
			"what is fetched. This deletes nodes already loaded from outside them - " +
			"the margin keeps what interferes from just over the line"
		c.built = true
	}
	if c.prune.Click.Clicked(gtx) && c.do != nil {
		p := map[string]any{}
		if v, ok := comp.Num(&c.margin); ok {
			p["margin_km"] = v
		}
		c.do("boundary.prune", p)
	}
	return c.bar.Layout(t, gtx)
}
