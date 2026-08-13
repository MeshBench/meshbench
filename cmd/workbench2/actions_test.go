package main

import (
	"testing"

	"gioui.org/f32"

	"gioui.org/layout"

	"github.com/A13xB0/meshcoresim/internal/gui/comp"
	"github.com/A13xB0/meshcoresim/internal/gui/state"
	"github.com/A13xB0/meshcoresim/internal/gui/theme"
)

// Do the buttons do anything?
//
// A screenshot proves a control was drawn. It cannot prove that pressing it
// reaches a verb, and this whole exercise started because a menu item had been
// dispatching a verb that does not exist.
//
// The text is set directly and only the buttons are clicked: where a field
// happens to sit is the layout's business and would make this a test of
// coordinates. Typing itself is covered separately.

type recorder struct {
	verbs  []string
	params []any
}

func (r *recorder) do(verb string, params any) {
	r.verbs = append(r.verbs, verb)
	r.params = append(r.params, params)
}

func (r *recorder) saw(verb string) bool {
	for _, v := range r.verbs {
		if v == verb {
			return true
		}
	}
	return false
}

// pressAlong clicks every few pixels across a row, so a button is found by
// being there rather than by a coordinate written down in advance.
func (h *panelHarness) pressAlong(y float32) {
	for x := float32(8); x < float32(h.sz.X); x += 12 {
		h.click(f32.Pt(x, y))
	}
}

func TestFleetControlsReachTheirVerbs(t *testing.T) {
	r := &recorder{}
	c := &fleetControls{do: r.do}
	h := newPanelHarness(c.Draw, &state.Snapshot{})
	h.frame()
	c.command.Editor.SetText("region put sco")
	c.regions.Editor.SetText("sco fif")
	h.frame()

	h.pressAlong(22)
	h.pressAlong(74)

	for _, want := range []string{"fleet.send", "nodes.regions", "nodes.allow_flood"} {
		if !r.saw(want) {
			t.Errorf("no button reached %s; got %v", want, r.verbs)
		}
	}
	for i, v := range r.verbs {
		if v != "fleet.send" {
			continue
		}
		m, _ := r.params[i].(map[string]any)
		if got, _ := m["command"].(string); got != "region put sco" {
			t.Errorf("fleet.send carried %q, not what the field holds", got)
		}
	}
	for i, v := range r.verbs {
		if v != "nodes.regions" {
			continue
		}
		m, _ := r.params[i].(map[string]any)
		rs, _ := m["regions"].([]any)
		if len(rs) != 2 {
			t.Errorf("nodes.regions carried %v, want two regions", rs)
		}
	}
}

func TestImportControlsReachAllFourSteps(t *testing.T) {
	r := &recorder{}
	c := &importControls{do: r.do}
	h := newPanelHarness(c.Draw, &state.Snapshot{})
	h.frame()
	c.url.Editor.SetText("https://example.test/")
	h.frame()

	h.pressAlong(22)

	for _, want := range []string{"import.fetch", "import.commit", "infer.run", "infer.apply"} {
		if !r.saw(want) {
			t.Errorf("no button reached %s; got %v", want, r.verbs)
		}
	}
}

func TestPlanningControlsAskAllThreeQuestions(t *testing.T) {
	r := &recorder{}
	c := &planningControls{do: r.do}
	h := newPanelHarness(c.Draw, &state.Snapshot{})
	h.frame()
	h.pressAlong(22)

	modes := map[string]bool{}
	for i, v := range r.verbs {
		if v != "coverage.start" {
			t.Errorf("planning reached %q", v)
			continue
		}
		m, _ := r.params[i].(map[string]any)
		mode, _ := m["mode"].(string)
		modes[mode] = true
	}
	for _, want := range []string{"best", "gaps", "redundancy", "node"} {
		if !modes[want] {
			t.Errorf("no button asked for %s coverage; got %v", want, modes)
		}
	}
}

func TestValidateControlsReachCalibration(t *testing.T) {
	r := &recorder{}
	c := &validateControls{do: r.do}
	h := newPanelHarness(c.Draw, &state.Snapshot{})
	h.frame()
	c.db.Editor.SetText("12.5")
	h.frame()
	h.pressAlong(22)

	for _, want := range []string{"validate.fetch", "validate.calibrate", "validate.uncalibrate"} {
		if !r.saw(want) {
			t.Errorf("no button reached %s; got %v", want, r.verbs)
		}
	}
	for i, v := range r.verbs {
		if v != "validate.calibrate" {
			continue
		}
		m, _ := r.params[i].(map[string]any)
		if got, _ := m["db"].(float64); got != 12.5 {
			t.Errorf("calibrate carried %v dB, not what the field holds", m["db"])
		}
	}
}

