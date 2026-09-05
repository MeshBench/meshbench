// Picking a row, which is the whole reason the inspector exists.
//
// The selection was drawn and never wired: the highlight moved with p.sel, the
// inspector read p.sel, and nothing anywhere set it from a press. So the window
// opened describing whatever was first and stayed there, on both tables - and
// the fault was invisible in a screenshot, because the row it was stuck on
// looked exactly like a row somebody had chosen.
//
// Driven through real presses at real coordinates rather than by poking the
// widget, because "the widget's Clicked returns true" was never the thing in
// doubt: what was missing was any hit area at all.
package boardview

import (
	"testing"

	"gioui.org/f32"
	"gioui.org/layout"

	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/ui/theme"
	"github.com/MeshBench/meshbench/internal/ui/uitest"
)

// A press somewhere in the table moves the selection off the first row.
//
// Swept down the table rather than aimed at one row: a coordinate written down
// in advance is a test that breaks when a rule moves by two pixels, and what is
// being asserted is that the rows are pressable at all.
func TestPressingATableRowMovesTheSelection(t *testing.T) {
	for _, tab := range []Tab{TabRadio, TabWiring} {
		p, snap := deckPanel(tab)
		h := uitest.New(func(th *theme.Theme, gtx layout.Context,
			s *state.Snapshot) layout.Dimensions {
			return p.Draw(th, gtx, s)
		}, snap)
		h.Frame()
		if p.sel != 0 {
			t.Fatalf("%v: opened on row %d, want the first", tab, p.sel)
		}
		moved := false
		for y := float32(120); y < 420 && !moved; y += 6 {
			h.PressAlong(y)
			moved = p.sel != 0
		}
		if !moved {
			t.Errorf("%v: no press anywhere in the table moved the selection, "+
				"so every row of it describes the first row for ever", tab)
		}
	}
}

// The index down the rail is the other view of the same list, and pressing a
// part in it has to move the same selection.
func TestPressingThePartsIndexMovesTheSelection(t *testing.T) {
	p, snap := deckPanel(TabWiring)
	h := uitest.New(func(th *theme.Theme, gtx layout.Context,
		s *state.Snapshot) layout.Dimensions {
		return p.Draw(th, gtx, s)
	}, snap)
	h.Frame()
	moved := false
	// Down the rail only: the index is the lower half of the left column, and
	// the table is off to the right of every x this touches.
	for y := float32(300); y < 640 && !moved; y += 5 {
		for x := float32(8); x < 180 && !moved; x += 20 {
			h.Click(f32.Pt(x, y))
			moved = p.sel != 0
		}
	}
	if !moved {
		t.Error("no press in the parts index moved the selection, so the rail " +
			"draws a list that cannot be picked from")
	}
}

// One row, one widget, wherever it is drawn.
//
// The table and the index share it, so the two cannot disagree about what is
// selected - two clickables per row would produce a row that sometimes works.
func TestOneRowIsOneWidget(t *testing.T) {
	p := &Panel{Node: "Deck", Tab: TabWiring}
	r := Row{Group: "Input", Name: "trackball"}
	first := p.pick(p.rowKey(r))
	if p.pick(p.rowKey(r)) != first {
		t.Error("one row produced two clickables, so a press can land on the " +
			"wrong one of them")
	}
	if p.pick(p.rowKey(Row{Group: "Input", Name: "keyboard"})) == first {
		t.Error("two rows share one widget")
	}
	// Keyed by what the row is, not by where it sits: widget identity is
	// address, and this table is re-derived from the board every frame, so a
	// slice indexed by position would hand a press to whatever is third now.
	p.Tab = TabRadio
	if p.pick(p.rowKey(r)) == first {
		t.Error("the same name on two tabs shares one widget, and both tables " +
			"carry a Radio group")
	}
}

// A layer-shell window draws its own title bar, because nothing else will.
//
// Without it the window cannot be dragged, maximised or closed. The three
// accessors the pop-out loop needs were all implemented here and the bar was
// never laid out, so nothing failed and nothing worked - and the node window
// beside it, which does lay one out, made the difference look like a platform
// quirk.
func TestALayeredWindowDrawsItsOwnTitleBar(t *testing.T) {
	p, snap := deckPanel(TabRadio)
	p.SetLayered(true)
	h := uitest.New(func(th *theme.Theme, gtx layout.Context,
		s *state.Snapshot) layout.Dimensions {
		return p.Draw(th, gtx, s)
	}, snap)
	h.Frame()
	if got := p.TitleBar().Title; got == "" {
		t.Error("a layered window laid out no title bar, so it cannot be " +
			"dragged, maximised or closed")
	}
	// And an ordinary window leaves it to the compositor rather than drawing a
	// second bar under the real one.
	q, snap2 := deckPanel(TabRadio)
	h2 := uitest.New(func(th *theme.Theme, gtx layout.Context,
		s *state.Snapshot) layout.Dimensions {
		return q.Draw(th, gtx, s)
	}, snap2)
	h2.Frame()
	if q.TitleBar().Title != "" {
		t.Error("an undecorated window drew a title bar of its own as well")
	}
}

// deckPanel is a T-Deck part way through a run, and the snapshot it reads.
func deckPanel(tab Tab) (*Panel, *state.Snapshot) {
	p := &Panel{Node: "Deck", Tab: tab}
	st := state.NodeStat{Name: "Deck", Board: "LilyGo_TDeck", Backend: "emulated",
		Running: true, IRQReads: 9,
		Radio: state.RadioState{Reported: true, Boosted: true, GainReg: 0x96,
			TxPowerDBm: 22, SF: 10, CR: 5, FreqHz: 869618000,
			BandwidthHz: 250000, IRQMask: 2}}
	return p, &state.Snapshot{Stats: []state.NodeStat{st}}
}
