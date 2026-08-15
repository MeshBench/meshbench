// One node, in its own window.
//
// The thing people put on a second monitor: what this node is running, what
// it has printed, and a box to type into. Six tabs to Alex's mock - Console,
// Companion, Settings, Stats, Activity, Connect - with Companion and Connect
// only for nodes that speak the companion protocol.
package workbench

import (
	"fmt"
	"image"
	"strconv"
	"strings"
	"sync"

	"gioui.org/app"
	"gioui.org/io/key"
	"gioui.org/io/system"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
	"gioui.org/widget"

	"github.com/MeshBench/meshbench/internal/gui/comp"
	"github.com/MeshBench/meshbench/internal/gui/state"
	"github.com/MeshBench/meshbench/internal/gui/theme"
)

// nodeTab is which pane of the window is showing, in the mock's order.
type nodeTab int

const (
	tabConsole nodeTab = iota
	// tabCompanion and tabConnect only exist for a node that speaks the
	// companion protocol. A repeater has no channels and no contacts, and a
	// tab that is always there and always empty teaches people to ignore
	// tabs.
	tabCompanion
	tabSettings
	tabRadio
	tabStats
	tabActivity
	tabConnect
	numNodeTabs
)

func (n nodeTab) String() string {
	switch n {
	case tabCompanion:
		return "Companion"
	case tabSettings:
		return "Settings"
	case tabRadio:
		return "Radio"
	case tabStats:
		return "Stats"
	case tabActivity:
		return "Activity"
	case tabConnect:
		return "Connect"
	}
	return "Console"
}

// tabTitle is the console tab's name, which says which console it is.
func (p *nodeWindowPanel) tabTitle(n nodeTab) string {
	if n == tabConsole && p.isCompanion() {
		return "meshcore-cli"
	}
	return n.String()
}

// nodeWindowPanel is the body. Kept separate from the window so it can be
// drawn - and tested - without one.
type nodeWindowPanel struct {
	node string
	tab  nodeTab
	tabs [numNodeTabs]widget.Clickable
	// radioScroll is the Radio tab's own list state.
	radioScroll widget.List
	comp        companionTab

	input    comp.Field
	send     comp.Button
	start    comp.Button
	stop     comp.Button
	energy   comp.Button
	tcpBtn   comp.Button
	serBtn   comp.Button
	dropBtn  comp.Button
	list     layout.List
	statList widget.List
	setList  widget.List
	acts     comp.Table
	built    bool
	rowsSet  bool
	seq      uint64

	// OnCommand is given a line to send to the node's firmware.
	OnCommand func(node, line string)
	// OnCLI is the same for a companion, whose lines are meshcore-cli rather
	// than the firmware's own console.
	OnCLI func(node, line string)
	// OnAction is given a verb and this node's name.
	OnAction func(action, node string)
	// OnServe serves this companion to a real client, over tcp or serial.
	OnServe func(node, kind string)
	// OnOpenPacket opens the packet view for an activity row.
	OnOpenPacket func(id uint64)
	// Kind is what this node is, which decides which tabs it grows.
	Kind string
}

// visibleTabs is the tab set this node gets.
func (p *nodeWindowPanel) visibleTabs() []nodeTab {
	if p.isCompanion() {
		return []nodeTab{tabConsole, tabCompanion, tabSettings, tabRadio,
			tabStats, tabActivity, tabConnect}
	}
	return []nodeTab{tabConsole, tabSettings, tabRadio, tabStats, tabActivity}
}

