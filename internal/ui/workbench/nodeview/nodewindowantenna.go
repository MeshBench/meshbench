// The Antenna tab: what this node stands under, and which way it points.
//
// The model has always been able to represent a beam, and nothing in the
// interface could build one, so every node in every scenario was an omni aimed
// nowhere. A hilltop repeater with a sector down a valley is ordinary practice,
// and choosing where to point it is the question this tool exists to answer.
//
// The choice applies the moment it is made rather than waiting for a form to be
// submitted, because the answer is on the map beside the window: turn the beam
// and the drawn pattern turns with it, which is the only feedback that makes
// aiming a thing somebody can do by eye.
package nodeview

import (
	"fmt"
	"strconv"
	"strings"

	"gioui.org/layout"
	"gioui.org/widget"

	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/ui/comp"
	"github.com/MeshBench/meshbench/internal/ui/theme"
)

// antennaSorts are the patterns that can be built, in the order they are worth
// meeting: the reference, the two omnis, then the beam. The tokens are the
// model's own, so what a button says and what the verb takes cannot drift.
var antennaSorts = [4]struct{ label, token string }{
	{"isotropic", "isotropic"},
	{"dipole", "dipole"},
	{"omni", "collinear"},
	{"beam", "yagi"},
}

// antennaPols are the three the model prices. A fourth spelling is refused by
// the verb, so it is not offered here either.
var antennaPols = [3]struct{ label, token string }{
	{"vertical", "vertical"},
	{"horizontal", "horizontal"},
	{"circular", "circular"},
}

// antennaControls is the form: the sort, its numbers, and where it points.
type antennaControls struct {
	sorts [4]comp.Button
	pols  [3]comp.Button

	gain     comp.Field
	beam     comp.Field
	f2b      comp.Field
	bearing  comp.Field
	downtilt comp.Field
	feedline comp.Field
	apply    comp.Button
	applyAll comp.Button

	aimAt comp.Field
	aim   comp.Button

	list widget.List
	// shown is the antenna the boxes were last filled from. A box is refilled
	// when the node's own antenna moves under it - another window, a script, a
	// sweep - and left alone otherwise, so typing into one is not undone by the
	// next frame.
	shown state.Antenna
	built bool
}

// fill puts a node's antenna in the boxes.
//
// The two a beam alone uses are left empty when they are zero rather than
// showing one: a collinear has no beamwidth, and "0 deg" in a box reads as a
// claim about an antenna with an infinitely narrow lobe. Empty is what the verb
// reads as "leave it alone", so the two agree.
func (c *antennaControls) fill(a state.Antenna) {
	set := func(f *comp.Field, v float64) { f.Editor.SetText(number(v)) }
	beamOnly := func(f *comp.Field, v float64) {
		if v == 0 {
			f.Editor.SetText("")
			return
		}
		f.Editor.SetText(number(v))
	}
	set(&c.gain, a.GainDBiPeak)
	beamOnly(&c.beam, a.BeamwidthDeg)
	beamOnly(&c.f2b, a.FrontToBackDB)
	set(&c.bearing, a.BearingDeg)
	set(&c.downtilt, a.DowntiltDeg)
	set(&c.feedline, a.FeedlineDB)
	c.shown = a
}

// number is a figure somebody is about to edit.
//
// Two decimals and no trailing zeroes. A bearing computed from two positions is
// a full float, and 186.26595467067725 in a box is not a number anybody can
// change: the digits past the second are below what a mast can be pointed to
// and they push the units off the end of the field.
func number(v float64) string {
	s := strconv.FormatFloat(v, 'f', 2, 64)
	s = strings.TrimRight(s, "0")
	return strings.TrimSuffix(s, ".")
}

