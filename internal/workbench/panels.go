// The panels this binary provides, drawn from a snapshot.
package workbench

import (
	"gioui.org/layout"

	"github.com/A13xB0/meshcoresim/internal/gui/comp"
	"github.com/A13xB0/meshcoresim/internal/gui/state"
	"github.com/A13xB0/meshcoresim/internal/gui/theme"
)

// nodesPanel is the node list, on the new table component.
type nodesPanel struct {
	// OnSelect tells the rest of the workbench which node was picked.
	//
	// Without it this table set its own highlight and nothing else: the map
	// did not follow, the Inspector did not follow, and every control that
	// acts on "the selected node" acted on a different one. A list you can
	// click and that changes nothing is decoration.
	OnSelect func(node string)
	tbl      comp.Table
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
			return np.tbl.Layout(t, gtx, func(key string) {
				np.tbl.Selected = key
				if np.OnSelect != nil {
					np.OnSelect(key)
				}
			})
		}),
	)
}