func TestBenchControlsReachTheirVerbs(t *testing.T) {
	r := &recorder{}
	c := &benchControls{do: r.do}
	h := newPanelHarness(c.Draw, &state.Snapshot{
		Nodes: []state.Node{{Name: "AngusOutlaw1", Kind: "companion", Selected: true}},
	})
	h.frame()
	c.msg.Editor.SetText("hello")
	h.frame()
	h.pressAlong(22)
	h.pressAlong(74)

	for _, want := range []string{"bench.serve", "bench.drop", "bench.stray",
		"companion.connect", "companion.send", "companion.advert"} {
		if !r.saw(want) {
			t.Errorf("no button reached %s; got %v", want, r.verbs)
		}
	}
	// With the node field blank it must act on the selection, not on "".
	for i, v := range r.verbs {
		if v != "companion.connect" {
			continue
		}
		m, _ := r.params[i].(map[string]any)
		if got, _ := m["node"].(string); got != "AngusOutlaw1" {
			t.Errorf("connect acted on %q, not the selected node", got)
		}
	}
}

func TestSweepControlsDefineAndRun(t *testing.T) {
	r := &recorder{}
	c := &sweepControls{do: r.do}
	h := newPanelHarness(c.Draw, &state.Snapshot{})
	h.frame()
	c.versions.Editor.SetText("repeater-v1.16.0 repeater-v1.17.0")
	c.seeds.Editor.SetText("1 2 3")
	c.sender.Editor.SetText("AngusOutlaw1")
	h.frame()
	h.pressAlong(22)

	for _, want := range []string{"experiment.vary", "experiment.seeds",
		"experiment.senders", "experiment.start", "experiment.stop", "experiment.export"} {
		if !r.saw(want) {
			t.Errorf("no button reached %s; got %v", want, r.verbs)
		}
	}
	for i, v := range r.verbs {
		if v != "experiment.vary" {
			continue
		}
		m, _ := r.params[i].(map[string]any)
		vs, _ := m["values"].([]any)
		if len(vs) != 2 {
			t.Errorf("vary carried %v, want the two versions typed", vs)
		}
	}
	for i, v := range r.verbs {
		if v != "experiment.seeds" {
			continue
		}
		m, _ := r.params[i].(map[string]any)
		ss, _ := m["seeds"].([]any)
		if len(ss) != 3 {
			t.Errorf("seeds carried %v, want three", ss)
		}
	}
}

func TestFeedControlsStartAndStop(t *testing.T) {
	r := &recorder{}
	c := &feedControls{do: r.do}
	h := newPanelHarness(c.Draw, &state.Snapshot{})
	h.frame()
	c.url.Editor.SetText("https://example.test/")
	h.frame()
	h.pressAlong(22)
	for _, want := range []string{"feed.pull", "feed.stop"} {
		if !r.saw(want) {
			t.Errorf("no button reached %s; got %v", want, r.verbs)
		}
	}
}

// The map toolbar: typing filters, and the tools select.
func TestMapToolbarFiltersAndPicksTools(t *testing.T) {
	mv := &comp.MapView{Zoom: 1000}
	m := &mapTools{mv: mv}
	h := newPanelHarness(
		func(t *theme.Theme, gtx layout.Context, _ *state.Snapshot) layout.Dimensions {
			return m.Draw(t, gtx)
		}, &state.Snapshot{})
	h.frame()

	// The filter applies as it is typed, with no button to press. The text is
	// set directly because where the box sits is the layout's business, and
	// typing itself is covered by the filter tests.
	m.filter.Editor.SetText("repeater")
	h.frame()
	if mv.Filter != "repeater" {
		t.Errorf("map filter is %q after typing", mv.Filter)
	}

	// A tool other than the default, found by pressing along the row.
	before := mv.Zoom
	h.pressAlong(22)
	if mv.Tool == "" || mv.Tool == "select" {
		t.Errorf("no tool was chosen; tool is %q", mv.Tool)
	}
	if mv.Zoom == before && !mv.FitNext {
		t.Errorf("neither zoom nor fit responded: zoom %v, fitNext %v", mv.Zoom, mv.FitNext)
	}
}
