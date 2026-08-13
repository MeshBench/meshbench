package main

import (
	"testing"

	"github.com/A13xB0/meshcoresim/internal/gui/state"
)

// The sidebar opens the section that was pressed. Its rows are plain
// clickables rather than buttons - they change the view, not the world - so
// the control audit does not press them and this does.
func TestConfigurationSectionsSwitch(t *testing.T) {
	cfg := &configPanel{do: func(string, any) {}}
	h := newPanelHarness(cfg.Draw, auditSnapshot())
	h.frame()
	h.frame()
	if len(cfg.secRows) != len(configSections) {
		t.Fatalf("%d sidebar rows for %d sections", len(cfg.secRows), len(configSections))
	}
	for i := range configSections {
		cfg.secRows[i].Click()
		h.frame()
		h.frame()
		if cfg.active != i {
			t.Errorf("pressed %q and the open section is %q",
				configSections[i], configSections[cfg.active])
		}
	}
}

// Setting a value goes through the same verb a script would use, and a value
// that does not parse stays local and says so.
func TestConfigurationSetsThroughVerbs(t *testing.T) {
	r := &recorder{}
	cfg := &configPanel{do: r.do}
	h := newPanelHarness(cfg.auditDraw, auditSnapshot())
	h.frame()

	cfg.seed.Editor.SetText("4242")
	cfg.setSeed.Click.Click()
	h.frame()
	h.frame()
	if !r.saw("sim.seed") {
		t.Errorf("set seed reached %v, want sim.seed", r.verbs)
	}

	cfg.margin.Editor.SetText("not a number")
	cfg.setMargin.Click.Click()
	h.frame()
	h.frame()
	if r.saw("study.margin") {
		t.Error("a margin that does not parse still reached the store")
	}
	if cfg.margin.Error == "" {
		t.Error("a margin that does not parse said nothing in the field")
	}
}

// The status pill tells warming apart from running, because a run that is
// still measuring links is the one state a person must not start a study in.
func TestConfigurationPillStates(t *testing.T) {
	s := auditSnapshot()
	if got := pillWord(s); got != "Ready to run" {
		t.Errorf("idle run reads %q", got)
	}
	s.Jobs = []state.Job{{ID: "links", What: "measuring"}}
	if got := pillWord(s); got != "Warming up" {
		t.Errorf("warming run reads %q", got)
	}
	s.Jobs = nil
	s.Playing = true
	if got := pillWord(s); got != "Running" {
		t.Errorf("playing run reads %q", got)
	}
}
