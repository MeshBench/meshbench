// The map and the panels that answer for one selected thing.
package workbench

import (
	"gioui.org/layout"

	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/ui/shell"
	"github.com/MeshBench/meshbench/internal/ui/theme"
)

func addMapPanels(d panelDeps) {
	d.sh.Add(&shell.Panel{Name: "Map", Windowable: true,
		Draw: func(t *theme.Theme, gtx layout.Context, s *state.Snapshot) layout.Dimensions {
			d.wbUI.applyCamera()
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return d.mapTop.Draw(t, gtx)
				}),
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					return d.mv.Layout(t, gtx, s)
				}),
			)
		}})
	d.sh.Add(&shell.Panel{Name: "Nodes", Windowable: true, Draw: d.nodes.Draw})
	// The Inspector is the events panel's light form, scoped to the
	// selection: what this node has said and heard, not a form about it.
	insp := &eventsPanel{compact: true, forNode: true, OnOpenPacket: d.openPacket}
	d.sh.Add(&shell.Panel{Name: "Inspector", Windowable: true, Draw: insp.Draw})
	pkt := &packetPanel{do: d.do}
	d.sh.Add(&shell.Panel{Name: "Packet", Windowable: true, Draw: pkt.Draw})
}
