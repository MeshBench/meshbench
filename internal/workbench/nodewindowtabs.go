// The tabs of a node window: what each one draws, and the settings form that
// is the only one of them able to change anything.
package workbench

import (
	"fmt"
	"strings"

	"gioui.org/layout"
	"github.com/MeshBench/meshbench/internal/gui/comp"
	"github.com/MeshBench/meshbench/internal/gui/state"
	"github.com/MeshBench/meshbench/internal/gui/theme"
)

// settings is what this node is: identity, radio, regions and firmware - the
// mock's card, from what the snapshot honestly carries.
func (p *nodeWindowPanel) settings(t *theme.Theme, gtx layout.Context, s *state.Snapshot) layout.Dimensions {
	var node *state.Node
	if s != nil {
		for i := range s.Nodes {
			if s.Nodes[i].Name == p.node {
				node = &s.Nodes[i]
				break
			}
		}
	}
	if node == nil {
		return layout.Center.Layout(gtx, comp.Text(t, t.Sz.Caption, t.P.Faint,
			"this node is not in the loaded network"))
	}
	st := p.statFor(s)
	if p.energy.Click.Clicked(gtx) && p.OnAction != nil {
		// December is the question a solar node answers with its worst day.
		p.OnAction("node.energy", p.node)
	}

	head := func(sec string) layout.Widget {
		return func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: t.Sp.S, Bottom: t.Sp.XS}.Layout(gtx,
				func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
						layout.Rigid(comp.Text(t, t.Sz.Caption, t.P.Dim, strings.ToUpper(sec))),
						layout.Rigid(layout.Spacer{Width: t.Sp.S}.Layout),
						layout.Flexed(1, comp.HRule(t)),
					)
				})
		}
	}
	fw := orDash(node.Firmware)
	fwNote := "change the build from the Nodes running panel"
	if st != nil && st.Backend != "" {
		fwNote = st.Backend + " - " + fwNote
	}
	rows := []layout.Widget{
		comp.Card(t, "", func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
						layout.Rigid(comp.Text(t, t.Sz.Section, t.P.Ink, node.Name)),
						layout.Rigid(layout.Spacer{Width: t.Sp.S}.Layout),
						layout.Rigid(comp.Pill(t, t.P.Accent, shortKind(node.Kind))),
					)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return comp.CellGrid(t, gtx, 170, []layout.Widget{
						comp.StatCell(t, "Position",
							fmt.Sprintf("%.5f, %.5f", node.Lat, node.Lon), ""),
						comp.StatCell(t, "Height",
							fmt.Sprintf("%.0f m", node.HeightM), "above ground"),
						comp.StatCell(t, "Transmit power",
							fmt.Sprintf("%.0f dBm", node.TxDBm), ""),
					})
				}),
			)
		}),
		head("regions (observed)"),
		func(gtx layout.Context) layout.Dimensions {
			line := "none - this node relays only what its defaults allow"
			if len(node.Regions) > 0 {
				line = strings.Join(node.Regions, "  ")
			}
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(comp.Text(t, t.Sz.Body, t.P.Ink, line)),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					if node.DefaultScope == "" {
						return layout.Dimensions{}
					}
					return comp.Text(t, t.Sz.Caption, t.P.Faint,
						"default scope: "+node.DefaultScope)(gtx)
				}),
			)
		},
		head("firmware"),
		func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(comp.Mono(t, t.Sz.Body, t.P.Ink, fw)),
				layout.Rigid(comp.Text(t, t.Sz.Caption, t.P.Faint, fwNote)),
			)
		},
	}
	p.setList.Axis = layout.Vertical
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return comp.List(t, &p.setList, len(rows), func(gtx layout.Context, i int) layout.Dimensions {
				return layout.Inset{Bottom: t.Sp.XS}.Layout(gtx, rows[i])
			})(gtx)
		}),
		// Pinned under the list, not the last row of it: a question this good
		// should not need scrolling to find.
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			p.energy.Label, p.energy.Kind = "does it survive December?", comp.Secondary
			return layout.Inset{Top: t.Sp.S}.Layout(gtx,
				func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return p.energy.Layout(t, gtx)
						}),
					)
				})
		}),
	)
}