// clicks handles every control, whichever tab is showing - shared with the
// audit's flat draw, so a control cannot be wired in one and dead in the
// other.
func (p *nodeWindowPanel) clicks(gtx layout.Context) {
	for i := range p.tabs {
		if p.tabs[i].Clicked(gtx) {
			p.tab = nodeTab(i)
		}
	}
	if p.start.Click.Clicked(gtx) && p.OnAction != nil {
		p.OnAction("node.start", p.node)
	}
	if p.stop.Click.Clicked(gtx) && p.OnAction != nil {
		p.OnAction("node.stop", p.node)
	}
	// Enter sends, because a console with a send button and no Enter is a
	// console nobody will use twice.
	submitted := false
	for {
		ev, ok := p.input.Editor.Update(gtx)
		if !ok {
			break
		}
		if _, ok := ev.(widget.SubmitEvent); ok {
			submitted = true
		}
	}
	if p.send.Click.Clicked(gtx) || submitted {
		if line := strings.TrimSpace(p.input.Editor.Text()); line != "" {
			// A companion has no text console: it speaks the framed protocol
			// a phone speaks, so it gets meshcore-cli's vocabulary.
			if p.isCompanion() && p.OnCLI != nil {
				p.OnCLI(p.node, line)
			} else if p.OnCommand != nil {
				p.OnCommand(p.node, line)
			}
			p.input.Editor.SetText("")
		}
	}
}

func (p *nodeWindowPanel) Draw(t *theme.Theme, gtx layout.Context, s *state.Snapshot) layout.Dimensions {
	p.clicks(gtx)
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(p.head(t, s)),
		layout.Rigid(layout.Spacer{Height: t.Sp.S}.Layout),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			switch p.tab {
			case tabStats:
				return p.stats(t, gtx, s)
			case tabSettings:
				return p.settings(t, gtx, s)
			case tabRadio:
				return p.radio(t, gtx, s)
			case tabActivity:
				return p.activity(t, gtx, s)
			case tabConnect:
				return p.connect(t, gtx, s)
			case tabCompanion:
				p.comp.node, p.comp.OnCLI = p.node, p.OnCLI
				return p.comp.Draw(t, gtx, s)
			}
			return p.console(t, gtx, s)
		}),
	)
}

// head is the node's name, what it is and has done, and the tab strip.
func (p *nodeWindowPanel) head(t *theme.Theme, s *state.Snapshot) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		st := p.statFor(s)
		running := st != nil && st.Running
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(comp.Text(t, t.Sz.Title, t.P.Ink, p.node)),
					layout.Rigid(layout.Spacer{Width: t.Sp.M}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						col, what := t.P.Faint, "stopped"
						if running {
							col, what = t.P.Good, "running"
						}
						return comp.Text(t, t.Sz.Caption, col, what)(gtx)
					}),
					layout.Flexed(1, comp.Spacer),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						if running {
							p.stop.Label, p.stop.Kind = "stop it", comp.Destructive
							return p.stop.Layout(t, gtx)
						}
						p.start.Label, p.start.Kind = "start it", comp.Primary
						return p.start.Layout(t, gtx)
					}),
				)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				// The mock's subtitle: what it is and what it has done.
				line := shortKind(p.Kind)
				if st != nil {
					line += fmt.Sprintf("  |  sent %d   heard %d", st.Sent, st.Heard)
				}
				return comp.Text(t, t.Sz.Caption, t.P.Dim, line)(gtx)
			}),
			layout.Rigid(layout.Spacer{Height: t.Sp.XS}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				var kids []layout.FlexChild
				for _, tb := range p.visibleTabs() {
					tb := tb
					kids = append(kids, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return p.tabs[tb].Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							ink, line := t.P.Dim, t.P.Rule
							if p.tab == tb {
								ink, line = t.P.Ink, theme.Alpha(t.P.Accent, 0.8)
							} else if p.tabs[tb].Hovered() {
								ink = t.P.Ink
							}
							return layout.Inset{Right: t.Sp.S}.Layout(gtx,
								func(gtx layout.Context) layout.Dimensions {
									macro := op.Record(gtx.Ops)
									dims := layout.Inset{Top: t.Sp.XS, Bottom: t.Sp.XS,
										Left: t.Sp.S, Right: t.Sp.S}.Layout(gtx,
										comp.Text(t, t.Sz.Body, ink, p.tabTitle(tb)))
									call := macro.Stop()
									comp.RoundRect(gtx, dims.Size, 5, theme.Alpha(t.P.Sunk, 0.6))
									comp.Border(gtx, dims.Size, 5, 1, line)
									call.Add(gtx.Ops)
									return dims
								})
						})
					}))
				}
				return layout.Flex{}.Layout(gtx, kids...)
			}),
			layout.Rigid(layout.Spacer{Height: t.Sp.S}.Layout),
			layout.Rigid(comp.HRule(t)),
		)
	}
}

