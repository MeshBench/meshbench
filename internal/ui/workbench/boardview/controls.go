// Everything on this board somebody can drive.
//
// The Hardware tab offers the buttons and the trackball; this offers those and
// the rest of what is wired - the card in the slot, the keyboard, the touch
// layer - in one column beside the table that says whether any of it is
// working. That pairing is the point: press a thing, then look at what the pin
// did.
//
// What is not here is as deliberate as what is. A board's meter has no control
// because nothing models the converter behind it, and a slider that set a
// voltage the firmware cannot read would be a control that lies. The wiring
// table says so on that row instead, which is the honest place for it.
package boardview

import (
	"gioui.org/layout"

	"github.com/MeshBench/meshbench/internal/app/state"
	hw "github.com/MeshBench/meshbench/internal/firmware/board"
	"github.com/MeshBench/meshbench/internal/ui/comp"
	"github.com/MeshBench/meshbench/internal/ui/theme"
)

// controls is what somebody can press, type at, and put in the slot.
func (p *Panel) controls(t *theme.Theme, gtx layout.Context, b hw.Board,
	s *state.Snapshot) layout.Dimensions {

	if !hasPanel(b) {
		return layout.Dimensions{}
	}
	var kids []layout.FlexChild
	add := func(w layout.Widget) {
		kids = append(kids, layout.Rigid(w))
	}
	add(func(gtx layout.Context) layout.Dimensions {
		return p.parts.Buttons(t, gtx, b.Hardware)
	})
	if len(b.Hardware.PartsOfKind(hw.Ball)) > 0 {
		add(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: t.Sp.XXS}.Layout(gtx,
				func(gtx layout.Context) layout.Dimensions {
					return p.parts.Ball(t, gtx, b.Hardware)
				})
		})
	}
	// Said rather than left to be discovered. Keys go to whatever has focus,
	// and a keyboard that silently needs a click first is one somebody decides
	// is broken - which is exactly what happened on the tab next door.
	if hasKeys(b) {
		add(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: t.Sp.XXS}.Layout(gtx,
				comp.Text(t, t.Sz.Caption, t.P.Faint,
					"click the panel, then type - keys go to the board's own keyboard"))
		})
	}
	if row := p.cardRow(t, gtx, b, s); row != nil {
		add(row)
	}
	if len(kids) == 0 {
		return layout.Dimensions{}
	}
	return layout.Inset{Top: t.Sp.XS, Bottom: t.Sp.XS}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx, kids...)
		})
}

// cardRow is the slot: whether a card is in it, and the way to wipe one.
//
// Two controls rather than the tab's whole card panel, which carries the file's
// path, a way to browse for another and three paragraphs about why a card
// survives a reflash. That belongs where somebody is choosing what to run; here
// the question is narrower - is there storage behind this pin - and the row
// answers it beside the pin.
func (p *Panel) cardRow(t *theme.Theme, gtx layout.Context, b hw.Board,
	s *state.Snapshot) layout.Widget {

	if len(b.Hardware.PartsOfKind(hw.Card)) == 0 {
		return nil
	}
	n := nodeRow(s, p.Node)
	if n == nil {
		return nil
	}
	p.cardIn.Bool.Value = n.CardFitted
	p.cardIn.Label = "card in the slot"
	p.wipeCard.Kind, p.wipeCard.Label = comp.Quiet, "erase"
	return func(gtx layout.Context) layout.Dimensions {
		if p.cardIn.Bool.Update(gtx) && p.OnDo != nil {
			p.OnDo("node.card", map[string]any{
				"node": p.Node, "fitted": p.cardIn.Bool.Value})
		}
		if p.wipeCard.Click.Clicked(gtx) && p.OnDo != nil {
			p.OnDo("node.card", map[string]any{"node": p.Node, "wipe": true})
		}
		return layout.Inset{Top: t.Sp.S}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return p.cardIn.LayoutSwitch(t, gtx)
				}),
				layout.Rigid(layout.Spacer{Width: t.Sp.S}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return p.wipeCard.Layout(t, gtx)
				}),
			)
		})
	}
}

// nodeRow is this node as the snapshot has it, for the fields that are the
// scenario's rather than the running process's.
func nodeRow(s *state.Snapshot, name string) *state.Node {
	if s == nil {
		return nil
	}
	for i := range s.Nodes {
		if s.Nodes[i].Name == name {
			return &s.Nodes[i]
		}
	}
	return nil
}
