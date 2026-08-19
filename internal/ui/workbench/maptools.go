// The map's own controls.
//
// The old map carried a toolbar - a node filter, a basemap picker, fit and
// terrain - and a tool strip down its left edge for select, move, place, link
// and measure. The Gio map had a layer panel and nothing else, so aiming it
// meant knowing the verb names.
package workbench

import (
	"gioui.org/layout"
	"gioui.org/widget"

	"github.com/MeshBench/meshbench/internal/ui/comp"
	"github.com/MeshBench/meshbench/internal/ui/theme"
)

type mapTools struct {
	filter comp.Field
	fit    comp.Button
	zoomIn comp.Button
	zoomUt comp.Button

	tools   [5]widget.Clickable
	current string
	// placeKind is what the place tool places, and the row of choices that
	// appears when place is the tool. The old workbench had a tool per kind;
	// one tool and a kind beside it says the same thing in less rail.
	placeKind string
	kinds     [4]widget.Clickable

	mv    *comp.MapView
	built bool
}

var toolNames = [5]string{"select", "move", "place", "link", "measure"}

// placeKinds are what the place tool can put down, in the scenario's own
// words. The same four the old workbench had as separate tools.
var placeKinds = [4]struct{ label, kind string }{
	{"repeater", "simple-repeater"},
	{"companion", "companion"},
	{"observer", "sdr-observer"},
	{"emitter", "emitter"},
}

// Draw is the toolbar. One row above the map, because the map is the thing
// being looked at and a sidebar of controls would take width from it.
func (m *mapTools) Draw(t *theme.Theme, gtx layout.Context) layout.Dimensions {
	if !m.built {
		m.filter.Hint = "filter nodes by name or kind"
		m.filter.Editor.SingleLine = true
		m.fit.Label, m.fit.Kind = "fit", comp.Secondary
		m.zoomIn.Label, m.zoomIn.Kind = "+", comp.Quiet
		m.zoomUt.Label, m.zoomUt.Kind = "-", comp.Quiet
		m.current = "select"
		// Said out loud rather than left implied. The map reads Tool to decide
		// what a click does, and an empty one while the toolbar shows "select"
		// highlighted is the toolbar and the map disagreeing.
		if m.mv != nil {
			m.mv.Tool = m.current
		}
		m.built = true
	}
	// The filter applies as it is typed: a box that needs a button pressed
	// after it is a box people think is broken.
	if m.mv != nil {
		m.mv.Filter = m.filter.Editor.Text()
	}
	if m.fit.Click.Clicked(gtx) && m.mv != nil {
		m.mv.FitNext = true
	}
	if m.zoomIn.Click.Clicked(gtx) && m.mv != nil {
		m.mv.Zoom *= 1.5
	}
	if m.zoomUt.Click.Clicked(gtx) && m.mv != nil {
		m.mv.Zoom /= 1.5
	}
	for i := range m.tools {
		if m.tools[i].Clicked(gtx) {
			m.current = toolNames[i]
			if m.mv != nil {
				m.mv.Tool = m.current
				// Leaving the link tool abandons its half-made pick: a
				// pick that survives switching to pan is a click three
				// minutes later completing a link nobody remembers starting.
				if m.current != "link" {
					m.mv.CancelLink()
				}
			}
		}
	}

	kids := []layout.FlexChild{
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Right: t.Sp.S}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return m.filter.Layout(t, gtx)
			})
		}),
	}
	for i, name := range toolNames {
		i, name := i, name
		kids = append(kids, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			fg := t.P.Dim
			if m.current == name {
				fg = t.P.Accent
			}
			return m.tools[i].Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Left: t.Sp.S, Right: t.Sp.S,
					Top: t.Sp.XS, Bottom: t.Sp.XS}.Layout(gtx,
					comp.Text(t, t.Sz.Caption, fg, name))
			})
		}))
	}
	for _, b := range []*comp.Button{&m.zoomUt, &m.zoomIn, &m.fit} {
		b := b
		kids = append(kids, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Left: t.Sp.XS}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return b.Layout(t, gtx)
			})
		}))
	}
	return layout.Inset{Bottom: t.Sp.XS}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Alignment: layout.Middle}.Layout(gtx, kids...)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return m.kindRow(t, gtx)
			}),
		)
	})
}

// kindRow is what the place tool will place, on a line of its own.
//
// Of its own because the toolbar is already a full row: four more labels
// beside the tools pushed link and measure off the end of a narrow map, which
// is a worse fault than the one the kinds were added to fix.
func (m *mapTools) kindRow(t *theme.Theme, gtx layout.Context) layout.Dimensions {
	if m.current != "place" {
		return layout.Dimensions{}
	}
	if m.placeKind == "" {
		m.placeKind = placeKinds[0].kind
	}
	kids := []layout.FlexChild{
		layout.Rigid(comp.Text(t, t.Sz.Caption, t.P.Faint, "place a ")),
	}
	for i := range placeKinds {
		i := i
		if m.kinds[i].Clicked(gtx) {
			m.placeKind = placeKinds[i].kind
		}
		kids = append(kids, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			fg := t.P.Dim
			if m.placeKind == placeKinds[i].kind {
				fg = t.P.Accent
			}
			return m.kinds[i].Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Left: t.Sp.XS, Right: t.Sp.XS}.Layout(gtx,
					comp.Text(t, t.Sz.Caption, fg, placeKinds[i].label))
			})
		}))
	}
	kids = append(kids, layout.Rigid(comp.Text(t, t.Sz.Caption, t.P.Faint,
		" - then click the map")))
	return layout.Flex{Alignment: layout.Middle}.Layout(gtx, kids...)
}
