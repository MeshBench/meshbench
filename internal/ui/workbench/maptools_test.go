package workbench

import (
	"sort"
	"strings"
	"testing"

	"gioui.org/f32"
	"gioui.org/layout"

	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/ui/comp"
	"github.com/MeshBench/meshbench/internal/ui/theme"
)

// The map toolbar, pressed.
//
// It was left out of the control audit because it draws without a snapshot and
// acts on the map rather than on a verb, which made it the one strip of the
// workbench nothing checked. The map is the thing people point at.
func TestEveryMapToolReachesTheMap(t *testing.T) {
	mv := &comp.MapView{Zoom: 1000}
	m := &mapTools{mv: mv}
	h := newPanelHarness(func(th *theme.Theme, gtx layout.Context,
		_ *state.Snapshot) layout.Dimensions {
		return m.Draw(th, gtx)
	}, nil)
	h.frame()
	h.frame()

	// The toolbar is one row. Sweep it and watch what changes.
	tools := map[string]bool{}
	zoomedIn, zoomedOut, fitted := false, false, false
	for y := float32(2); y < 80; y += 3 {
		for x := float32(2); x < float32(h.sz.X); x += 4 {
			before := mv.Zoom
			h.click(f32.Pt(x, y))
			switch {
			case mv.Zoom > before:
				zoomedIn = true
			case mv.Zoom < before:
				zoomedOut = true
			}
			if mv.FitNext {
				fitted, mv.FitNext = true, false
			}
			tools[m.current] = true
		}
	}

	var missing []string
	for _, name := range toolNames {
		if !tools[name] {
			missing = append(missing, "the "+name+" tool")
		}
	}
	if !zoomedIn {
		missing = append(missing, "zoom in")
	}
	if !zoomedOut {
		missing = append(missing, "zoom out")
	}
	if !fitted {
		missing = append(missing, "fit")
	}

	// The filter applies as it is typed rather than on a press, so what it
	// has to reach is the map's own filter.
	h.click(f32.Pt(100, 20))
	h.typeText("bishop")
	// One more frame: the toolbar hands the text to the map at the top of
	// Draw, before the editor has seen the keystroke, so the map learns about
	// it on the frame after the one that typed it.
	h.frame()
	if mv.Filter != "bishop" {
		missing = append(missing, "the filter box (the map sees "+
			mv.Filter+" after typing bishop)")
	}

	if len(missing) > 0 {
		sort.Strings(missing)
		t.Errorf("map toolbar controls that reach nothing:\n  %s",
			strings.Join(missing, "\n  "))
	}
	t.Logf("%d tools, zoom, fit and the filter all reach the map", len(toolNames))
}