// console is the scrollback and the box to type into.
func (p *nodeWindowPanel) console(t *theme.Theme, gtx layout.Context, s *state.Snapshot) layout.Dimensions {
	lines := []string(nil)
	if s != nil && s.ConsoleNode == p.node {
		lines = s.Console
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

// auditDraw is every tab's controls at once, flat, for the audit - a tab
// hides its controls from a pointer, and the audit's whole point is pressing
// them all.
func (p *nodeWindowPanel) auditDraw(t *theme.Theme, gtx layout.Context, s *state.Snapshot) layout.Dimensions {
	p.clicks(gtx)
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Bottom: t.Sp.XS}.Layout(gtx, p.head(t, s))
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			// Bounded: settings fills whatever it is offered, and offered
			// everything it would leave nothing for the tabs below it.
			gtx.Constraints.Max.Y = gtx.Dp(300)
			return layout.Inset{Bottom: t.Sp.XS}.Layout(gtx,
				func(gtx layout.Context) layout.Dimensions { return p.settings(t, gtx, s) })
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Bottom: t.Sp.XS}.Layout(gtx,
				func(gtx layout.Context) layout.Dimensions { return p.connect(t, gtx, s) })
		}),
		// The console last, with whatever height remains: its own input row
		// sits at its bottom edge and stays on screen.
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return p.console(t, gtx, s)
		}),
	)
}

