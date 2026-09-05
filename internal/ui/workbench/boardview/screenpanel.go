// The board's panel on its own, as large as the window it is given.
//
// Worth a window rather than a bigger box because of what the work is: a
// firmware developer reads what the board drew while watching what its pins
// did, and on two monitors those are two windows. It scales in whole steps to
// whatever it is dragged to, so the picture stays the firmware's own at every
// size, and it says which step that turned out to be - a panel at 1:1 and one
// at 3:1 are different evidence and nothing else on the screen would say which
// this is.
package boardview

import (
	"fmt"

	"gioui.org/layout"

	"github.com/MeshBench/meshbench/internal/app/state"
	hw "github.com/MeshBench/meshbench/internal/firmware/board"
	"github.com/MeshBench/meshbench/internal/ui/comp"
	"github.com/MeshBench/meshbench/internal/ui/theme"
)

// ScreenPanel is the popped-out panel.
type ScreenPanel struct {
	Node string
	OnDo func(verb string, params any)

	// view is the same ScreenView the window it came from draws, so the touch
	// mapping and the key focus are shared rather than copied.
	view *ScreenView
	// pic is this window's own image of the panel, and must not be the other
	// window's. The two are separate frame loops drawing at separate scales,
	// so one buffer between them is one goroutine reallocating it while the
	// other writes into it.
	pic comp.ScreenImage
}

func (p *ScreenPanel) Draw(t *theme.Theme, gtx layout.Context,
	s *state.Snapshot) layout.Dimensions {

	st := statOf(s, p.Node)
	b, ok := boardOf(st)
	if !ok || !hasPanel(b) || b.Hardware.Screen == nil {
		return layout.Center.Layout(gtx, comp.Text(t, t.Sz.Caption, t.P.Faint,
			"this node has no panel to show"))
	}
	sc := b.Hardware.Screen

	// Chosen here rather than read back off the view after it has drawn: a
	// Flex lays its rigid children out before its flexed one, so the caption
	// below would ask which scale was used before the panel had picked, and
	// print 0:1. One decision, passed to both.
	foot := gtx
	foot.Constraints.Max.Y -= gtx.Dp(t.Sp.L)
	n := FitIn(sc, foot)

	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return p.view.Layout(t, gtx, b, st, n, p.OnDo, p.Node, &p.pic)
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: t.Sp.XS, Bottom: t.Sp.XS}.Layout(gtx,
				func(gtx layout.Context) layout.Dimensions {
					return layout.Center.Layout(gtx,
						comp.Mono(t, t.Sz.Caption, t.P.Faint,
							fmt.Sprintf("%d × %d · %s · %d:1", sc.WidthPx, sc.HeightPx,
								sc.Controller, n)))
				})
		}),
	)
}

// statOf and boardOf are the same two lookups the main panel makes, said once
// so the two windows cannot come to disagree about which board a node is.
func statOf(s *state.Snapshot, node string) *state.NodeStat {
	if s == nil {
		return nil
	}
	for i := range s.Stats {
		if s.Stats[i].Name == node {
			return &s.Stats[i]
		}
	}
	return nil
}

// boardOf is the profile this node runs, whether or not anybody has recorded
// what it carries.
//
// A board with no Hardware is not a node without a board: every nRF52 profile
// is one today, and refusing them cost the radio table as well - which needs no
// panel at all, because the chip's own registers reach here whichever emulator
// is running it.
func boardOf(st *state.NodeStat) (hw.Board, bool) {
	if st == nil || st.Board == "" {
		return hw.Board{}, false
	}
	b, err := hw.BoardByName(st.Board)
	if err != nil {
		return hw.Board{}, false
	}
	return b, true
}

// hasPanel reports whether anybody has recorded what this board carries. Not
// the same question as whether it carries anything: a board that carries
// nothing declares an empty panel and says so.
func hasPanel(b hw.Board) bool { return b.Hardware != nil }
