package study

import (
	"strings"
	"testing"

	"github.com/MeshBench/meshbench/internal/app/session"
	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/world/scenario"
)

// aStudy is the study verbs on a store, with a two-node network in the
// snapshot and none of the machinery a raster would need.
//
// Deliberately no engine and no terrain: every refusal below has to happen
// before any of that is touched, and a test that needed an engine to prove a
// parameter was refused would be proving something else.
func aStudy(t *testing.T) (*state.Store, *session.Sim) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", home)
	t.Setenv("XDG_CACHE_HOME", home)
	t.Setenv("HOME", home)

	st := state.New(10)
	s := &session.Sim{}
	registerCoverageVerbs(st, s)
	registerCoverageMap(st, s)
	registerPlanningVerbs(st, s)
	registerCoverageCombined(st, s)
	registerValidate(st, s)
	st.Handle("test.nodes", func(w *state.World, p any) (any, error) {
		nodes, ok := p.([]state.Node)
		if !ok {
			t.Fatalf("test.nodes was handed %T", p)
		}
		w.Nodes = nodes
		return nil, nil
	})
	go st.Run(t.Context())

	nodes := session.StateNodes([]scenario.Node{
		nodeAt("West Lomond", scenario.SimpleRepeater, 56.25, -3.29),
		nodeAt("Dunfermline", scenario.SimpleRepeater, 56.07, -3.46),
	})
	if _, err := st.Do(t.Context(), "test.nodes", nodes); err != nil {
		t.Fatal(err)
	}
	return st, s
}

// refuses runs a verb that must fail, and returns the message so the caller can
// check it says something useful.
func refuses(t *testing.T, st *state.Store, verb string, params any) string {
	t.Helper()
	got, err := st.Do(t.Context(), verb, params)
	if err == nil {
		t.Fatalf("%s accepted %v and answered %v", verb, params, got)
	}
	return err.Error()
}

// mentions fails unless every fragment is in the message. A refusal that does
// not name the verb, the parameter and the way out is a refusal somebody has to
// come and read the source to act on.
func mentions(t *testing.T, msg string, want ...string) {
	t.Helper()
	for _, w := range want {
		if !strings.Contains(msg, w) {
			t.Errorf("%q does not mention %q", msg, w)
		}
	}
}

// A node name that matches nothing must be refused by name, not answered as
// though no node had been selected: the two send whoever asked to completely
// different places.
func TestCoverageComputeRefusesANodeThisNetworkHasNot(t *testing.T) {
	st, _ := aStudy(t)
	msg := refuses(t, st, "coverage.compute", "West Lomand")
	mentions(t, msg, "coverage.compute", "West Lomand", "West Lomond", "Dunfermline")
}

// And with nothing named and nothing selected it still says so, because that
// is a different fact and has a different answer.
func TestCoverageComputeStillAsksForASelection(t *testing.T) {
	st, _ := aStudy(t)
	msg := refuses(t, st, "coverage.compute", nil)
	mentions(t, msg, "no node selected")
}

func TestWaterfallCaptureRefusesANodeThisNetworkHasNot(t *testing.T) {
	st, _ := aStudy(t)
	msg := refuses(t, st, "waterfall.capture", "Nowhere")
	mentions(t, msg, "waterfall.capture", "Nowhere", "West Lomond")
}

// A raster over the wrong ground looks exactly like a raster over the right
// ground, so three borders and a typo must not quietly become the whole
// network's box.
func TestCoverageMapRefusesAHalfGivenViewport(t *testing.T) {
	st, _ := aStudy(t)
	msg := refuses(t, st, "coverage.map", map[string]any{
		"south": 56.0, "north": 56.5, "west": -3.6,
	})
	mentions(t, msg, "coverage.map", "south", "north", "west", "east")
}

func TestCoverageMapRefusesABorderThatIsNotANumber(t *testing.T) {
	st, _ := aStudy(t)
	msg := refuses(t, st, "coverage.map", map[string]any{
		"south": "56.0", "north": 56.5, "west": -3.6, "east": -3.0,
	})
	mentions(t, msg, "coverage.map", "south")

	// And one that is a number but not a latitude.
	msg = refuses(t, st, "coverage.map", map[string]any{
		"south": 560.0, "north": 56.5, "west": -3.6, "east": -3.0,
	})
	mentions(t, msg, "south", "-90", "90")
}

// The resolution rides along with a viewport, and a resolution outside what a
// raster can be used to be replaced by the saved one - so a caller who asked
// for 30,000 cells was told a 240-cell picture was what they had asked for.
func TestCoverageMapRefusesAResolutionItCannotDraw(t *testing.T) {
	st, _ := aStudy(t)
	msg := refuses(t, st, "coverage.map", map[string]any{"cells": 30000.0})
	mentions(t, msg, "coverage.map", "cells", "4096")

	msg = refuses(t, st, "coverage.map", map[string]any{"cells": "lots"})
	mentions(t, msg, "coverage.map", "cells")
}

