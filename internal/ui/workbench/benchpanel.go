// The Companion bench: a mesh and an address to point a client at.
//
// The front door for somebody writing an application against MeshCore rather
// than studying a network. Everything it does existed already - firmware
// starts from a menu, a port is served from a node's own window - but not in
// one place that assumes you want an endpoint and gets you one.
package workbench

import (
	"sort"
	"strings"

	"gioui.org/layout"

	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/ui/comp"
	"github.com/MeshBench/meshbench/internal/ui/theme"
)

// benchPanel lists the companions and what each is served on.
//
// It is also the App view's only way of saying which companion is being
// talked about. The view carries neither the map nor the node list, and
// those were the only two things that ever selected anything - so every verb
// needing a node refused, and the console beside this panel said "select a
// node" with nothing on screen able to.
type benchPanel struct {
	tb   comp.Table
	init bool
	// OnAction is how the panel asks the store to do something. A panel does
	// not open sockets.
	OnAction func(action, node string)
	// OnSelect says which companion the rest of the view is about.
	OnSelect func(node string)
	// rows are the per-row buttons, pooled by companion name: a widget
	// rebuilt each frame never sees the release that completes its press.
	rows map[string]*benchRow
}

// benchRow is one companion's buttons.
type benchRow struct {
	serve comp.Button
	// client opens the node's own window, which for a companion opens on
	// its client tab - the three-mode client that already exists rather
	// than a second one drawn here.
	client comp.Button
}

func (p *benchPanel) row(name string) *benchRow {
	if p.rows == nil {
		p.rows = map[string]*benchRow{}
	}
	r := p.rows[name]
	if r == nil {
		r = &benchRow{}
		r.serve.Label, r.serve.Kind = "serve TCP", comp.Secondary
		r.client.Label, r.client.Kind = "open client", comp.Quiet
		p.rows[name] = r
	}
	return r
}

func (p *benchPanel) Draw(t *theme.Theme, gtx layout.Context, s *state.Snapshot) layout.Dimensions {
	if !p.init {
		p.tb.Cols = []comp.Column{
			{Title: "companion", Width: 190, Sortable: true},
			{Title: "transport", Width: 96},
			// Wide, because it holds an address somebody has to read off
			// the screen and type into a client on another machine.
			{Title: "address", Width: 320, Mono: true},
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
	// The selection is the world's, not the table's: clicking a row fires the
	// same verb the map fires, so every panel in the view follows it.
	//
	// Only a companion counts as this panel's subject. The selection is the
	// whole application's, and a repeater picked on the map must not leave
	// this panel offering to serve a client to something that has no
	// companion protocol at all.
	p.tb.Selected = ""
	if n := selectedNodeName(s); n != "" {
		for _, c := range comps {
			if c == n {
				p.tb.Selected = n
				break
			}
		}
	}

	served := map[string]state.Endpoint{}
	for _, e := range s.Endpoints {
		served[e.Node] = e
	}
	rows := make([]comp.Row, 0, len(comps))
	for _, name := range comps {
		e, ok := served[name]
		// A dash for what has not happened, never a zero or an empty cell:
		// "not served" is a state this panel exists to change, and it reads
		// as one rather than as missing data.
		kind, addr, client := "-", "not served", "-"
		if ok {
			kind, addr = e.Kind, e.Addr
			// Every address it answers on. A machine on wifi and ethernet
			// has two and a phone can only reach one of them, so which to
			// type in is not this program's to guess - and the row under the
			// pointer says the whole list, for when the column runs out.
			if len(e.Addrs) > 1 {
				addr = strings.Join(e.Addrs, ", ")
			}
			client = "waiting"
			if e.Attached {
				client = "attached"
			}
		}
		rows = append(rows, comp.Row{Key: name,
			Cells: []string{name, kind, addr, client}})
	}
	p.tb.SetRows(rows)

	sel := p.tb.Selected
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(comp.SectionTitle(t, "a mesh and an endpoint")),
		layout.Rigid(comp.Text(t, t.Sz.Caption, t.P.Dim,
			"pick a companion: the console, the events and everything sent "+
				"are about whichever is selected")),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return p.tb.Layout(t, gtx, func(key string) {
				if p.OnSelect != nil {
					p.OnSelect(key)
				}
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return p.actions(t, gtx, sel, served)
		}),
	)
}

// actions is what can be done to the selected companion, said in its own
// name rather than as a row of verbs about nothing.
func (p *benchPanel) actions(t *theme.Theme, gtx layout.Context, sel string,
	served map[string]state.Endpoint) layout.Dimensions {
	if sel == "" {
		return layout.Inset{Top: t.Sp.S}.Layout(gtx,
			comp.Text(t, t.Sz.Caption, t.P.Faint,
				"no companion selected - click a row above"))
	}
	r := p.row(sel)
	e, isServed := served[sel]
	r.serve.Label = "serve TCP"
	if isServed {
		r.serve.Label = "drop clients"
	}
	if r.serve.Click.Clicked(gtx) && p.OnAction != nil {
		if isServed {
			p.OnAction("bench.drop", sel)
		} else {
			p.OnAction("serve.tcp", sel)
		}
	}
	if r.client.Click.Clicked(gtx) && p.OnAction != nil {
		p.OnAction("node.window", sel)
	}
	// The row under the pointer wins, so a list too long for its column can
	// be read in full without selecting anything.
	if h := p.tb.Hovered(); h != "" {
		if e, ok := served[h]; ok && len(e.Addrs) > 0 {
			return layout.Inset{Top: t.Sp.S}.Layout(gtx, comp.Text(t, t.Sz.Caption,
				t.P.Dim, h+" answers on "+strings.Join(e.Addrs, ", ")))
		}
	}
	note := sel + " is not served yet"
	if isServed {
		note = sel + " is on " + e.Addr + " - point a client at it"
		if len(e.Addrs) > 1 {
			note += ", or " + strings.Join(e.Addrs[1:], ", ")
		}
	}
	return layout.Inset{Top: t.Sp.S}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Right: t.Sp.S}.Layout(gtx,
					func(gtx layout.Context) layout.Dimensions {
						return r.serve.Layout(t, gtx)
					})
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Right: t.Sp.M}.Layout(gtx,
					func(gtx layout.Context) layout.Dimensions {
						return r.client.Layout(t, gtx)
					})
			}),
			layout.Flexed(1, comp.OneLine(t, t.Sz.Caption, t.P.Dim, note, false)),
		)
	})
}