// connect serves this companion to a real client - meshcore-cli, a phone app
// over a bridge - which is the whole reason a simulated companion exists.
func (p *nodeWindowPanel) connect(t *theme.Theme, gtx layout.Context, s *state.Snapshot) layout.Dimensions {
	if p.tcpBtn.Click.Clicked(gtx) && p.OnServe != nil {
		p.OnServe(p.node, "tcp")
	}
	if p.serBtn.Click.Clicked(gtx) && p.OnServe != nil {
		p.OnServe(p.node, "serial")
	}
	if p.dropBtn.Click.Clicked(gtx) && p.OnAction != nil {
		p.OnAction("bench.drop", p.node)
	}
	p.tcpBtn.Label, p.tcpBtn.Kind = "serve over TCP", comp.Primary
	p.serBtn.Label, p.serBtn.Kind = "serve a serial port", comp.Secondary
	p.dropBtn.Label, p.dropBtn.Kind = "drop clients", comp.Quiet

	var mine []state.Endpoint
	if s != nil {
		for _, e := range s.Endpoints {
			if e.Node == p.node {
				mine = append(mine, e)
			}
		}
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(comp.Text(t, t.Sz.Caption, t.P.Dim,
			"serve this node to a real client - meshcore-cli or an app over a "+
				"bridge - exactly as real hardware would appear")),
		layout.Rigid(layout.Spacer{Height: t.Sp.S}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{Right: t.Sp.S}.Layout(gtx,
						func(gtx layout.Context) layout.Dimensions { return p.tcpBtn.Layout(t, gtx) })
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{Right: t.Sp.S}.Layout(gtx,
						func(gtx layout.Context) layout.Dimensions { return p.serBtn.Layout(t, gtx) })
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return p.dropBtn.Layout(t, gtx)
				}),
			)
		}),
		layout.Rigid(layout.Spacer{Height: t.Sp.S}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if len(mine) == 0 {
				return comp.Text(t, t.Sz.Caption, t.P.Faint,
					"not served yet - the endpoint appears here once it is")(gtx)
			}
			var kids []layout.FlexChild
			for _, e := range mine {
				e := e
				kids = append(kids, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					attached := "waiting for a client"
					col := t.P.Dim
					if e.Attached {
						attached, col = "client attached", t.P.Good
					}
					return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
						layout.Rigid(comp.Mono(t, t.Sz.Body, t.P.Ink, e.Kind+"  "+e.Addr)),
						layout.Rigid(layout.Spacer{Width: t.Sp.M}.Layout),
						layout.Rigid(comp.Text(t, t.Sz.Caption, col, attached)),
					)
				}))
			}
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx, kids...)
		}),
		layout.Rigid(layout.Spacer{Height: t.Sp.S}.Layout),
		layout.Rigid(comp.Text(t, t.Sz.Caption, t.P.Faint,
			"the Companion tab drives the node in-process; this serves it to "+
				"clients outside the workbench, and both cannot hold the port at once")),
	)
}

// stats is what this node costs and what it has carried.
func (p *nodeWindowPanel) stats(t *theme.Theme, gtx layout.Context, s *state.Snapshot) layout.Dimensions {
	st := p.statFor(s)
	if st == nil {
		return layout.Center.Layout(gtx, comp.Text(t, t.Sz.Caption, t.P.Faint,
			"no statistics for this node yet"))
	}
	rows := [][2]string{
		{"firmware", orDash(st.Firmware)},
		{"backend", orDash(st.Backend)},
		{"memory", siBytes(st.RSSBytes)},
		{"processor time", cpuTime(st.CPUms)},
		{"processor now", fmt.Sprintf("%.1f%%", st.CPUPct)},
		{"sent", fmt.Sprintf("%d", st.Sent)},
		{"heard", fmt.Sprintf("%d", st.Heard)},
		{"last sent", lastPacket(st.LastSentMs, st.LastSentTo)},
		{"last heard", lastPacket(st.LastHeardMs, st.LastHeardFrom)},
		{"radio busy", busyPct(*st)},
		{"spurious interrupts", fmt.Sprintf("%d", st.Spurious)},
	}
	p.statList.Axis = layout.Vertical
	return comp.List(t, &p.statList, len(rows), func(gtx layout.Context, i int) layout.Dimensions {
		return layout.Flex{}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints.Min.X = gtx.Dp(170)
				return comp.Text(t, t.Sz.Body, t.P.Dim, rows[i][0])(gtx)
			}),
			layout.Flexed(1, comp.Mono(t, t.Sz.Data, t.P.Ink, rows[i][1])),
		)
	})(gtx)
}