// isCompanion decides which command line and which tabs this node gets.
func (p *nodeWindowPanel) isCompanion() bool {
	return strings.Contains(strings.ToLower(p.Kind), "companion")
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

// nodeWindows tracks which nodes have a window, so a second request raises
// rather than opening a duplicate.
type nodeWindows struct {
	mu   sync.Mutex
	open map[string]bool
}

func newNodeWindows() *nodeWindows { return &nodeWindows{open: map[string]bool{}} }

func (w *nodeWindows) openFor(node string, newTheme func() *theme.Theme,
	st *state.Store, onCommand func(node, line string), onAction func(action, node string),
	onCLI func(node, line string), onServe func(node, kind string),
	onOpenPacket func(id uint64)) {
	w.mu.Lock()
	if w.open[node] {
		w.mu.Unlock()
		return
	}
	w.open[node] = true
	w.mu.Unlock()

	go func() {
		defer func() {
			w.mu.Lock()
			delete(w.open, node)
			w.mu.Unlock()
		}()
		th := newTheme()
		p := &nodeWindowPanel{node: node, OnCommand: onCommand, OnAction: onAction,
			OnCLI: onCLI, OnServe: onServe, OnOpenPacket: onOpenPacket,
			Kind: kindOfNode(st, node)}
		p.tab = openOnTab
		win := new(app.Window)
		win.Option(app.Title("MeshBench - "+node), app.Size(unit.Dp(820), unit.Dp(620)))
		// Raised as it opens; see windows.go for why that is all there is.
		win.Perform(system.ActionRaise)
		var ops op.Ops
		for {
			switch e := win.Event().(type) {
			case app.DestroyEvent:
				return
			case app.FrameEvent:
				gtx := app.NewContext(&ops, e)
				comp.Fill(gtx, th.P.Ground)
				layout.UniformInset(th.Sp.M).Layout(gtx,
					func(gtx layout.Context) layout.Dimensions {
						return p.Draw(th, gtx, st.Snapshot())
					})
				e.Frame(gtx.Ops)
				win.Invalidate()
			}
		}
	}()
}

var _ = key.NameEscape
var _ = image.Pt

// kindOfNode reads what a node is from the current snapshot.
func kindOfNode(st *state.Store, node string) string {
	s := st.Snapshot()
	if s == nil {
		return ""
	}
	for i := range s.Nodes {
		if s.Nodes[i].Name == node {
			return s.Nodes[i].Kind
		}
	}
	return ""
}

// companionSubTab is which pane of the companion tab is showing.
type companionSubTab int

const (
	subMessages companionSubTab = iota
	subContacts
	subRadio
)

// companionTab is the mini companion: the workbench driving a node's
// companion protocol in-process, the way a phone would.
//
// Redesigned to Alex's mock: a status strip saying who holds the port,
// Messages / Contacts / Radio sub-tabs, and the replies underneath. Every
// action is still a meshcore-cli line through OnCLI, so the buttons and the
// command line cannot mean different things.
type companionTab struct {
	freq, bw, sf, cr comp.Field
	txdbm, name      comp.Field
	channel, msg     comp.Field

	applyRadio comp.Button
	applyName  comp.Button
	sendMsg    comp.Button
	getChans   comp.Button
	syncMsgs   comp.Button
	contacts   comp.Button
	takeOver   comp.Button
	release    comp.Button

	sub     companionSubTab
	subTabs [3]widget.Clickable
	claimed bool
	list    widget.List
	built   bool

	// OnCLI carries every one of these as a meshcore-cli line.
	OnCLI func(node, line string)
	node  string
}

func (c *companionTab) build() {
	c.freq.Hint, c.bw.Hint = "freq kHz", "bandwidth Hz"
	c.sf.Hint, c.cr.Hint = "spreading factor", "coding rate"
	c.txdbm.Hint, c.name.Hint = "tx dBm", "advertised name"
	c.channel.Hint, c.msg.Hint = "channel", "type a message..."
	for _, f := range []*comp.Field{&c.freq, &c.bw, &c.sf, &c.cr,
		&c.txdbm, &c.name, &c.channel, &c.msg} {
		f.Editor.SingleLine = true
	}
	c.applyRadio.Label, c.applyRadio.Kind = "apply to the node", comp.Primary
	c.applyName.Label, c.applyName.Kind = "set name and power", comp.Secondary
	c.sendMsg.Label, c.sendMsg.Kind = "send", comp.Primary
	c.getChans.Label, c.getChans.Kind = "read channel", comp.Secondary
	c.syncMsgs.Label, c.syncMsgs.Kind = "sync messages", comp.Secondary
	c.contacts.Label, c.contacts.Kind = "sync contacts", comp.Secondary
	c.takeOver.Label, c.takeOver.Kind = "connect", comp.Primary
	c.release.Label, c.release.Kind = "disconnect", comp.Quiet
	c.list.Axis = layout.Vertical
	c.built = true
}

// clicks handles every control, whichever sub-tab is showing.
func (c *companionTab) clicks(gtx layout.Context) {
	send := func(line string) {
		if c.OnCLI != nil {
			c.OnCLI(c.node, line)
		}
	}
	for i := range c.subTabs {
		if c.subTabs[i].Clicked(gtx) {
			c.sub = companionSubTab(i)
		}
	}
	if c.applyRadio.Click.Clicked(gtx) {
		send(fmt.Sprintf("set radio %s %s %s %s",
			fieldText(&c.freq), fieldText(&c.bw), fieldText(&c.sf), fieldText(&c.cr)))
	}
	if c.applyName.Click.Clicked(gtx) {
		if n := fieldText(&c.name); n != "" {
			send("set name " + n)
		}
		if p := fieldText(&c.txdbm); p != "" {
			send("set tx " + p)
		}
	}
	// Sending a message from a companion, which is the thing a companion is
	// for. With no channel it goes to the public one, as meshcore-cli does.
	if c.sendMsg.Click.Clicked(gtx) {
		if m := fieldText(&c.msg); m != "" {
			if ch := fieldText(&c.channel); ch != "" && ch != "0" {
				send("chan " + ch + " " + m)
			} else {
				send("public " + m)
			}
			c.msg.Editor.SetText("")
		}
	}
	if c.getChans.Click.Clicked(gtx) {
		send("get_channel " + orZero(fieldText(&c.channel)))
	}
	if c.syncMsgs.Click.Clicked(gtx) {
		send("sync_msgs")
	}
	if c.contacts.Click.Clicked(gtx) {
		send("contacts")
	}
	if c.takeOver.Click.Clicked(gtx) {
		c.claimed = true
		send("infos")
	}
	if c.release.Click.Clicked(gtx) && c.OnCLI != nil {
		// Releasing hands the port back to the console, which is what
		// somebody wants after looking rather than after changing anything.
		c.claimed = false
		c.OnCLI(c.node, "__disconnect")
	}
}

func (c *companionTab) Draw(t *theme.Theme, gtx layout.Context, s *state.Snapshot) layout.Dimensions {
	if !c.built {
		c.build()
	}
	c.clicks(gtx)

	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return c.statusStrip(t, gtx)
		}),
		layout.Rigid(comp.HRule(t)),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			var kids []layout.FlexChild
			for i, label := range []string{"Messages", "Contacts", "Radio"} {
				i := i
				label := label
				kids = append(kids, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return c.subTabs[i].Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						ink := t.P.Dim
						if c.sub == companionSubTab(i) || c.subTabs[i].Hovered() {
							ink = t.P.Ink
						}
						return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return layout.Inset{Right: t.Sp.L, Top: t.Sp.XS,
									Bottom: t.Sp.XS}.Layout(gtx,
									comp.Text(t, t.Sz.Body, ink, label))
							}),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								if c.sub != companionSubTab(i) {
									return layout.Dimensions{Size: imagePtXY(0, gtx.Dp(2))}
								}
								return comp.FillRect(gtx,
									imagePtXY(gtx.Constraints.Min.X, gtx.Dp(2)), t.P.Accent)
							}),
						)
					})
				}))
			}
			return layout.Flex{}.Layout(gtx, kids...)
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: t.Sp.S}.Layout(gtx,
				func(gtx layout.Context) layout.Dimensions {
					switch c.sub {
					case subContacts:
						return c.contactsPane(t, gtx, s)
					case subRadio:
						return c.radioPane(t, gtx)
					}
					return c.messagesPane(t, gtx, s)
				})
		}),
	)
}

