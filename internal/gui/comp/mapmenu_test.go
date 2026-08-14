package comp

import (
	"image"
	"testing"

	"gioui.org/f32"
	"gioui.org/font/gofont"
	"gioui.org/io/input"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/text"
	"gioui.org/unit"

	"github.com/MeshBench/meshbench/internal/gui/state"
	"github.com/MeshBench/meshbench/internal/gui/theme"
)

// The right-click menu has to reach its own entries.
//
// Reported repeatedly: the menu opens on a node, an entry is chosen, and
// nothing happens. A menu that draws and cannot be used is worse than no menu,
// because it says the feature is there.

type mapHarness struct {
	mv  *MapView
	th  *theme.Theme
	r   input.Router
	ops op.Ops
	sz  image.Point
	sn  *state.Snapshot
}

func newMapHarness() *mapHarness {
	return &mapHarness{
		mv: &MapView{Zoom: 4000, CentreLat: 56.3, CentreLon: -3.3},
		th: theme.New(theme.Dark, theme.Default,
			text.NewShaper(text.WithCollection(gofont.Collection()))),
		sz: image.Pt(900, 700),
		sn: &state.Snapshot{Nodes: []state.Node{
			{Name: "Abernethy Repeater", Kind: "simple-repeater", Lat: 56.33, Lon: -3.32},
			{Name: "Bishop Hill", Kind: "simple-repeater", Lat: 56.2, Lon: -3.28},
		}},
	}
}

func (h *mapHarness) frame() {
	h.ops.Reset()
	gtx := layout.Context{
		Ops:         &h.ops,
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Exact(h.sz),
		Source:      h.r.Source(),
	}
	h.mv.Layout(h.th, gtx, h.sn)
	h.r.Frame(&h.ops)
}

func (h *mapHarness) press(at f32.Point, b pointer.Buttons) {
	h.r.Queue(
		pointer.Event{Kind: pointer.Press, Position: at, Buttons: b},
		pointer.Event{Kind: pointer.Release, Position: at, Buttons: b},
	)
	h.frame()
}

// Opening the menu and choosing something from it.
func TestChoosingFromTheMapMenuReachesTheAction(t *testing.T) {
	h := newMapHarness()
	got := ""
	h.mv.OnMenu = func(action, node string, lat, lon float64) { got = action }
	h.frame()

	// Right-click somewhere empty: the menu that offers map actions.
	h.press(f32.Pt(200, 200), pointer.ButtonSecondary)
	if !h.mv.menu.open {
		t.Fatal("a right-click did not open the menu at all")
	}

	// Then choose the first entry, which sits just inside the menu's top left.
	// Scanned down the menu rather than computed: where each row lands is the
	// layout's business.
	for y := float32(204); y < 320 && got == ""; y += 4 {
		h.press(f32.Pt(230, y), pointer.ButtonPrimary)
	}
	if got == "" {
		t.Fatal("the menu opened and choosing an entry reached nothing")
	}
	if h.mv.menu.open {
		t.Error("the menu stayed open after an entry was chosen")
	}
	t.Logf("chose %q", got)
}
