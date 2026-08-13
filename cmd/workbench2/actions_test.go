package main

import (
	"testing"

	"gioui.org/f32"

	"github.com/A13xB0/meshcoresim/internal/gui/state"
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