// statusStrip says who holds the node's port right now.
func (c *companionTab) statusStrip(t *theme.Theme, gtx layout.Context) layout.Dimensions {
	word, col := "console mode", t.P.Dim
	if c.claimed {
		word, col = "connected - this window drives the node", t.P.Good
	}
	return layout.Inset{Bottom: t.Sp.S}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(comp.Dot(col, gtx.Dp(3))),
			layout.Rigid(comp.Text(t, t.Sz.Caption, col, "  "+word)),
			layout.Flexed(1, comp.Spacer),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if c.claimed {
					return c.release.Layout(t, gtx)
				}
				return c.takeOver.Layout(t, gtx)
			}),
		)
	})
}

// replies is the conversation with the firmware, shared by every sub-tab.
func (c *companionTab) replies(t *theme.Theme, gtx layout.Context, s *state.Snapshot, empty string) layout.Dimensions {
	lines := []string(nil)
	if s != nil && s.ConsoleNode == c.node {
		lines = s.Console
	}
	if len(lines) == 0 {
		return layout.Center.Layout(gtx, comp.Text(t, t.Sz.Caption, t.P.Faint, empty))
	}
	c.list.ScrollToEnd = true
	return comp.List(t, &c.list, len(lines), func(gtx layout.Context, i int) layout.Dimensions {
		return comp.Mono(t, t.Sz.Data, t.P.Ink, lines[i])(gtx)
	})(gtx)
}