func TestCoverageMapRefusesAStationThisNetworkHasNot(t *testing.T) {
	st, _ := aStudy(t)
	msg := refuses(t, st, "coverage.map", map[string]any{"station": "Nowhere"})
	mentions(t, msg, "coverage.map", "Nowhere", "West Lomond")
}

// Absent is a read of the current setting and must stay one; present and
// unreadable is somebody trying to change it, and answering them with the
// unchanged number reports the opposite of what happened.
func TestCoverageResolutionReadsWhenAskedNothingAndRefusesNonsense(t *testing.T) {
	st, _ := aStudy(t)
	got, err := st.Do(t.Context(), "coverage.resolution", nil)
	if err != nil {
		t.Fatalf("reading the resolution: %v", err)
	}
	if got.(map[string]any)["cells"] != mapGridDefault {
		t.Errorf("read %v, want the default %d",
			got.(map[string]any)["cells"], mapGridDefault)
	}
	msg := refuses(t, st, "coverage.resolution", map[string]any{"cells": "sharp"})
	mentions(t, msg, "coverage.resolution", "cells")

	if _, err := st.Do(t.Context(), "coverage.resolution",
		map[string]any{"cells": 512.0}); err != nil {
		t.Fatalf("a resolution in range was refused: %v", err)
	}
}

// The precedent the rest of these follow: say what was wrong and what the
// choices are.
func TestCoverageStartNamesTheModesItHas(t *testing.T) {
	st, _ := aStudy(t)
	msg := refuses(t, st, "coverage.start", map[string]any{"mode": "sideways"})
	mentions(t, msg, "sideways", "best", "gaps", "redundancy", "node")
}

// The window is the whole scientific claim a validation run makes. Zero, a
// negative and "twelve" all used to come out as twenty-four hours, so the
// answer described a window nobody had asked for.
func TestValidateFetchRefusesAWindowItCannotUse(t *testing.T) {
	st, _ := aStudy(t)
	const url = "http://corescope.invalid"
	for _, bad := range []any{0.0, -6.0, "twelve"} {
		msg := refuses(t, st, "validate.fetch",
			map[string]any{"url": url, "hours": bad})
		mentions(t, msg, "validate.fetch", "hours")
	}
}

func TestValidateFetchStillNeedsASource(t *testing.T) {
	st, _ := aStudy(t)
	msg := refuses(t, st, "validate.fetch", map[string]any{})
	mentions(t, msg, "no source")
}

// Calibration is the number every later margin is measured against. A db that
// cannot be read must not fall through to the residuals, and with nothing
// measured at all the verb must refuse rather than apply the most optimistic
// model there is.
func TestValidateCalibrateRefusesADbItCannotRead(t *testing.T) {
	st, s := aStudy(t)
	msg := refuses(t, st, "validate.calibrate", map[string]any{"db": "lots"})
	mentions(t, msg, "validate.calibrate", "db")

	msg = refuses(t, st, "validate.calibrate", nil)
	mentions(t, msg, "nothing has been measured")

	msg = refuses(t, st, "validate.calibrate", map[string]any{"db": -3.0})
	mentions(t, msg, "excess loss is a loss")

	// A number that is a number is still applied: the refusals must not have
	// taken the working case with them.
	if _, err := st.Do(t.Context(), "validate.calibrate",
		map[string]any{"db": 12.0}); err != nil {
		t.Fatalf("a usable calibration was refused: %v", err)
	}
	if s.ExcessLossDB() != 12 || !s.ExcessSet() {
		t.Errorf("excess loss is %v (set %v), want 12 and set",
			s.ExcessLossDB(), s.ExcessSet())
	}
}

// project.save joins the name onto a directory and writes to it, so a name
// that is a path is a write anywhere the workbench can reach.
func TestProjectSaveRefusesAPathForAName(t *testing.T) {
	st, _ := aStudy(t)
	for _, bad := range []string{"../escape", "sub/dir", `..\escape`, ".hidden"} {
		msg := refuses(t, st, "project.save", map[string]any{"name": bad})
		mentions(t, msg, "project.save", "not a project name")
	}

	got, err := st.Do(t.Context(), "project.save", map[string]any{"name": "fife"})
	if err != nil {
		t.Fatalf("an ordinary name was refused: %v", err)
	}
	if got.(map[string]any)["saved"] != "fife" {
		t.Errorf("saved %v, want fife", got.(map[string]any)["saved"])
	}
}