// build names every control once. Labels and hints are set here rather than per
// frame because the audit reads a hint to decide what to type into a box.
func (c *antennaControls) build() {
	for i := range c.sorts {
		c.sorts[i].Label, c.sorts[i].Kind = antennaSorts[i].label, comp.Quiet
	}
	for i := range c.pols {
		c.pols[i].Label, c.pols[i].Kind = antennaPols[i].label, comp.Quiet
	}
	for f, spec := range map[*comp.Field]struct{ label, hint, suffix string }{
		&c.gain:     {"peak gain", "gain in dB at the horizon", "dBi"},
		&c.beam:     {"beamwidth", "half-power width in degrees", "deg"},
		&c.f2b:      {"front to back", "how far down the back is, in dB", "dB"},
		&c.bearing:  {"bearing", "compass degrees, 0 at north", "deg"},
		&c.downtilt: {"downtilt", "degrees below the horizon", "deg"},
		&c.feedline: {"feedline", "cable and connector loss in dB", "dB"},
	} {
		f.Label, f.Hint, f.Suffix = spec.label, spec.hint, spec.suffix
		f.Editor.SingleLine = true
	}
	c.aimAt.Label, c.aimAt.Hint = "aim at", "another node, by name"
	c.aimAt.Editor.SingleLine = true
	c.apply.Label, c.apply.Kind = "apply", comp.Primary
	c.applyAll.Label, c.applyAll.Kind = "apply to every node", comp.Secondary
	c.aim.Label, c.aim.Kind = "point it there", comp.Secondary
	c.built = true
}

// antenna draws the tab and fires what was pressed.
func (p *WindowPanel) antenna(t *theme.Theme, gtx layout.Context,
	s *state.Snapshot) layout.Dimensions {
	c := &p.ant
	if !c.built {
		c.build()
	}
	var node *state.Node
	if s != nil {
		for i := range s.Nodes {
			if s.Nodes[i].Name == p.Node {
				node = &s.Nodes[i]
			}
		}
	}
	if node != nil && node.Antenna != c.shown {
		c.fill(node.Antenna)
	}
	current := c.shown.Type
	for i := range c.sorts {
		kind := comp.Quiet
		if antennaSorts[i].token == current {
			kind = comp.Primary
		}
		c.sorts[i].Kind = kind
	}
	for i := range c.pols {
		kind := comp.Quiet
		if antennaPols[i].token == c.shown.Polarisation {
			kind = comp.Primary
		}
		c.pols[i].Kind = kind
	}

	c.list.Axis = layout.Vertical
	rows := c.rows(t, node)
	return comp.List(t, &c.list, len(rows), func(gtx layout.Context, i int) layout.Dimensions {
		return rows[i](gtx)
	})(gtx)
}

// rows is the tab's content in order, so the draw above stays a list and the
// layout stays readable.
func (c *antennaControls) rows(t *theme.Theme, node *state.Node) []layout.Widget {
	head := func(sec string) layout.Widget { return comp.SectionRule(t, sec) }
	buttons := func(bs []*comp.Button) layout.Widget {
		return func(gtx layout.Context) layout.Dimensions {
			bar := comp.ActionBar{Buttons: bs}
			return bar.Layout(t, gtx)
		}
	}
	numbers := comp.ActionBar{
		Fields:  []*comp.Field{&c.gain, &c.beam, &c.f2b},
		Buttons: nil,
	}
	aiming := comp.ActionBar{
		Fields:  []*comp.Field{&c.bearing, &c.downtilt, &c.feedline},
		Buttons: []*comp.Button{&c.apply, &c.applyAll},
		Note: "applies to this node; the second button gives every node in the " +
			"scenario the same antenna, which is how a large network is set at all",
	}
	at := comp.ActionBar{
		Fields:  []*comp.Field{&c.aimAt},
		Buttons: []*comp.Button{&c.aim},
		Note: "the bearing between two placed nodes is exact, so this is a better " +
			"answer than reading one off the map and typing it back",
	}
	return []layout.Widget{
		func(gtx layout.Context) layout.Dimensions { return c.summary(t, gtx, node) },
		head("sort"),
		buttons([]*comp.Button{&c.sorts[0], &c.sorts[1], &c.sorts[2], &c.sorts[3]}),
		head("what it is"),
		func(gtx layout.Context) layout.Dimensions { return numbers.Layout(t, gtx) },
		head("where it points"),
		func(gtx layout.Context) layout.Dimensions { return aiming.Layout(t, gtx) },
		func(gtx layout.Context) layout.Dimensions { return at.Layout(t, gtx) },
		head("polarisation"),
		buttons([]*comp.Button{&c.pols[0], &c.pols[1], &c.pols[2]}),
		func(gtx layout.Context) layout.Dimensions {
			return comp.Text(t, t.Sz.Caption, t.P.Faint,
				"a mismatch is charged against the far end's polarisation: 3 dB "+
					"circular to linear, 20 dB vertical to horizontal. Two nodes "+
					"that have not said cost each other nothing")(gtx)
		},
	}
}

