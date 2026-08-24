// One node, in its own window.
//
// The thing people put on a second monitor: what this node is running, what
// it has printed, and a box to type into. Six tabs to Alex's mock - Console,
// Companion, Settings, Stats, Activity, Connect - with Companion and Connect
// only for nodes that speak the companion protocol.
package workbench

import (
	"fmt"
	"strings"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/widget"

	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/ui/comp"
	"github.com/MeshBench/meshbench/internal/ui/theme"
)

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
	trueRF   comp.Check
	wasTrue  bool
	send     comp.Button
	start    comp.Button
	stop     comp.Button
	energy   comp.Button
	changeFw comp.Button
	// pick is the build list, the same control the Nodes running panel opens
	// from its firmware cell. Shared so the two cannot come to offer
	// different builds or apply them differently.
	pick     buildPicker
	tcpBtn   comp.Button
	serBtn   comp.Button
	dropBtn  comp.Button
	sdrServe comp.Button
	sdrStop  comp.Button
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
	// Layered reports that the window is a Wayland layer-shell surface, which
	// carries no decoration of the compositor's and so draws its own title
	// bar. Set by the window loop from ConfigEvent.
	Layered bool
	// bar is that title bar, and maximised is its restore state, both owned
	// here so the widget's address never changes across frames. The window
	// loop polls them; the panel only draws.
	bar       comp.TitleBar
	maximised bool
	// Kind is what this node is, which decides which tabs it grows.
	Kind string
	// out is the Output tab's own state: which source is showing, and the
	// widgets that say so.
	out outputPane
	// hasHardware is set each frame from the node's board, so the Hardware
	// tab appears exactly when the board declares something to show. Not a
	// preference: a setting and the hardware can disagree, and a node showing
	// a display its board has not got is worse than one showing none.
	hasHardware bool
	// boardButtons are the drawn buttons, pooled by pin, and buttonDown is
	// what each was last reported as - so a hold is sent once and a release
	// once, rather than every frame the pointer is down.
	boardButtons map[int]*widget.Clickable
	buttonDown   map[int]bool
	// screenTouch is what pointer events on the drawn panel are addressed to,
	// and screenScale is what they have to be divided by to become the
	// panel's own coordinates.
	screenTouch struct{}
	screenScale int
	// screenKeys is what typing at the board is addressed to. Focus is taken
	// by clicking the drawn panel, which is how somebody would pick a
	// handheld up before typing on it.
	screenKeys struct{}
}

// visibleTabs is the tab set this node gets.
func (p *nodeWindowPanel) visibleTabs() []nodeTab {
	var tabs []nodeTab
	switch {
	case p.isCompanion():
		tabs = []nodeTab{tabCompanion, tabSettings, tabRadio,
			tabStats, tabActivity, tabConnect}
	case p.isObserver():
		// No console and no Radio tab: an observer runs no firmware and has
		// no chip to read back, and a tab that is always empty teaches
		// people to ignore tabs.
		//
		// No Hardware tab either, and that one is not about tidiness: an
		// observer is not a board. It has no screen to draw and no button to
		// press, so there is nothing for the tab to be.
		return []nodeTab{tabSDR, tabSettings, tabStats, tabActivity}
	default:
		tabs = []nodeTab{tabConsole, tabSettings, tabRadio, tabStats, tabActivity}
	}
	// Whatever the node's role. The hardware belongs to the board, not to the
	// application running on it - a companion on a T-Deck has the same screen
	// and the same keyboard as a repeater on one, and the tab was reachable
	// on the repeater and not on the handheld it was built for.
	if p.hasHardware {
		tabs = append(tabs, tabHardware)
	}
	// Every node that runs firmware, whatever it runs and wherever it runs.
	// What a board printed while failing to start is the one thing that says
	// why, and it was reachable only by finding the file in a terminal.
	tabs = append(tabs, tabOutput)
	return tabs
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
	p.outputClicks(gtx)
	if p.start.Click.Clicked(gtx) && p.OnAction != nil {
		p.OnAction("node.start", p.node)
	}
	if p.stop.Click.Clicked(gtx) && p.OnAction != nil {
		p.OnAction("node.stop", p.node)
	}
	if p.sdrServe.Click.Clicked(gtx) && p.OnAction != nil {
		p.OnAction("sdr.serve", p.node)
	}
	if p.sdrStop.Click.Clicked(gtx) && p.OnAction != nil {
		p.OnAction("sdr.stop", p.node)
	}
	// Which build this node runs, from the window that is about this node.
	//
	// The capability was only ever reachable from the Nodes running table or
	// over the control socket, so pinning one local build to one node meant
	// leaving the window that names it. Same control, same verb.
	if p.changeFw.Click.Clicked(gtx) {
		p.pick.open(p.node)
	}
	p.pick.OnPick = func(node string, b buildChoice) {
		if p.OnDo == nil {
			return
		}
		// node.set_firmware, not node.set_firmware_only: firmware is chosen
		// when a node launches, so the verb stops it, provisions it again and
		// starts it. Recording the choice and leaving the node on its old
		// build is the control somebody presses twice and then distrusts.
		//
		// Board and role travel with the version. A board image is not a
		// version on its own - "wadamesh" means nothing until it is wadamesh
		// for a LilyGo_TDeck built as a companion - so a pin carrying only the
		// version is one the runner cannot honour.
		p.OnDo("node.set_firmware", map[string]any{
			"node": node, "version": b.Version, "board": b.Board, "role": b.Role})
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
		if line := strings.TrimSpace(p.input.Editor.Text()); line != "" && p.OnCommand != nil {
			p.OnCommand(p.node, line)
			p.input.Editor.SetText("")
		}
	}
}

