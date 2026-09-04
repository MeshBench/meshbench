// The panels this binary provides, drawn from a snapshot.
package workbench

import (
	"fmt"

	"gioui.org/layout"
	"gioui.org/op"

	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/ui/comp"
	"github.com/MeshBench/meshbench/internal/ui/theme"
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
	// said is the row the foot of the panel named last frame. The footer is a
	// rigid and the table is the flex's flexed child, so the footer is
	// measured before the table it reads: the row that has just come under the
	// pointer is named a frame later, and this is how that frame is asked for.
	said string
	// selected is the workbench's selection this frame, so the footer can
	// hold a name after the pointer has moved off the row.
	selected string
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
		// The count comes from the stats, which is the only place it is
		// measured. This column used to read a field on the node that nothing
		// ever wrote, so every row said nought however busy the mesh was, and
		// there was no way to tell that from a node which had heard nothing.
		heard := make(map[string]int, len(s.Stats))
		for _, st := range s.Stats {
			heard[st.Name] = st.Heard
		}
		rows := make([]comp.Row, 0, len(s.Nodes))
		for i := range s.Nodes {
			n := &s.Nodes[i]
			rows = append(rows, comp.Row{
				Key:   n.Name,
				Cells: []string{n.Name, shortKind(n.Kind), itoa(heard[n.Name])},
				Tint:  comp.Tint(t.NodeColour(kindOf(n.Kind))),
			})
		}
		np.tbl.SetRows(rows)
		np.seq, np.rowsSet = s.Seq, true
	}
	// The selection the whole workbench is on, not the table's own idea of
	// it: a node picked on the map or by a verb is the one somebody is
	// working on, and the foot of this panel is where its name fits.
	np.selected = comp.SelectedNodeName(s)
	d := layout.Flex{Axis: layout.Vertical}.Layout(gtx,
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
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return np.footer(t, gtx, s)
		}),
	)
	if np.said != np.pointedAt() {
		gtx.Execute(op.InvalidateCmd{})
	}
	return d
}

// pointedAt is the row the footer should be naming: the one under the
// pointer, or the selection when the pointer is elsewhere.
func (np *nodesPanel) pointedAt() string {
	if h := np.tbl.Hovered(); h != "" {
		return h
	}
	if np.selected != "" {
		return np.selected
	}
	return np.tbl.Selected
}

// footer reads out the row under the pointer in full, and says how many rows
// there are when there is no row under the pointer.
//
// A name column in a 340dp rail cuts "Abernethy Repeater" and
// "Abernethy Repeater 2" to the same fourteen characters, and a table whose
// rows cannot be told apart is a table that cannot be used. The selection
// holds the line when the pointer leaves, so the answer survives long enough
// to be read.
func (np *nodesPanel) footer(t *theme.Theme, gtx layout.Context, s *state.Snapshot) layout.Dimensions {
	name := np.pointedAt()
	np.said = name
	line, col := name, t.P.Ink
	if line == "" {
		total := 0
		if s != nil {
			total = len(s.Nodes)
		}
		line, col = fmt.Sprintf("%d of %d nodes - point at a row for the whole name",
			np.tbl.Shown(), total), t.P.Faint
	}
	return layout.Inset{Top: t.Sp.XS, Left: t.Sp.S, Right: t.Sp.S}.Layout(gtx,
		comp.OneLine(t, t.Sz.Caption, col, line, false))
}
