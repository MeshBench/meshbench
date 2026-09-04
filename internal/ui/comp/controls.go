// Controls, on the panels that were tables.
//
// Every verb behind these already existed; what was missing was any way to
// reach one without a socket. A panel that displays and cannot act is a report,
// and the old workbench's equivalents were tools.
package comp

import (
	"strconv"
	"strings"

	"gioui.org/layout"

	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/ui/theme"
)

// ActionBar is a row of Fields and Buttons above a panel's table.
//
// Laid out as one row that wraps to a second when it must, because these are
// short forms - a node, a command, a number - and stacking every one of them
// vertically pushes the table off the bottom of a docked panel.
type ActionBar struct {
	Fields []*Field
	// Extras sit between the Fields and the Buttons: a dropdown, a switch -
	// any control that is neither a box nor a button but belongs in the row.
	// They take the theme at draw time, not at build time, so a theme change
	// reaches them like everything else.
	Extras  []func(t *theme.Theme, gtx layout.Context) layout.Dimensions
	Buttons []*Button
	Note    string
}

func (a *ActionBar) Layout(t *theme.Theme, gtx layout.Context) layout.Dimensions {
	// Wrap when the column is narrow.
	//
	// One row is right in a docked panel across the width of a window, and
	// wrong in the Inspector's column, where three Buttons were squeezed into
	// vertical strips one letter wide. A control nobody can read is not a
	// control, so below this width each one gets its own line.
	narrow := gtx.Constraints.Max.X < gtx.Dp(560)
	if narrow {
		return a.Stacked(t, gtx)
	}
	kids := make([]layout.FlexChild, 0, len(a.Fields)+len(a.Buttons))
	for _, f := range a.Fields {
		f := f
		kids = append(kids, layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Right: t.Sp.S}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return f.Layout(t, gtx)
			})
		}))
	}
	for _, e := range a.Extras {
		e := e
		kids = append(kids, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Max.X = min(gtx.Constraints.Max.X, gtx.Dp(220))
			return layout.Inset{Right: t.Sp.S}.Layout(gtx,
				func(gtx layout.Context) layout.Dimensions { return e(t, gtx) })
		}))
	}
	for _, b := range a.Buttons {
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
			if a.Note == "" {
				return layout.Dimensions{}
			}
			return layout.Inset{Top: t.Sp.XS, Bottom: t.Sp.XS}.Layout(gtx,
				Text(t, t.Sz.Caption, t.P.Faint, a.Note))
		}),
		layout.Rigid(layout.Spacer{Height: t.Sp.S}.Layout),
	)
}

// Num reads a field as a number, treating empty as absent rather than as zero:
// "send every 0 ms" is not what an empty box means.
func Num(f *Field) (float64, bool) {
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

func FieldText(f *Field) string { return strings.TrimSpace(f.Editor.Text()) }

// SelectedNodeName is what most of these act on when no node is typed.
func SelectedNodeName(s *state.Snapshot) string {
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

// stacked is the narrow-column form: Fields full width, one button per line.
func (a *ActionBar) Stacked(t *theme.Theme, gtx layout.Context) layout.Dimensions {
	kids := make([]layout.FlexChild, 0, len(a.Fields)+len(a.Buttons)+1)
	for _, f := range a.Fields {
		f := f
		kids = append(kids, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Bottom: t.Sp.XS}.Layout(gtx,
				func(gtx layout.Context) layout.Dimensions { return f.Layout(t, gtx) })
		}))
	}
	for _, e := range a.Extras {
		e := e
		kids = append(kids, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Bottom: t.Sp.XS}.Layout(gtx,
				func(gtx layout.Context) layout.Dimensions { return e(t, gtx) })
		}))
	}
	for _, b := range a.Buttons {
		b := b
		kids = append(kids, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Bottom: t.Sp.XS}.Layout(gtx,
				func(gtx layout.Context) layout.Dimensions { return b.Layout(t, gtx) })
		}))
	}
	if a.Note != "" {
		kids = append(kids, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Bottom: t.Sp.S}.Layout(gtx,
				Text(t, t.Sz.Caption, t.P.Faint, a.Note))
		}))
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx, kids...)
}

func SplitFields(s string) []string {
	var out []string
	out = append(out, fieldsOf(s)...)
	return out
}

func fieldsOf(s string) []string {
	var out, cur []rune
	_ = out
	var res []string
	for _, r := range s {
		if r == ' ' || r == ',' || r == '\t' {
			if len(cur) > 0 {
				res = append(res, string(cur))
				cur = cur[:0]
			}
			continue
		}
		cur = append(cur, r)
	}
	if len(cur) > 0 {
		res = append(res, string(cur))
	}
	return res
}
