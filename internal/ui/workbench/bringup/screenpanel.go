// The board's panel on its own, as large as the window it is given.
//
// Worth a window rather than a bigger box because of what the work is: a
// firmware developer reads what the board drew while watching what its pins
// did, and on two monitors those are two windows. It scales in whole steps to
// whatever it is dragged to, so the picture stays the firmware's own at every
// size, and it says which step that turned out to be - a panel at 1:1 and one
// at 3:1 are different evidence and nothing else on the screen would say which
// this is.
package bringup

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

	Layered   bool
	maximised bool
	bar       comp.TitleBar
}

func (p *ScreenPanel) SetLayered(on bool)       { p.Layered = on }
func (p *ScreenPanel) TitleBar() *comp.TitleBar { return &p.bar }
func (p *ScreenPanel) SetMaximised(on bool)     { p.maximised = on }

func (p *ScreenPanel) Draw(t *theme.Theme, gtx layout.Context,
	s *state.Snapshot) layout.Dimensions {

	st := statOf(s, p.Node)
	b, ok := boardOf(st)
	if !ok || b.Hardware.Screen == nil {
		return layout.Center.Layout(gtx, comp.Text(t, t.Sz.Caption, t.P.Faint,
			"this node has no panel to show"))
	}
	sc := b.Hardware.Screen
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				// Zero asks for the largest whole scale the space allows,
				// which is what makes the window resizable.
				return p.view.Layout(t, gtx, b, st, 0, p.OnDo, p.Node)
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: t.Sp.XS, Bottom: t.Sp.XS}.Layout(gtx,
				func(gtx layout.Context) layout.Dimensions {
					return layout.Center.Layout(gtx,
						comp.Mono(t, t.Sz.Caption, t.P.Faint,
							fmt.Sprintf("%d × %d · %s · %d:1", sc.WidthPx, sc.HeightPx,
								sc.Controller, p.view.drawn)))
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

func boardOf(st *state.NodeStat) (hw.Board, bool) {
	if st == nil || st.Board == "" {
		return hw.Board{}, false
	}
	b, err := hw.BoardByName(st.Board)
	if err != nil || b.Hardware == nil {
		return hw.Board{}, false
	}
	return b, true
}
