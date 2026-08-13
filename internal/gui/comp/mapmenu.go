package comp

import (
	"image"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/widget"

	"github.com/A13xB0/meshcoresim/internal/gui/theme"
)

// The right-click menu (10.8d).
//
// Its entries are named for what they do to the network, not for the verb they
// call, and an entry that cannot apply here is absent rather than greyed: a
// menu on empty water offering "coverage from here" would be offering
// something the model has no node to answer for.

// MenuItem is one entry, and the verb it stands for.
type MenuItem struct {
	Label string
	// Action is what the shell should do. A string rather than a closure so
	// the same menu can be driven from a script, and so this file does not
	// need to know what a store is.
	Action string
	click  widget.Clickable
}

type mapMenu struct {
	open  bool
	at    image.Point
	node  string
	lat   float64
	lon   float64
	items []MenuItem
	// box is where the menu was last drawn, in map pixels.
	//
	// The map dismisses an open menu on any press, which is what clicking
	// away from a menu means - and it was also doing it for the press that
	// lands on an entry. The entry's own click was registered and then never
	// read, because the menu had closed by the frame that would have read it.
	// So the map has to know where the menu is in order to leave it alone.
	box image.Rectangle
}

// menuFor is the list for a right-click, which depends on whether it landed on
// a node.
func menuFor(node string) []MenuItem {
	if node != "" {
		return []MenuItem{
			{Label: "Centre on this node", Action: "map.centre"},
			{Label: "Coverage from here", Action: "coverage.compute"},
			{Label: "Show only this node's neighbours", Action: "map.neighbours"},
			{Label: "Originate a packet here", Action: "sim.inject"},
			{Label: "Open in its own window", Action: "node.window"},
		}
	}
	return []MenuItem{
		{Label: "Fit the whole network", Action: "map.fit"},
		{Label: "Centre here", Action: "map.centre_here"},
		{Label: "Clear the coverage overlay", Action: "coverage.clear"},
	}
}

// layoutMenu draws the menu if it is open, and reports a chosen action.
func (m *MapView) layoutMenu(t *theme.Theme, gtx layout.Context, sz image.Point) {
	if !m.menu.open {
		return
	}
	for i := range m.menu.items {
		if m.menu.items[i].click.Clicked(gtx) {
			m.menu.open = false
			if m.OnMenu != nil {
				m.OnMenu(m.menu.items[i].Action, m.menu.node, m.menu.lat, m.menu.lon)
			}
			return
		}
	}

	pad := gtx.Dp(t.Sp.S)
	inner := gtx
	inner.Constraints.Min = image.Point{}
	inner.Constraints.Max = image.Pt(gtx.Dp(280), sz.Y)

	rec := op.Record(gtx.Ops)
	var kids []layout.FlexChild
	for i := range m.menu.items {
		i := i
		kids = append(kids, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return material_clickable(t, &m.menu.items[i].click,
				m.menu.items[i].Label)(gtx)
		}))
	}
	dims := layout.Flex{Axis: layout.Vertical}.Layout(inner, kids...)
	content := rec.Stop()

	box := image.Pt(dims.Size.X+pad*2, dims.Size.Y+pad*2)
	// Kept on screen: a menu opened near the right edge otherwise runs off it.
	at := m.menu.at
	if at.X+box.X > sz.X {
		at.X = sz.X - box.X
	}
	if at.Y+box.Y > sz.Y {
		at.Y = sz.Y - box.Y
	}
	m.menu.box = image.Rectangle{Min: at, Max: at.Add(box)}
	off := op.Offset(at).Push(gtx.Ops)
	defer off.Pop()
	paint.FillShape(gtx.Ops, theme.Alpha(t.P.Panel, 0.97), clip.Rect{Max: box}.Op())
	Border(gtx, box, 2, 1, t.P.Rule)
	in := op.Offset(image.Pt(pad, pad)).Push(gtx.Ops)
	content.Add(gtx.Ops)
	in.Pop()
}

// material_clickable is a menu row: the whole row is the target, not the text.
func material_clickable(t *theme.Theme, c *widget.Clickable, label string) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		return c.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			if c.Hovered() {
				paint.FillShape(gtx.Ops, theme.Alpha(t.P.Accent, 0.18),
					clip.Rect{Max: image.Pt(gtx.Constraints.Max.X,
						gtx.Dp(t.RowHeight()))}.Op())
			}
			return layout.Inset{Left: t.Sp.S, Right: t.Sp.S,
				Top: t.Sp.XS, Bottom: t.Sp.XS}.Layout(gtx,
				Text(t, t.Sz.Body, t.P.Ink, label))
		})
	}
}