func (p *nodeWindowPanel) Draw(t *theme.Theme, gtx layout.Context, s *state.Snapshot) layout.Dimensions {
	// A companion has no console tab, and tabConsole is the zero value every
	// window opens on - left alone it would draw a pane its own strip does
	// not offer.
	if p.isCompanion() && p.tab == tabConsole {
		p.tab = tabCompanion
	}
	// An observer's window opens on its SDR pane, and never lands on a tab
	// its strip does not offer.
	if p.isObserver() {
		switch p.tab {
		case tabConsole, tabCompanion, tabRadio, tabConnect:
			p.tab = tabSDR
		}
	}
	p.hasHardware = p.boardPanel(s).HasAnything()
	if p.tab == tabHardware {
		p.boardPresses(gtx, s)
		p.boardTouches(gtx, s)
		p.boardKeys(gtx, s)
	}
	if !p.hasHardware && p.tab == tabHardware {
		p.tab = tabConsole
	}
	p.clicks(gtx)
	// The build list goes over the whole window, not inside the Settings
	// tab's flex: laid out as a child of a pane that has already taken the
	// space, it drew at zero height and could not be clicked. That is the
	// same trap the Nodes running panel fell into, which is why both use the
	// one control.
	//
	// Over the title bar too, when there is one: the bar is a child of the
	// same flex, and an overlay that stopped short of it would leave a strip
	// of live buttons above a modal list.
	if p.pick.showing() {
		defer func() {
			macro := op.Record(gtx.Ops)
			p.pick.overlay(t, gtx)
			macro.Stop().Add(gtx.Ops)
		}()
	}
	// The window's own chrome when nothing else gave it any: a layer-shell
	// window has no title bar but the one drawn here.
	var kids []layout.FlexChild
	if p.Layered {
		p.bar.Title, p.bar.Maximised = p.node, p.maximised
		kids = append(kids, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return p.bar.Layout(t, gtx)
		}))
	}
	kids = append(kids,
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
			case tabSDR:
				return p.sdrTab(t, gtx, s)
			case tabHardware:
				return p.hardware(t, gtx, s)
			case tabOutput:
				return p.output(t, gtx, s)
			}
			return p.console(t, gtx, s)
		}),
	)
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx, kids...)
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
						if p.isObserver() {
							return p.observerStatus(t, s)(gtx)
						}
						col, what := t.P.Faint, "stopped"
						if running {
							col, what = t.P.Good, "running"
						}
						return comp.Text(t, t.Sz.Caption, col, what)(gtx)
					}),
					layout.Flexed(1, comp.Spacer),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						if p.isObserver() {
							// Start and stop belong to firmware, which an
							// observer does not run; its head action is the
							// antenna going on and off the wire.
							return p.observerServeButton(t, gtx, s)
						}
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
										comp.Text(t, t.Sz.Body, ink, tb.String()))
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
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Bottom: t.Sp.XS}.Layout(gtx,
				func(gtx layout.Context) layout.Dimensions { return p.sdrTab(t, gtx, s) })
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			// The True RF switch alone, not the whole Radio tab: the tab
			// bails out early for a node whose chip has not reported, and
			// the audit needs the control on screen regardless.
			gtx.Constraints.Max.Y = gtx.Dp(40)
			return layout.Inset{Bottom: t.Sp.XS}.Layout(gtx,
				func(gtx layout.Context) layout.Dimensions {
					p.wireTrueRF(gtx, s)
					return p.trueRF.LayoutSwitch(t, gtx)
				})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			// The output pane's own chrome - the source buttons and the
			// follow toggle. Bounded, because it fills what it is given.
			gtx.Constraints.Max.Y = gtx.Dp(120)
			return layout.Inset{Bottom: t.Sp.XS}.Layout(gtx,
				func(gtx layout.Context) layout.Dimensions { return p.output(t, gtx, s) })
		}),
		// The console last, with whatever height remains: its own input row
		// sits at its bottom edge and stays on screen.
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return p.console(t, gtx, s)
		}),
	)
}
