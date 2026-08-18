// The panels that bring a network in or check it: import, boundary and
// validate.
package workbench

import (
	"gioui.org/layout"
	"github.com/MeshBench/meshbench/internal/gui/comp"
	"github.com/MeshBench/meshbench/internal/gui/state"
	"github.com/MeshBench/meshbench/internal/gui/theme"
)

// importControls is the order that matters, as buttons in that order.
type importControls struct {
	bar    actionBar
	url    comp.Field
	fetch  comp.Button
	commit comp.Button
	infer  comp.Button
	apply  comp.Button
	do     Do
	built  bool
}

func (c *importControls) Draw(t *theme.Theme, gtx layout.Context, s *state.Snapshot) layout.Dimensions {
	if !c.built {
		c.url.Hint = "a CoreScope deployment URL"
		c.url.Editor.SingleLine = true
		c.fetch.Label, c.fetch.Kind = "1. fetch", comp.Primary
		c.commit.Label, c.commit.Kind = "2. commit", comp.Secondary
		c.infer.Label, c.infer.Kind = "3. read traffic", comp.Secondary
		c.apply.Label, c.apply.Kind = "4. apply regions", comp.Primary
		c.bar.fields = []*comp.Field{&c.url}
		c.bar.buttons = []*comp.Button{&c.fetch, &c.commit, &c.infer, &c.apply}
		c.bar.note = "numbered because the order matters and every step has been " +
			"skipped: a mesh with regions inferred but not applied transmits " +
			"everything, relays nothing, and reports no error"
		c.built = true
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

// boundaryControls chooses the study area.
//
// Two ways in, because Nominatim only covers what somebody has published a
// boundary for - a country, a council, a national park - and most of what a
// study needs to target is smaller than that. geojsonPath is the other one:
// a file drawn anywhere (geojson.io, QGIS, a council's open-data portal)
// becomes an area the same way an accepted search result does, through
// scenario.ParseGeoJSON either way.
type boundaryControls struct {
	bar         actionBar
	place       comp.Field
	margin      comp.Field
	geojsonPath comp.Field
	search      comp.Button
	accept      comp.Button
	prune       comp.Button
	importGeo   comp.Button
	do          Do
	built       bool
}

func (c *boundaryControls) Draw(t *theme.Theme, gtx layout.Context, s *state.Snapshot) layout.Dimensions {
	if !c.built {
		c.place.Hint = "a place: Fife, Scotland, Perth and Kinross"
		c.margin.Hint = "margin km"
		c.geojsonPath.Hint = "path to a .geojson file - for areas too small to search for"
		c.place.Editor.SingleLine = true
		c.margin.Editor.SingleLine = true
		c.geojsonPath.Editor.SingleLine = true
		c.search.Label, c.search.Kind = "search", comp.Primary
		c.accept.Label, c.accept.Kind = "accept it", comp.Secondary
		c.prune.Label, c.prune.Kind = "delete what is outside", comp.Destructive
		c.importGeo.Label, c.importGeo.Kind = "import it", comp.Secondary
		c.bar.fields = []*comp.Field{&c.place, &c.margin, &c.geojsonPath}
		c.bar.buttons = []*comp.Button{&c.search, &c.accept, &c.prune, &c.importGeo}
		c.bar.note = "the study area decides which nodes are measured, not which " +
			"packets are forwarded - the margin keeps what interferes from outside it. " +
			"Nominatim only knows places somebody has published a boundary for; " +
			"anything smaller is a GeoJSON file"
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
	if c.importGeo.Click.Clicked(gtx) && c.do != nil {
		c.do("boundary.import", map[string]any{"path": fieldText(&c.geojsonPath)})
	}
	return c.bar.layout(t, gtx)
}

// validateControls is the calibration chain.
type validateControls struct {
	bar   actionBar
	hours comp.Field
	db    comp.Field
	fetch comp.Button
	cal   comp.Button
	uncal comp.Button
	do    Do
	built bool
}

func (c *validateControls) Draw(t *theme.Theme, gtx layout.Context, s *state.Snapshot) layout.Dimensions {
	if !c.built {
		c.hours.Hint = "hours to look back"
		c.db.Hint = "excess loss dB (blank: what was measured)"
		c.hours.Editor.SingleLine = true
		c.db.Editor.SingleLine = true
		c.fetch.Label, c.fetch.Kind = "fetch and compare", comp.Primary
		c.cal.Label, c.cal.Kind = "apply calibration", comp.Secondary
		c.uncal.Label, c.uncal.Kind = "back to the default", comp.Quiet
		c.bar.fields = []*comp.Field{&c.hours, &c.db}
		c.bar.buttons = []*comp.Button{&c.fetch, &c.cal, &c.uncal}
		c.bar.note = "positive residual means the model predicted more signal than " +
			"was heard, so it is optimistic and the excess loss should go up"
		c.built = true
	}
	if c.fetch.Click.Clicked(gtx) && c.do != nil {
		p := map[string]any{}
		if v, ok := num(&c.hours); ok {
			p["hours"] = v
		}
		c.do("validate.fetch", p)
	}
	if c.cal.Click.Clicked(gtx) && c.do != nil {
		p := map[string]any{}
		if v, ok := num(&c.db); ok {
			p["db"] = v
		}
		c.do("validate.calibrate", p)
	}
	if c.uncal.Click.Clicked(gtx) && c.do != nil {
		c.do("validate.uncalibrate", nil)
	}
	return c.bar.layout(t, gtx)
}

func splitFields(s string) []string {
	var out []string
	out = append(out, fieldsOf(s)...)
	return out
}

func fieldsOf(s string) []string {
	var out, cur []rune
	_ = out
	var res []string
	for _, r := range s {
		if r == ' ' || r == ',' || r == '\t' {
			if len(cur) > 0 {
				res = append(res, string(cur))
				cur = cur[:0]
			}
			continue
		}
		cur = append(cur, r)
	}
	if len(cur) > 0 {
		res = append(res, string(cur))
	}
	return res
}
