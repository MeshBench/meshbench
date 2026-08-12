// The Companion bench: a mesh and an address to point a client at (6.25).
//
// The front door for somebody writing an application against MeshCore rather
// than studying a network. Everything it does existed already - firmware
// starts from a menu, a port is served from a node's own window - but not in
// one place that assumes you want an endpoint and gets you one.
package main

import (
	"fmt"
	"sort"

	"gioui.org/layout"

	"github.com/A13xB0/meshcoresim/internal/engine"
	"github.com/A13xB0/meshcoresim/internal/gui/comp"
	"github.com/A13xB0/meshcoresim/internal/gui/state"
	"github.com/A13xB0/meshcoresim/internal/gui/theme"
)

// benchPanel lists the companions and what each is served on.
type benchPanel struct {
	tb     comp.Table
	init   bool
	serveT comp.Button
	serveS comp.Button
	drop   comp.Button
	stray  comp.Button
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
		// One primary: there is one obvious thing to do here, and four
		// equally loud buttons make somebody read all four to find it.
		p.serveT.Label, p.serveT.Kind = "serve over TCP", comp.Primary
		p.serveS.Label, p.serveS.Kind = "serve as a serial device", comp.Secondary
		p.drop.Label, p.drop.Kind = "drop every client connection", comp.Destructive
		p.stray.Label, p.stray.Kind = "inject a stray frame", comp.Secondary
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
	act := func(b *comp.Button, action string) layout.FlexChild {
		return layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if b.Click.Clicked(gtx) && p.OnAction != nil {
				p.OnAction(action, p.tb.Selected)
			}
			gtx.Constraints.Min.X = 0
			return layout.Inset{Right: t.Sp.S, Top: t.Sp.XS, Bottom: t.Sp.XS}.Layout(
				gtx, func(gtx layout.Context) layout.Dimensions {
					return b.Layout(t, gtx)
				})
		})
	}
	row := func(kids ...layout.FlexChild) layout.FlexChild {
		return layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{}.Layout(gtx, kids...)
		})
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(comp.SectionTitle(t, "a mesh and an endpoint")),
		layout.Rigid(comp.Text(t, t.Sz.Caption, t.P.Dim,
			"both transports carry the firmware's own serial protocol, byte for byte")),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return p.tb.Layout(t, gtx, nil)
		}),
		row(act(&p.serveT, "serve.tcp"), act(&p.serveS, "serve.serial")),
		layout.Rigid(comp.SectionTitle(t, "faults")),
		layout.Rigid(comp.Text(t, t.Sz.Caption, t.P.Dim,
			"the two the workbench can actually cause; radio faults are the map's business")),
		row(act(&p.drop, "bench.drop"), act(&p.stray, "bench.stray")),
	)
}

// serve opens an endpoint for one companion.
func (s *sim) serve(name, kind string) (state.Endpoint, error) {
	if s.eng == nil {
		return state.Endpoint{}, fmt.Errorf("no simulation")
	}
	var (
		link *engine.CompanionLink
		err  error
	)
	if kind == "serial" {
		link, err = s.eng.ServeCompanionSerial(name)
	} else {
		// Port zero: the operating system picks a free one and the link
		// reports it. A fixed default collides with the last run that has not
		// finished dying, and that error reads like a permissions problem.
		link, err = s.eng.ServeCompanionTCP(name, "127.0.0.1:0")
	}
	if err != nil {
		return state.Endpoint{}, err
	}
	if s.served == nil {
		s.served = map[string]*engine.CompanionLink{}
	}
	s.served[name] = link
	return state.Endpoint{Node: name, Kind: link.Kind, Addr: link.Addr}, nil
}

// endpoints is what is currently served, with whether anything is attached.
func (s *sim) endpoints() []state.Endpoint {
	out := make([]state.Endpoint, 0, len(s.served))
	for name, l := range s.served {
		out = append(out, state.Endpoint{
			Node: name, Kind: l.Kind, Addr: l.Addr, Attached: l.Attached(),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Node < out[j].Node })
	return out
}

// dropClients unplugs every attached client.
//
// The listener goes with the connection, so this is "the device was unplugged"
// rather than "the link glitched". An application that reconnects cleanly from
// this is one that survives a phone going to sleep.
func (s *sim) dropClients() int {
	var names []string
	for name, l := range s.served {
		if l.Attached() {
			names = append(names, name)
		}
	}
	for _, name := range names {
		_ = s.served[name].Close()
		delete(s.served, name)
	}
	return len(names)
}
