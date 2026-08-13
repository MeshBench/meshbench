// The panels this binary provides, drawn from a snapshot.
package main

import (
	"gioui.org/layout"
	"gioui.org/unit"

	"github.com/A13xB0/meshcoresim/internal/gui/comp"
	"github.com/A13xB0/meshcoresim/internal/gui/state"
	"github.com/A13xB0/meshcoresim/internal/gui/theme"
)

// nodesPanel is the node list, on the new table component.
type nodesPanel struct {
	tbl comp.Table
	// filter is kept here rather than built each frame.
	//
	// Gio tracks focus by the widget's address, so a Field declared inside
	// Draw is a different editor every frame: the click focuses one, and by
	// the time the characters arrive it no longer exists. The box drew
	// correctly and would not accept a single keystroke.
	filter comp.Field
	built  bool
	// seq is the snapshot this table was built from, and rowsSet says whether
	// it has ever been built. Without the second, a first snapshot whose
	// sequence happens to equal the zero value is skipped and the panel shows
	// an empty list of a network that is loaded.
	seq     uint64
	rowsSet bool
}

func (np *nodesPanel) Draw(t *theme.Theme, gtx layout.Context, s *state.Snapshot) layout.Dimensions {
	if !np.built {
		np.tbl.Cols = []comp.Column{
			{Title: "node", Sortable: true},
			{Title: "kind", Width: 96, Sortable: true},
			{Title: "heard", Width: 62, Right: true, Mono: true, Sortable: true},
		}
		np.built = true
	}
	if s != nil && (!np.rowsSet || s.Seq != np.seq) {
		rows := make([]comp.Row, 0, len(s.Nodes))
		for i := range s.Nodes {
			n := &s.Nodes[i]
			rows = append(rows, comp.Row{
				Key:   n.Name,
				Cells: []string{n.Name, shortKind(n.Kind), itoa(n.Heard)},
				Tint:  comp.Tint(t.NodeColour(kindOf(n.Kind))),
			})
		}
		np.tbl.SetRows(rows)
		np.seq, np.rowsSet = s.Seq, true
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			// The table's own editor, drawn with the field's chrome. One
			// widget at one address, which is what focus is keyed on.
			np.filter.Hint = "filter by name or kind"
			return np.filter.LayoutEditor(t, gtx, &np.tbl.Filter)
		}),
		layout.Rigid(layout.Spacer{Height: t.Sp.S}.Layout),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return np.tbl.Layout(t, gtx, func(key string) { np.tbl.Selected = key })
		}),
	)
}

func drawInspector(t *theme.Theme, gtx layout.Context, s *state.Snapshot) layout.Dimensions {
	if s == nil || len(s.Nodes) == 0 {
		return layout.Center.Layout(gtx,
			comp.Text(t, t.Sz.Caption, t.P.Faint, "nothing selected"))
	}
	n := s.Nodes[0]
	for i := range s.Nodes {
		if s.Nodes[i].Selected {
			n = s.Nodes[i]
			break
		}
	}
	rows := [][2]string{
		{"name", n.Name},
		{"kind", shortKind(n.Kind)},
		{"latitude", ftoa(n.Lat)},
		{"longitude", ftoa(n.Lon)},
		{"height", ftoa(n.HeightM) + " m above ground"},
		{"transmit power", ftoa(n.TxDBm) + " dBm"},
		{"regions", join(n.Regions)},
		{"firmware", n.Firmware},
	}
	children := make([]layout.FlexChild, 0, len(rows))
	for _, r := range rows {
		k, v := r[0], r[1]
		if v == "" {
			v = "none"
		}
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Bottom: t.Sp.XS}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						w := gtx.Dp(unit.Dp(110))
						gtx.Constraints.Min.X, gtx.Constraints.Max.X = w, w
						return comp.Text(t, t.Sz.Label, t.P.Dim, k)(gtx)
					}),
					layout.Flexed(1, comp.Text(t, t.Sz.Body, t.P.Ink, v)),
				)
			})
		}))
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
}