// activity is every event this node was an end of; clicking one opens its
// packet.
func (p *nodeWindowPanel) activity(t *theme.Theme, gtx layout.Context, s *state.Snapshot) layout.Dimensions {
	if !p.built {
		p.acts.Cols = []comp.Column{
			{Title: "at", Width: 76, Right: true, Mono: true, Sortable: true},
			{Title: "", Width: 44},
			{Title: "with", Width: 170, Sortable: true},
			{Title: "detail"},
		}
		p.acts.SortCol, p.acts.SortDesc, p.built = 0, true, true
	}
	if s != nil && (!p.rowsSet || s.Seq != p.seq) {
		rows := make([]comp.Row, 0, 64)
		for i := range s.Events {
			e := &s.Events[i]
			if e.From != p.node && e.To != p.node {
				continue
			}
			other := e.To
			if e.To == p.node {
				other = e.From
			}
			rows = append(rows, comp.Row{
				Key: fmt.Sprintf("%d/%d", e.PacketID, i),
				Cells: []string{
					fmt.Sprintf("%8.3f", float64(e.AtMs)/1000),
					e.Kind, other, e.Detail,
				},
			})
		}
		p.acts.SetRows(rows)
		p.seq, p.rowsSet = s.Seq, true
	}
	return p.acts.Layout(t, gtx, func(key string) {
		// A row is a packet; clicking it opens the packet view, the same as
		// everywhere else an event appears.
		if p.OnOpenPacket == nil {
			return
		}
		id, _, ok := strings.Cut(key, "/")
		if !ok {
			return
		}
		if v := atof(id); v > 0 {
			p.OnOpenPacket(uint64(v))
		}
	})
}

// console is the scrollback and the box to type into.
func (p *nodeWindowPanel) console(t *theme.Theme, gtx layout.Context, s *state.Snapshot) layout.Dimensions {
	var lines []string
	if s != nil {
		lines = s.Consoles[p.node]
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			if len(lines) == 0 {
				hint := "nothing printed yet - start the node, or type a command"
				if p.isCompanion() {
					hint = "a simulated companion. Type ? for what meshcore-cli " +
						"commands this build answers."
				}
				return layout.Center.Layout(gtx, comp.Text(t, t.Sz.Caption, t.P.Faint, hint))
			}
			p.list.Axis = layout.Vertical
			// Anchored at the end: a console is read from the bottom.
			p.list.ScrollToEnd = true
			return p.list.Layout(gtx, len(lines), func(gtx layout.Context, i int) layout.Dimensions {
				return comp.Mono(t, t.Sz.Data, t.P.Ink, lines[i])(gtx)
			})
		}),
		layout.Rigid(layout.Spacer{Height: t.Sp.XS}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					p.input.Hint = "a MeshCore command, then Enter"
					if p.isCompanion() {
						p.input.Hint = "a meshcore-cli command, then Enter - ? for the list"
					}
					p.input.Editor.SingleLine = true
					p.input.Editor.Submit = true
					return p.input.Layout(t, gtx)
				}),
				layout.Rigid(layout.Spacer{Width: t.Sp.S}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					p.send.Label, p.send.Kind = "send", comp.Primary
					return p.send.Layout(t, gtx)
				}),
			)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			note := "replies do not arrive while a sweep owns the clock: the " +
				"firmware only speaks when the engine steps it"
			if p.isCompanion() {
				note = "meshcore-cli, against a simulated companion. Commands it " +
					"has and this does not are refused by name rather than ignored."
			}
			return comp.OneLine(t, t.Sz.Caption, t.P.Faint, note, false)(gtx)
		}),
	)
}

func (p *nodeWindowPanel) statFor(s *state.Snapshot) *state.NodeStat {
	if s == nil {
		return nil
	}
	for i := range s.Stats {
		if s.Stats[i].Name == p.node {
			return &s.Stats[i]
		}
	}
	return nil
}

// isCompanion decides which command line and which tabs this node gets.
func (p *nodeWindowPanel) isCompanion() bool {
	return strings.Contains(strings.ToLower(p.Kind), "companion")
}
