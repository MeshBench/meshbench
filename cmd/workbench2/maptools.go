// The map's own controls.
//
// The old map carried a toolbar - a node filter, a basemap picker, fit and
// terrain - and a tool strip down its left edge for select, move, place, link
// and measure. The Gio map had a layer panel and nothing else, so aiming it
// meant knowing the verb names.
package main

import (
	"gioui.org/layout"
	"gioui.org/widget"

	"github.com/A13xB0/meshcoresim/internal/gui/comp"
	"github.com/A13xB0/meshcoresim/internal/gui/state"
	"github.com/A13xB0/meshcoresim/internal/gui/theme"
)

type mapTools struct {
	filter comp.Field
	fit    comp.Button
	zoomIn comp.Button
	zoomUt comp.Button

	tools   [5]widget.Clickable
	current string

	mv    *comp.MapView
	snap  func() *state.Snapshot
	built bool
}

var toolNames = [5]string{"select", "move", "place", "link", "measure"}

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
		return layout.Flex{Alignment: layout.Middle}.Layout(gtx, kids...)
	})
}
