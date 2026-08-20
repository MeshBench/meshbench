// The Import controls: the order that matters, as buttons in that order.
package workbench

import (
	"gioui.org/layout"
	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/ui/comp"
	"github.com/MeshBench/meshbench/internal/ui/theme"
)

// importControls is the order that matters, as buttons in that order.
type importControls struct {
	bar    actionBar
	url    comp.Field
	place  comp.Field
	area   comp.Button
	fetch  comp.Button
	commit comp.Button
	infer  comp.Button
	apply  comp.Button
	do     Do
	// OnArea looks a place up and offers what it found: a search for "Fife"
	// returns two, and adding by the typed text picks whichever matched
	// first. Areas add up, so Scotland and Ireland is one study.
	OnArea func(query string)
	built  bool
}

func (c *importControls) Draw(t *theme.Theme, gtx layout.Context, s *state.Snapshot) layout.Dimensions {
	if !c.built {
		c.url.Hint = "a CoreScope deployment URL"
		c.url.Editor.SingleLine = true
		c.place.Hint = "a place: Fife, Scotland, Ireland"
		c.place.Editor.SingleLine = true
		// The study area is first because it is the step that decides how
		// much of everything after it there is to do. A national feed
		// committed whole measures every pair of four hundred nodes against
		// the terrain and the buildings - tens of thousands of profiles -
		// and narrowing it afterwards throws that work away rather than
		// avoiding it.
		c.area.Label, c.area.Kind = "1. add area", comp.Primary
		c.fetch.Label, c.fetch.Kind = "2. fetch", comp.Primary
		c.commit.Label, c.commit.Kind = "3. commit", comp.Secondary
		c.infer.Label, c.infer.Kind = "4. read traffic", comp.Secondary
		c.apply.Label, c.apply.Kind = "5. apply regions", comp.Primary
		c.bar.fields = []*comp.Field{&c.url, &c.place}
		c.bar.buttons = []*comp.Button{&c.area, &c.fetch, &c.commit, &c.infer, &c.apply}
		c.bar.note = "numbered because the order matters and every step has been " +
			"skipped: an import with no study area brings in a country when a " +
			"county was wanted, and a mesh with regions inferred but not applied " +
			"transmits everything, relays nothing, and reports no error"
		c.built = true
	}
	if c.area.Click.Clicked(gtx) && c.OnArea != nil {
		// Searched and added here rather than in a panel of its own: an area
		// is chosen for an import, and a separate tab for it reads as an
		// unrelated feature somebody has to know to visit first.
		c.OnArea(fieldText(&c.place))
	}
	if c.fetch.Click.Clicked(gtx) && c.do != nil {
		c.do("import.fetch", map[string]any{"url": fieldText(&c.url)})
	}
	if c.commit.Click.Clicked(gtx) && c.do != nil {
		c.do("import.commit", map[string]any{"strategy": "replace-all"})
	}
	if c.infer.Click.Clicked(gtx) && c.do != nil {
		c.do("infer.run", map[string]any{"hours": 168})
	}
	if c.apply.Click.Clicked(gtx) && c.do != nil {
		c.do("infer.apply", nil)
	}
	return c.bar.layout(t, gtx)
}
