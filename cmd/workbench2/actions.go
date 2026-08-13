// Controls, on the panels that were tables.
//
// Every verb behind these already existed; what was missing was any way to
// reach one without a socket. A panel that displays and cannot act is a report,
// and the old workbench's equivalents were tools.
package main

import (
	"strconv"
	"strings"

	"gioui.org/layout"

	"github.com/A13xB0/meshcoresim/internal/gui/comp"
	"github.com/A13xB0/meshcoresim/internal/gui/state"
	"github.com/A13xB0/meshcoresim/internal/gui/theme"
)

// actionBar is a row of fields and buttons above a panel's table.
//
// Laid out as one row that wraps to a second when it must, because these are
// short forms - a node, a command, a number - and stacking every one of them
// vertically pushes the table off the bottom of a docked panel.
type actionBar struct {
	fields []*comp.Field
	// extras sit between the fields and the buttons: a dropdown, a switch -
	// any control that is neither a box nor a button but belongs in the row.
	// They take the theme at draw time, not at build time, so a theme change
	// reaches them like everything else.
	extras  []func(t *theme.Theme, gtx layout.Context) layout.Dimensions
	buttons []*comp.Button
	note    string
}

func (a *actionBar) layout(t *theme.Theme, gtx layout.Context) layout.Dimensions {
	// Wrap when the column is narrow.
	//
	// One row is right in a docked panel across the width of a window, and
	// wrong in the Inspector's column, where three buttons were squeezed into
	// vertical strips one letter wide. A control nobody can read is not a
	// control, so below this width each one gets its own line.
	narrow := gtx.Constraints.Max.X < gtx.Dp(560)
	if narrow {
		return a.stacked(t, gtx)
	}
	kids := make([]layout.FlexChild, 0, len(a.fields)+len(a.buttons))
	for _, f := range a.fields {
		f := f
		kids = append(kids, layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Right: t.Sp.S}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return f.Layout(t, gtx)
			})
		}))
	}
	for _, e := range a.extras {
		e := e
		kids = append(kids, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Max.X = min(gtx.Constraints.Max.X, gtx.Dp(220))
			return layout.Inset{Right: t.Sp.S}.Layout(gtx,
				func(gtx layout.Context) layout.Dimensions { return e(t, gtx) })
		}))
	}
	for _, b := range a.buttons {
		b := b
		kids = append(kids, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Right: t.Sp.XS}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return b.Layout(t, gtx)
			})
		}))
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Alignment: layout.Middle}.Layout(gtx, kids...)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if a.note == "" {
				return layout.Dimensions{}
			}
			return layout.Inset{Top: t.Sp.XS, Bottom: t.Sp.XS}.Layout(gtx,
				comp.Text(t, t.Sz.Caption, t.P.Faint, a.note))
		}),
		layout.Rigid(layout.Spacer{Height: t.Sp.S}.Layout),
	)
}

// num reads a field as a number, treating empty as absent rather than as zero:
// "send every 0 ms" is not what an empty box means.
func num(f *comp.Field) (float64, bool) {
	s := strings.TrimSpace(f.Editor.Text())
	if s == "" {
		return 0, false
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, false
	}
	return v, true
}

func fieldText(f *comp.Field) string { return strings.TrimSpace(f.Editor.Text()) }

// selectedNodeName is what most of these act on when no node is typed.
func selectedNodeName(s *state.Snapshot) string {
	if s == nil {
		return ""
	}
	for i := range s.Nodes {
		if s.Nodes[i].Selected {
			return s.Nodes[i].Name
		}
	}
	return ""
}

// stacked is the narrow-column form: fields full width, one button per line.
func (a *actionBar) stacked(t *theme.Theme, gtx layout.Context) layout.Dimensions {
	kids := make([]layout.FlexChild, 0, len(a.fields)+len(a.buttons)+1)
	for _, f := range a.fields {
		f := f
		kids = append(kids, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Bottom: t.Sp.XS}.Layout(gtx,
				func(gtx layout.Context) layout.Dimensions { return f.Layout(t, gtx) })
		}))
	}
	for _, e := range a.extras {
		e := e
		kids = append(kids, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Bottom: t.Sp.XS}.Layout(gtx,
				func(gtx layout.Context) layout.Dimensions { return e(t, gtx) })
		}))
	}
	for _, b := range a.buttons {
		b := b
		kids = append(kids, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Bottom: t.Sp.XS}.Layout(gtx,
				func(gtx layout.Context) layout.Dimensions { return b.Layout(t, gtx) })
		}))
	}
	if a.note != "" {
		kids = append(kids, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Bottom: t.Sp.S}.Layout(gtx,
				comp.Text(t, t.Sz.Caption, t.P.Faint, a.note))
		}))
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx, kids...)
}