func (c *companionTab) messagesPane(t *theme.Theme, gtx layout.Context, s *state.Snapshot) layout.Dimensions {
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(comp.Text(t, t.Sz.Caption, t.P.Dim, "channel  ")),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					gtx.Constraints.Max.X = gtx.Dp(90)
					return c.channel.Layout(t, gtx)
				}),
				layout.Rigid(layout.Spacer{Width: t.Sp.S}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return c.syncMsgs.Layout(t, gtx)
				}),
				layout.Rigid(layout.Spacer{Width: t.Sp.S}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return c.getChans.Layout(t, gtx)
				}),
			)
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return c.replies(t, gtx, s,
				"nothing on this channel yet - everything here comes out of the "+
					"firmware; there are no invented replies")
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					return c.msg.Layout(t, gtx)
				}),
				layout.Rigid(layout.Spacer{Width: t.Sp.S}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return c.sendMsg.Layout(t, gtx)
				}),
			)
		}),
		layout.Rigid(comp.Text(t, t.Sz.Caption, t.P.Faint,
			"a message with no channel goes to the public one, as it does from meshcore-cli")),
	)
}

func (c *companionTab) contactsPane(t *theme.Theme, gtx layout.Context, s *state.Snapshot) layout.Dimensions {
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return c.contacts.Layout(t, gtx)
				}),
			)
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return c.replies(t, gtx, s,
				"no contacts synced yet - a companion learns its contacts from adverts")
		}),
	)
}

func (c *companionTab) radioPane(t *theme.Theme, gtx layout.Context) layout.Dimensions {
	radio := actionBar{
		fields:  []*comp.Field{&c.freq, &c.bw, &c.sf, &c.cr},
		buttons: []*comp.Button{&c.applyRadio},
		note: "a preset that no longer matches its neighbours is a node that " +
			"hears nothing and reports no fault",
	}
	ident := actionBar{
		fields:  []*comp.Field{&c.name, &c.txdbm},
		buttons: []*comp.Button{&c.applyName},
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return radio.layout(t, gtx) }),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return ident.layout(t, gtx) }),
	)
}

// auditDraw is every control at once, flat, for the audit - the sub-tabs hide
// controls from a pointer, and the audit's whole point is pressing them all.
func (c *companionTab) auditDraw(t *theme.Theme, gtx layout.Context, s *state.Snapshot) layout.Dimensions {
	if !c.built {
		c.build()
	}
	c.clicks(gtx)
	rows := []layout.Widget{
		func(gtx layout.Context) layout.Dimensions { return c.statusStrip(t, gtx) },
		func(gtx layout.Context) layout.Dimensions { return c.radioPane(t, gtx) },
		func(gtx layout.Context) layout.Dimensions {
			bar := actionBar{fields: []*comp.Field{&c.channel, &c.msg},
				buttons: []*comp.Button{&c.sendMsg, &c.getChans, &c.syncMsgs, &c.contacts}}
			return bar.layout(t, gtx)
		},
		func(gtx layout.Context) layout.Dimensions {
			bar := actionBar{buttons: []*comp.Button{&c.takeOver, &c.release}}
			return bar.layout(t, gtx)
		},
	}
	var kids []layout.FlexChild
	for _, r := range rows {
		r := r
		kids = append(kids, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Bottom: t.Sp.XS}.Layout(gtx, r)
		}))
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx, kids...)
}

// atof reads a numeric row key the way the verbs expect numbers.
func atof(s string) float64 {
	v, _ := strconv.ParseFloat(s, 64)
	return v
}

func orZero(s string) string {
	if s == "" {
		return "0"
	}
	return s
}

// openOnTab is which tab a node window opens on. Console, except when a
// capture is being taken of one of the others - a tab cannot be reached from
// outside the application otherwise, and a screenshot of it is how the tab
// gets checked.
var openOnTab nodeTab