// summary is what this node's antenna is right now, in one line, so the boxes
// below are read as a change to something rather than as a blank form.
func (c *antennaControls) summary(t *theme.Theme, gtx layout.Context,
	node *state.Node) layout.Dimensions {
	line := "this node is not in the loaded network"
	if node != nil {
		line = "no antenna, so this node is credited no gain at all"
		if node.Antenna.Type != "" {
			line = fmt.Sprintf("%s, %.1f dBi, pointing %.0f degrees, %.0f down",
				node.Antenna.Type, node.Antenna.GainDBiPeak,
				node.Antenna.BearingDeg, node.Antenna.DowntiltDeg)
		}
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(comp.OneLine(t, t.Sz.Body, t.P.Ink, line, false)),
		layout.Rigid(comp.Text(t, t.Sz.Caption, t.P.Faint,
			"gain is evaluated towards the far end, so turning this changes which "+
				"links close. Switch the map's Antenna layer on to see the shape")),
	)
}

// antennaClicks fires what was pressed. Shared with the audit's flat draw, so a
// control cannot be wired in one and dead in the other.
func (p *WindowPanel) antennaClicks(gtx layout.Context) {
	c := &p.ant
	if p.OnDo == nil {
		return
	}
	for i := range c.sorts {
		if c.sorts[i].Click.Clicked(gtx) {
			p.OnDo("nodes.antenna", map[string]any{
				"node": p.Node, "pattern": antennaSorts[i].token})
		}
	}
	for i := range c.pols {
		if c.pols[i].Click.Clicked(gtx) {
			p.OnDo("nodes.antenna", map[string]any{
				"node": p.Node, "polarisation": antennaPols[i].token})
		}
	}
	applied := c.apply.Click.Clicked(gtx)
	if all := c.applyAll.Click.Clicked(gtx); applied || all {
		params := c.typed()
		if !all {
			// Absent means every node, which is what makes the second button a
			// different button rather than the same one with a warning.
			params["node"] = p.Node
		}
		p.OnDo("nodes.antenna", params)
	}
	if c.aim.Click.Clicked(gtx) {
		p.OnDo("node.aim", map[string]any{
			"node": p.Node, "at": comp.FieldText(&c.aimAt)})
	}
}

// antennaAuditRows is every antenna control at once and none of its prose, for
// the audit's flat layout - a tab hides its controls from a pointer, and the
// audit's whole point is pressing them.
func (p *WindowPanel) antennaAuditRows(t *theme.Theme, gtx layout.Context,
	s *state.Snapshot) layout.Dimensions {
	c := &p.ant
	if !c.built {
		c.build()
	}
	var node *state.Node
	if s != nil {
		for i := range s.Nodes {
			if s.Nodes[i].Name == p.Node {
				node = &s.Nodes[i]
			}
		}
	}
	if node != nil && node.Antenna != c.shown {
		c.fill(node.Antenna)
	}
	// Two rows rather than the tab's five: every other pane in the flat layout
	// has already taken its bound, and the console's own send button goes off
	// the bottom of the canvas for every row added above it.
	sorts := comp.ActionBar{Buttons: []*comp.Button{
		&c.sorts[0], &c.sorts[1], &c.sorts[2], &c.sorts[3],
		&c.pols[0], &c.pols[1], &c.pols[2]}}
	rest := comp.ActionBar{
		Fields: []*comp.Field{&c.gain, &c.beam, &c.f2b,
			&c.bearing, &c.downtilt, &c.feedline, &c.aimAt},
		Buttons: []*comp.Button{&c.apply, &c.applyAll, &c.aim},
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return sorts.Layout(t, gtx) }),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return rest.Layout(t, gtx) }),
	)
}

// typed is what is in the boxes, leaving out the ones nobody filled in - the
// verb applies only what it is given, so an empty box means "leave it alone"
// rather than "set it to zero".
func (c *antennaControls) typed() map[string]any {
	out := map[string]any{}
	for key, f := range map[string]*comp.Field{
		"gain_dbi_peak":    &c.gain,
		"beamwidth_deg":    &c.beam,
		"front_to_back_db": &c.f2b,
		"bearing_deg":      &c.bearing,
		"downtilt_deg":     &c.downtilt,
		"feedline_db":      &c.feedline,
	} {
		if v := comp.FieldText(f); v != "" {
			out[key] = atof(v)
		}
	}
	return out
}
