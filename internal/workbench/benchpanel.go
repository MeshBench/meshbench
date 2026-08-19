// The Companion bench: a mesh and an address to point a client at.
//
// The front door for somebody writing an application against MeshCore rather
// than studying a network. Everything it does existed already - firmware
// starts from a menu, a port is served from a node's own window - but not in
// one place that assumes you want an endpoint and gets you one.
package workbench

import (
	"sort"

	"gioui.org/layout"

	"github.com/MeshBench/meshbench/internal/gui/comp"
	"github.com/MeshBench/meshbench/internal/gui/state"
	"github.com/MeshBench/meshbench/internal/gui/theme"
)

// benchPanel lists the companions and what each is served on.
type benchPanel struct {
	tb   comp.Table
	init bool
	// OnAction is how the panel asks the store to do something. A panel does
	// not open sockets.
	OnAction func(action, node string)
}

func (p *benchPanel) Draw(t *theme.Theme, gtx layout.Context, s *state.Snapshot) layout.Dimensions {
	if !p.init {
		p.tb.Cols = []comp.Column{
			{Title: "companion", Width: 190, Sortable: true},
			{Title: "transport", Width: 96},
			{Title: "address", Width: 200, Mono: true},
			{Title: "client"},
		}
		p.init = true
	}
	if s == nil {
		return layout.Dimensions{}
	}

	var comps []string
	for i := range s.Nodes {
		if s.Nodes[i].Kind == "companion" {
			comps = append(comps, s.Nodes[i].Name)
		}
	}
	sort.Strings(comps)
	if len(comps) == 0 {
		return layout.Center.Layout(gtx, comp.Text(t, t.Sz.Body, t.P.Dim,
			"no companion in this network - every shipped fixture carries several"))
	}

	served := map[string]state.Endpoint{}
	for _, e := range s.Endpoints {
		served[e.Node] = e
	}
	rows := make([]comp.Row, 0, len(comps))
	for _, name := range comps {
		e, ok := served[name]
		kind, addr, client := "-", "not served", "-"
		if ok {
			kind, addr = e.Kind, e.Addr
			client = "waiting"
			if e.Attached {
				client = "attached"
			}
		}
		rows = append(rows, comp.Row{Key: name,
			Cells: []string{name, kind, addr, client}})
	}
	p.tb.SetRows(rows)

	// Buttons in a row, sized to their labels. Laid out as rigid children of
	// a vertical flex they stretched the full width of the panel, which made
	// every one of them look like the main action.
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(comp.SectionTitle(t, "a mesh and an endpoint")),
		layout.Rigid(comp.Text(t, t.Sz.Caption, t.P.Dim,
			"both transports carry the firmware's own serial protocol, byte for byte")),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return p.tb.Layout(t, gtx, nil)
		}),
	)
}
