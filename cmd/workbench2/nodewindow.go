// One node, in its own window.
//
// The thing people put on a second monitor: what this node is running, what it
// has printed, and a box to type into. The old workbench had six tabs here and
// the Gio build had none - the map's "open in its own window" dispatched a
// verb that does not exist, so the menu item did nothing at all.
package main

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

	"github.com/A13xB0/meshcoresim/internal/gui/comp"
	"github.com/A13xB0/meshcoresim/internal/gui/state"
	"github.com/A13xB0/meshcoresim/internal/gui/theme"
)

// nodeTab is which half of the window is showing.
type nodeTab int

const (
	tabConsole nodeTab = iota
	tabStats
	tabActivity
	// tabCompanion only exists for a node that speaks the companion
	// protocol. A repeater has no channels and no contacts, and a tab that is
	// always there and always empty teaches people to ignore tabs.
	tabCompanion
)

func (n nodeTab) String() string {
	switch n {
	case tabStats:
		return "Stats"
	case tabActivity:
		return "Activity"
	case tabCompanion:
		return "Radio & contacts"
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
	tabs [4]widget.Clickable
	comp companionTab

	input    comp.Field
	send     comp.Button
	start    comp.Button
	stop     comp.Button
	list     layout.List
	statList widget.List
	acts     comp.Table
	built    bool
	rowsSet  bool
	seq      uint64

	// OnCommand is given a line to send to the node's firmware.
	OnCommand func(node, line string)
	// OnCLI is the same for a companion, whose lines are meshcore-cli rather
	// than the firmware's own console.
	OnCLI func(node, line string)
	// Kind is what this node is, which decides which of the two it gets.
	Kind string
	// OnAction is given a verb and this node's name.
	OnAction func(action, node string)
}

func (p *nodeWindowPanel) Draw(t *theme.Theme, gtx layout.Context, s *state.Snapshot) layout.Dimensions {
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
			// a phone speaks, so what it gets is meshcore-cli's vocabulary -
			// the tool people already use against real hardware - rather than
			// a console that would answer nothing.
			if p.isCompanion() && p.OnCLI != nil {
				p.OnCLI(p.node, line)
			} else if p.OnCommand != nil {
				p.OnCommand(p.node, line)
			}
			p.input.Editor.SetText("")
		}
	}

	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(p.head(t, s)),
		layout.Rigid(layout.Spacer{Height: t.Sp.S}.Layout),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			switch p.tab {
			case tabStats:
				return p.stats(t, gtx, s)
			case tabActivity:
				return p.activity(t, gtx, s)
			case tabCompanion:
				p.comp.node, p.comp.OnCLI = p.node, p.OnCLI
				return p.comp.Draw(t, gtx, s)
			}
			return p.console(t, gtx, s)
		}),
	)
}

// head is the node's name, what it is running, and the tab strip.
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
			layout.Rigid(layout.Spacer{Height: t.Sp.XS}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				tabs := 3
				if p.isCompanion() {
					tabs = 4
				}
				var kids []layout.FlexChild
				for i := 0; i < tabs; i++ {
					i := i
					kids = append(kids, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						fg := t.P.Dim
						if p.tab == nodeTab(i) {
							fg = t.P.Accent
						}
						return p.tabs[i].Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return layout.Inset{Right: t.Sp.M, Top: t.Sp.XS,
								Bottom: t.Sp.XS}.Layout(gtx,
								comp.Text(t, t.Sz.Body, fg, p.tabTitle(nodeTab(i))))
						})
					}))
				}
				return layout.Flex{}.Layout(gtx, kids...)
			}),
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

// stats is what this node costs and what it has carried.
func (p *nodeWindowPanel) stats(t *theme.Theme, gtx layout.Context, s *state.Snapshot) layout.Dimensions {
	st := p.statFor(s)
	if st == nil {
		return layout.Center.Layout(gtx, comp.Text(t, t.Sz.Caption, t.P.Faint,
			"no statistics for this node yet"))
	}
	var node *state.Node
	if s != nil {
		for i := range s.Nodes {
			if s.Nodes[i].Name == p.node {
				node = &s.Nodes[i]
				break
			}
		}
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
	if node != nil {
		rows = append(rows,
			[2]string{"position", fmt.Sprintf("%.5f, %.5f", node.Lat, node.Lon)},
			[2]string{"height", fmt.Sprintf("%.0f m above ground", node.HeightM)},
			[2]string{"transmit power", fmt.Sprintf("%.0f dBm", node.TxDBm)},
			[2]string{"regions", strings.Join(node.Regions, " ")},
		)
	}
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

// activity is every event this node was an end of.
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
	return p.acts.Layout(t, gtx, func(string) {})
}

// isCompanion decides which command line this node gets.
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
	onCLI func(node, line string)) {
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
			OnCLI: onCLI, Kind: kindOfNode(st, node)}
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

