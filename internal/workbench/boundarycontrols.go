// The Boundary controls: choosing the study area.
package workbench

import (
	"gioui.org/layout"
	"github.com/MeshBench/meshbench/internal/gui/comp"
	"github.com/MeshBench/meshbench/internal/gui/state"
	"github.com/MeshBench/meshbench/internal/gui/theme"
)

// boundaryControls chooses the study area.
type boundaryControls struct {
	bar    actionBar
	place  comp.Field
	margin comp.Field
	search comp.Button
	accept comp.Button
	prune  comp.Button
	do     Do
	built  bool
}

func (c *boundaryControls) Draw(t *theme.Theme, gtx layout.Context, s *state.Snapshot) layout.Dimensions {
	if !c.built {
		c.place.Hint = "a place: Fife, Scotland, Perth and Kinross"
		c.margin.Hint = "margin km"
		c.place.Editor.SingleLine = true
		c.margin.Editor.SingleLine = true
		c.search.Label, c.search.Kind = "search", comp.Primary
		c.accept.Label, c.accept.Kind = "accept it", comp.Secondary
		c.prune.Label, c.prune.Kind = "delete what is outside", comp.Destructive
		c.bar.fields = []*comp.Field{&c.place, &c.margin}
		c.bar.buttons = []*comp.Button{&c.search, &c.accept, &c.prune}
		c.bar.note = "the study area decides which nodes are measured, not which " +
			"packets are forwarded - the margin keeps what interferes from outside it"
		c.built = true
	}
	if c.search.Click.Clicked(gtx) && c.do != nil {
		c.do("boundary.set", map[string]any{"query": fieldText(&c.place)})
	}
	if c.accept.Click.Clicked(gtx) && c.do != nil {
		c.do("boundary.accept", map[string]any{"name": fieldText(&c.place)})
	}
	if c.prune.Click.Clicked(gtx) && c.do != nil {
		p := map[string]any{}
		if v, ok := num(&c.margin); ok {
			p["margin_km"] = v
		}
		c.do("boundary.prune", p)
	}
	return c.bar.layout(t, gtx)
}
