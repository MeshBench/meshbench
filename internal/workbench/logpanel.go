// The Log panel: what has happened in this session, newest last.
//
// The store's own log rather than a second one kept by the interface: every verb
// that says something says it there, so a script and a click leave the same
// trace.
package workbench

import (
	"gioui.org/layout"
	"gioui.org/widget"

	"github.com/MeshBench/meshbench/internal/gui/comp"
	"github.com/MeshBench/meshbench/internal/gui/state"
	"github.com/MeshBench/meshbench/internal/gui/theme"
)

// logPanel is what has happened in this session, newest last.
//
// The store's own log rather than a second one kept by the interface: every
// verb that says something says it there, so a script and a click leave the
// same trace.
type logPanel struct {
	list widget.List
}

func (p *logPanel) Draw(t *theme.Theme, gtx layout.Context, s *state.Snapshot) layout.Dimensions {
	if s == nil || len(s.Log) == 0 {
		return layout.Center.Layout(gtx, comp.Text(t, t.Sz.Body, t.P.Dim,
			"nothing has happened yet"))
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(comp.SectionTitle(t, "experiment log")),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			p.list.Axis = layout.Vertical
			return comp.List(t, &p.list, len(s.Log),
				func(gtx layout.Context, i int) layout.Dimensions {
					// Oldest first, so reading downwards is reading forwards.
					return comp.Mono(t, t.Sz.Caption, t.P.Dim, s.Log[i])(gtx)
				})(gtx)
		}),
	)
}