// companionTab is the radio, the channels and the contacts, for a node that
// speaks the companion protocol.
//
// The old workbench had these on a tab of the node window and this build had
// none of it. They are the settings a client changes and then wonders why the
// mesh went quiet: a radio preset that no longer matches its neighbours is a
// node that hears nothing and reports no fault.
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

	list  widget.List
	built bool

	// OnCLI carries every one of these as a meshcore-cli line, so the buttons
	// and the command line cannot mean different things.
	OnCLI func(node, line string)
	node  string
}

func (c *companionTab) Draw(t *theme.Theme, gtx layout.Context, s *state.Snapshot) layout.Dimensions {
	if !c.built {
		c.freq.Hint, c.bw.Hint = "freq kHz", "bandwidth Hz"
		c.sf.Hint, c.cr.Hint = "spreading factor", "coding rate"
		c.txdbm.Hint, c.name.Hint = "tx dBm", "advertised name"
		c.channel.Hint, c.msg.Hint = "channel", "message"
		for _, f := range []*comp.Field{&c.freq, &c.bw, &c.sf, &c.cr,
			&c.txdbm, &c.name, &c.channel, &c.msg} {
			f.Editor.SingleLine = true
		}
		c.applyRadio.Label, c.applyRadio.Kind = "apply to the node", comp.Primary
		c.applyName.Label, c.applyName.Kind = "set name and power", comp.Secondary
		c.sendMsg.Label, c.sendMsg.Kind = "send it", comp.Primary
		c.getChans.Label, c.getChans.Kind = "read channel", comp.Secondary
		c.syncMsgs.Label, c.syncMsgs.Kind = "sync messages", comp.Secondary
		c.contacts.Label, c.contacts.Kind = "sync contacts", comp.Secondary
		c.takeOver.Label, c.takeOver.Kind = "take it over", comp.Primary
		c.release.Label, c.release.Kind = "give it back", comp.Quiet
		c.built = true
	}
	send := func(line string) {
		if c.OnCLI != nil {
			c.OnCLI(c.node, line)
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
	// for.
	//
	// The box for it was declared, given a hint, and never laid out: a text
	// box that exists in the source and not on the screen. With no channel it
	// goes to the public one, which is what meshcore-cli does.
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
		send("infos")
	}
	if c.release.Click.Clicked(gtx) && c.OnCLI != nil {
		// Releasing hands the port back to the console, which is what somebody
		// wants after looking rather than after changing anything.
		c.OnCLI(c.node, "__disconnect")
	}

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
	traffic := actionBar{
		fields:  []*comp.Field{&c.channel, &c.msg},
		buttons: []*comp.Button{&c.sendMsg, &c.getChans, &c.syncMsgs, &c.contacts},
		note: "a message with no channel goes to the public one, as it does " +
			"from meshcore-cli",
	}
	claim := actionBar{
		buttons: []*comp.Button{&c.takeOver, &c.release},
		note: "taking it over claims the node's serial port, so its console " +
			"goes quiet until you give it back",
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return radio.layout(t, gtx) }),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return ident.layout(t, gtx) }),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return traffic.layout(t, gtx) }),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return claim.layout(t, gtx) }),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			lines := []string(nil)
			if s != nil && s.ConsoleNode == c.node {
				lines = s.Console
			}
			if len(lines) == 0 {
				return layout.Center.Layout(gtx, comp.Text(t, t.Sz.Caption, t.P.Faint,
					"what the node answers appears here and in the meshcore-cli tab"))
			}
			return comp.List(t, &c.list, len(lines), func(gtx layout.Context, i int) layout.Dimensions {
				return comp.Mono(t, t.Sz.Data, t.P.Ink, lines[i])(gtx)
			})(gtx)
		}),
	)
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
