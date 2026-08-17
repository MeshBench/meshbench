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
	// OnDo runs a verb with parameters. The companion client needs more than
	// a node name - a channel, a scope, a transport - so it cannot go through
	// OnAction, which only ever carries the one.
	OnDo func(verb string, params any)
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
				p.comp.node, p.comp.OnCLI, p.comp.OnDo = p.node, p.OnCLI, p.OnDo
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

// nodeWindows tracks which nodes have a window, so a second request raises
// rather than opening a duplicate.
type nodeWindows struct {
	mu   sync.Mutex
	open map[string]bool
}

func newNodeWindows() *nodeWindows { return &nodeWindows{open: map[string]bool{}} }

// nodeWindowHooks is how a node window reaches the rest of the application.
//
// A struct rather than a seventh positional callback: six was already a list
// nobody could read at the call site, and the companion client needs one more
// that carries parameters rather than only a node name.
type nodeWindowHooks struct {
	onCommand    func(node, line string)
	onAction     func(action, node string)
	onCLI        func(node, line string)
	onServe      func(node, kind string)
	onOpenPacket func(id uint64)
	onDo         func(verb string, params any)
}

func (w *nodeWindows) openFor(node string, newTheme func() *theme.Theme,
	st *state.Store, h nodeWindowHooks) {
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
		p := &nodeWindowPanel{node: node, OnCommand: h.onCommand, OnAction: h.onAction,
			OnCLI: h.onCLI, OnServe: h.onServe, OnOpenPacket: h.onOpenPacket,
			OnDo: h.onDo, Kind: kindOfNode(st, node)}
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

// openOnTab is which tab a node window opens on. Console, except when a
// capture is being taken of one of the others - a tab cannot be reached from
// outside the application otherwise, and a screenshot of it is how the tab
// gets checked.
var openOnTab nodeTab
