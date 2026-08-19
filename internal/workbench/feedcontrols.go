// The Feed controls: replaying the real network's traffic into the simulation.
package workbench

import (
	"gioui.org/layout"
	"github.com/MeshBench/meshbench/internal/gui/comp"
	"github.com/MeshBench/meshbench/internal/gui/state"
	"github.com/MeshBench/meshbench/internal/gui/theme"
)

// feedControls replays the real network's traffic into the simulation.
type feedControls struct {
	bar   actionBar
	url   comp.Field
	start comp.Button
	stop  comp.Button
	do    Do
	built bool
}

func (c *feedControls) Draw(t *theme.Theme, gtx layout.Context, s *state.Snapshot) layout.Dimensions {
	if !c.built {
		c.url.Hint = "a CoreScope deployment URL"
		c.url.Editor.SingleLine = true
		c.start.Label, c.start.Kind = "start live feed", comp.Primary
		c.stop.Label, c.stop.Kind = "stop", comp.Quiet
		c.bar.fields = []*comp.Field{&c.url}
		c.bar.buttons = []*comp.Button{&c.start, &c.stop}
		c.bar.note = "packets are taken at their first hop and re-transmitted by the " +
			"same-named node here, so what you watch is the simulated mesh relaying real traffic"
		c.built = true
	}
	if c.start.Click.Clicked(gtx) && c.do != nil {
		c.do("feed.pull", map[string]any{"url": fieldText(&c.url)})
	}
	if c.stop.Click.Clicked(gtx) && c.do != nil {
		c.do("feed.stop", nil)
	}
	return c.bar.layout(t, gtx)
}
