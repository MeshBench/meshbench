// The companion tab of a node window: three ways into one radio.
//
// Client is the workbench's own client, drawing decoded state, and its
// Settings view is what a phone app's radio screen offers - including the
// path hash size nothing else exposes. It is not a mode of its own: CLI and
// TCP hand the radio to whatever is on the other end of them, so a settings
// form only Client can use belongs inside Client. CLI is the meshcore-cli
// command line over the same session; it shares one claim with Client, so
// moving between them is free and neither can disagree with the other about
// what the node holds. TCP is the third: the port handed out over a socket or
// a pty, so a real client can have it, which necessarily means the workbench
// lets go of it.
//
// The claim is exclusive at the bridge, and that is the rule the whole tab is
// arranged around: one holder at a time, and it always says who.
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

// companionMode is which of the three ways in is showing.
type companionMode int

const (
	modeClient companionMode = iota
	modeCLI
	modeTCP
)

// companionModes is the switch, in the order it is offered.
var companionModes = []struct {
	Mode  companionMode
	Label string
}{
	{modeClient, "Client"},
	{modeCLI, "CLI"},
	{modeTCP, "TCP"},
}

// companionTab is the mini companion.
type companionTab struct {
	// The client's own controls.
	msg, scope comp.Field
	// newChan adds a channel slot, which the firmware addresses by index and
	// has no command to enumerate - so a name has to be given one.
	newChan    comp.Field
	addChan    comp.Button
	sendMsg    comp.Button
	applyScope comp.Button
	advertBtn  comp.Button
	refreshBtn comp.Button
	railList   widget.List
	convoList  widget.List
	// channelIdx is the channel being read. Unread counts clear against it,
	// so it is sent to the session rather than kept only here.
	channelIdx uint8
	sentRead   uint8
	readOnce   bool
	// clientSettings shows the radio settings in place of the conversation.
	// A toggle inside Client rather than a mode of its own: CLI and TCP hand
	// the radio to whatever is on the other end of them, so nothing outside
	// Client can want this pane.
	clientSettings bool

	// The settings form's own: a box per field, and the size selector that
	// the composer shares.
	setName, setFreq, setBW comp.Field
	setSF, setCR, setTx     comp.Field
	applyRadio              comp.Button
	radioList               widget.List
	radioSubmitted          bool
	// pathHash is the size the operator has chosen, in bytes. Defaults to
	// three - the largest the firmware accepts, and the one setting that
	// makes minimal, moderate and strict loop detection all mean the same
	// thing - so an untouched control is a decided preference, not an
	// ambiguous one the node's own report has to fill in.
	pathHash int

	// The command line's own.
	cmd     comp.Field
	runCmd  comp.Button
	cliList widget.List
	// Connect and release, shared by Client and CLI because they share the
	// claim.
	connectBtn comp.Button
	release    comp.Button

	// Serving, for the third mode.
	serveBtn     comp.Button
	stopServeBtn comp.Button
	dropBtn      comp.Button
	tcpChip      comp.Chip
	ptyChip      comp.Chip
	serveKind    string

	mode      companionMode
	modeChips [3]widget.Clickable
	clicks_   map[string]*widget.Clickable
	built     bool

	// OnCLI carries a command line to the session; OnDo runs a verb with
	// parameters, which is how everything else here acts.
	OnCLI func(node, line string)
	OnDo  func(verb string, params any)
	node  string
}

func (c *companionTab) build() {
	c.msg.Hint = "type a message, then Enter"
	c.scope.Hint = "#sco"
	c.cmd.Hint = "a meshcore-cli command, then Enter - ? for the list"
	// Enter sends. A box you have to reach for the mouse to submit is a box
	// that gets typed into and then abandoned.
	c.msg.Editor.SingleLine, c.msg.Editor.Submit = true, true
	c.cmd.Editor.SingleLine, c.cmd.Editor.Submit = true, true
	c.scope.Editor.SingleLine, c.scope.Editor.Submit = true, true
	c.newChan.Hint = "#name"
	c.newChan.Editor.SingleLine, c.newChan.Editor.Submit = true, true
	c.sendMsg.Label, c.sendMsg.Kind = "Send", comp.Primary
	c.runCmd.Label, c.runCmd.Kind = "Run", comp.Primary
	c.applyScope.Label, c.applyScope.Kind = "set", comp.Secondary
	c.advertBtn.Label, c.advertBtn.Kind = "advert", comp.Secondary
	c.addChan.Label, c.addChan.Kind = "add", comp.Secondary
	c.refreshBtn.Label, c.refreshBtn.Kind = "refresh", comp.Secondary
	c.connectBtn.Label, c.connectBtn.Kind = "Connect", comp.Primary
	c.release.Label, c.release.Kind = "Disconnect", comp.Secondary
	c.serveBtn.Label, c.serveBtn.Kind = "Serve", comp.Primary
	c.stopServeBtn.Label, c.stopServeBtn.Kind = "Stop serving", comp.Secondary
	c.dropBtn.Label, c.dropBtn.Kind = "Drop client", comp.Secondary
	c.railList.Axis, c.convoList.Axis, c.cliList.Axis = layout.Vertical, layout.Vertical, layout.Vertical
	c.buildRadio()
	c.pathHash = 3
	c.clicks_ = map[string]*widget.Clickable{}
	c.built = true
}

// click is one named clickable, made on first use.
func (c *companionTab) click(key string) *widget.Clickable {
	if c.clicks_ == nil {
		c.clicks_ = map[string]*widget.Clickable{}
	}
	ck, ok := c.clicks_[key]
	if !ok {
		ck = &widget.Clickable{}
		c.clicks_[key] = ck
	}
	return ck
}

// do runs a verb for this node.
func (c *companionTab) do(verb string, params map[string]any) {
	if c.OnDo == nil {
		return
	}
	if params == nil {
		params = map[string]any{}
	}
	params["node"] = c.node
	c.OnDo(verb, params)
}

func (c *companionTab) clicks(gtx layout.Context, cs state.Companion) {
	// Submissions first, so Enter and the button are the same act.
	sendMsg := submitted(gtx, &c.msg)
	runLine := submitted(gtx, &c.cmd)
	setScope := submitted(gtx, &c.scope)
	addChan := submitted(gtx, &c.newChan)
	// Every settings box drains its events whether or not it is on show, and
	// Enter in any of them is Apply - the form is one act, not six.
	c.radioSubmitted = false
	for _, f := range c.radioFields() {
		if submitted(gtx, f) {
			c.radioSubmitted = true
		}
	}
	c.radioClicks(gtx, cs)
	for i := range c.modeChips {
		if c.modeChips[i].Clicked(gtx) {
			c.setMode(companionModes[i].Mode)
		}
	}
	if c.click("client:messages").Clicked(gtx) {
		c.clientSettings = false
	}
	if c.click("client:settings").Clicked(gtx) {
		c.clientSettings = true
	}
	if c.connectBtn.Click.Clicked(gtx) {
		c.do("companion.connect", nil)
	}
	if c.release.Click.Clicked(gtx) {
		c.do("companion.disconnect", nil)
	}
	if c.sendMsg.Click.Clicked(gtx) || sendMsg {
		if m := fieldText(&c.msg); m != "" {
			params := map[string]any{"text": m, "channel": float64(c.channelIdx)}
			// Only when it differs from what the node already holds: the
			// firmware saves prefs on every set, and rewriting flash before
			// each message to store the value already there is a real cost
			// for no change.
			if c.pathHash != cs.PathHashBytes {
				params["path_hash"] = float64(c.pathHash)
			}
			c.do("companion.send", params)
			c.msg.Editor.SetText("")
		}
	}
	if c.applyScope.Click.Clicked(gtx) || setScope {
		c.do("companion.scope", map[string]any{"scope": fieldText(&c.scope)})
	}
	if c.advertBtn.Click.Clicked(gtx) {
		c.do("companion.advert", nil)
	}
	if c.addChan.Click.Clicked(gtx) || addChan {
		if n := fieldText(&c.newChan); n != "" {
			c.do("companion.add_channel", map[string]any{"name": n})
			c.newChan.Editor.SetText("")
		}
	}
	if c.refreshBtn.Click.Clicked(gtx) {
		c.do("companion.refresh", nil)
	}
	for i := range cs.Channels {
		idx := cs.Channels[i].Index
		if c.click(fmt.Sprintf("chan:%d", idx)).Clicked(gtx) {
			c.channelIdx = idx
		}
	}
	if c.runCmd.Click.Clicked(gtx) || runLine {
		if l := fieldText(&c.cmd); l != "" && c.OnCLI != nil {
			c.OnCLI(c.node, l)
			c.cmd.Editor.SetText("")
		}
	}
	if c.tcpChip.Click.Clicked(gtx) {
		c.serveKind = "tcp"
	}
	if c.ptyChip.Click.Clicked(gtx) {
		c.serveKind = "serial"
	}
	if c.serveBtn.Click.Clicked(gtx) {
		// Serving means letting go: the port takes one holder, and the serve
		// verb releases ours itself. Firing a disconnect first from here sent
		// two verbs on two goroutines, and when serve arrived first the
		// disconnect released the port out from under the new client.
		c.do("bench.serve", map[string]any{"kind": c.serveKind})
	}
	if c.stopServeBtn.Click.Clicked(gtx) {
		c.do("bench.drop", nil)
	}
	if c.dropBtn.Click.Clicked(gtx) {
		c.do("bench.drop", nil)
	}
	if c.click("copyaddr").Clicked(gtx) {
		comp.CopyText(gtx, cs.Serving.Addr)
	}
	// Tell the session which channel is being read, so its unread count
	// clears. Only on a change: this is a verb, not a per-frame poll.
	if cs.Connected && (!c.readOnce || c.sentRead != c.channelIdx) {
		c.readOnce, c.sentRead = true, c.channelIdx
		c.do("companion.read", map[string]any{"channel": float64(c.channelIdx)})
	}
}

// setMode switches which way in is showing.
//
// Client and CLI share the claim, so moving between them changes nothing on
// the wire. TCP is a different holder, which is why it is the only one that
// touches the connection - and it does that when Serve is pressed, not on the
// switch, so looking at the tab never disconnects anything.
func (c *companionTab) setMode(m companionMode) {
	c.mode = m
}

func (c *companionTab) Draw(t *theme.Theme, gtx layout.Context, s *state.Snapshot) layout.Dimensions {
	if !c.built {
		c.build()
	}
	cs := companionState(s, c.node)
	c.clicks(gtx, cs)

	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return c.statusStrip(t, gtx, cs)
		}),
		layout.Rigid(comp.HRule(t)),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			switch c.mode {
			case modeCLI:
				return c.cliPane(t, gtx, s, cs)
			case modeTCP:
				return c.tcpPane(t, gtx, cs)
			}
			return c.clientPane(t, gtx, cs, cs.Connected)
		}),
	)
}

// submitted reports whether Enter was pressed in a field this frame.
//
// The editor's events have to be drained whether or not anybody is listening,
// so this is called for every box every frame rather than only the one in the
// mode on show.
func submitted(gtx layout.Context, f *comp.Field) bool {
	hit := false
	for {
		ev, ok := f.Editor.Update(gtx)
		if !ok {
			return hit
		}
		if _, ok := ev.(widget.SubmitEvent); ok {
			hit = true
		}
	}
}

// companionState is this node's session out of a snapshot.
func companionState(s *state.Snapshot, node string) state.Companion {
	if s == nil {
		return state.Companion{Node: node}
	}
	for _, c := range s.Companions {
		if c.Node == node {
			return c
		}
	}
	return state.Companion{Node: node}
}

// statusStrip says who holds the port, and offers the three ways in.
func (c *companionTab) statusStrip(t *theme.Theme, gtx layout.Context, cs state.Companion) layout.Dimensions {
	return layout.UniformInset(t.Sp.S).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(comp.Text(t, t.Sz.Caption, holderInk(t, cs), holderWord(cs))),
			layout.Rigid(layout.Spacer{Width: t.Sp.S}.Layout),
			layout.Flexed(1, comp.OneLine(t, t.Sz.Caption, t.P.Faint, identityLine(cs), false)),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				var kids []layout.FlexChild
				for i, m := range companionModes {
					i, m := i, m
					kids = append(kids, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Inset{Left: t.Sp.XXS}.Layout(gtx,
							func(gtx layout.Context) layout.Dimensions {
								return c.modeChips[i].Layout(gtx, func(gtx layout.Context) layout.Dimensions {
									return modeChip(t, gtx, m.Label, c.mode == m.Mode)
								})
							})
					}))
				}
				return layout.Flex{Alignment: layout.Middle}.Layout(gtx, kids...)
			}),
			layout.Rigid(layout.Spacer{Width: t.Sp.S}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				// TCP owns its own start and stop, in its pane.
				if c.mode == modeTCP {
					return layout.Dimensions{}
				}
				if cs.Connected {
					return c.release.Layout(t, gtx)
				}
				return c.connectBtn.Layout(t, gtx)
			}),
		)
	})
}

// modeChip is one segment of the Client / CLI / TCP switch.
func modeChip(t *theme.Theme, gtx layout.Context, label string, on bool) layout.Dimensions {
	ink, bg := t.P.Dim, t.P.Sunk
	if on {
		ink, bg = t.P.AccentInk, t.P.Accent
	}
	macro := op.Record(gtx.Ops)
	dims := layout.Inset{Left: t.Sp.S, Right: t.Sp.S, Top: t.Sp.XXS, Bottom: t.Sp.XXS}.
		Layout(gtx, comp.Text(t, t.Sz.Caption, ink, label))
	call := macro.Stop()
	comp.RoundRect(gtx, dims.Size, 5, bg)
	comp.Border(gtx, dims.Size, 5, 1, t.P.Rule)
	call.Add(gtx.Ops)
	return dims
}

// holderWord is who has the port, in one word.
func holderWord(cs state.Companion) string {
	switch {
	case cs.Serving.Addr != "" && cs.Serving.Attached:
		return "served, in use"
	case cs.Serving.Addr != "":
		return "served"
	case cs.Connected:
		return "connected"
	}
	return "not connected"
}

func holderInk(t *theme.Theme, cs state.Companion) colorNRGBA {
	switch {
	case cs.Connected, cs.Serving.Attached:
		return t.P.Good
	case cs.Serving.Addr != "":
		return t.P.Warn
	}
	return t.P.Faint
}

// identityLine is what the node says it is, which is not always what the
// scenario believes - and when they differ the node is the one that is right.
func identityLine(cs state.Companion) string {
	if !cs.Connected {
		if cs.Serving.Addr != "" {
			return cs.Serving.Addr + " - the workbench is not reading this port"
		}
		return ""
	}
	var b strings.Builder
	if cs.Name != "" {
		b.WriteString(cs.Name)
	}
	if cs.Key != "" {
		b.WriteString(" · " + cs.Key)
	}
	if cs.FreqKHz > 0 {
		fmt.Fprintf(&b, " · %.3f MHz SF%d CR4/%d", float64(cs.FreqKHz)/1000, cs.SF, cs.CR)
	}
	if cs.Scoped {
		if cs.Scope == "" {
			b.WriteString(" · unscoped")
		} else {
			b.WriteString(" · " + cs.Scope)
		}
	}
	return b.String()
}

// auditDraw is every control at once, flat, for the audit - the modes hide
// controls from a pointer, and the audit's whole point is pressing them all.
func (c *companionTab) auditDraw(t *theme.Theme, gtx layout.Context, s *state.Snapshot) layout.Dimensions {
	if !c.built {
		c.build()
	}
	cs := companionState(s, c.node)
	c.clicks(gtx, cs)
	// Two bars, not one. An action bar flexes its boxes evenly across the
	// width, so ten of them in a row are each too narrow to take a keystroke -
	// which the audit reports as a box that does not accept typing, and it is
	// right to. The settings form has its own row here because it has its own
	// row on screen.
	radioFields, radioButtons := c.radioWidgets()
	bar := actionBar{
		fields: []*comp.Field{&c.msg, &c.scope, &c.cmd, &c.newChan},
		extras: []func(t *theme.Theme, gtx layout.Context) layout.Dimensions{
			func(t *theme.Theme, gtx layout.Context) layout.Dimensions {
				return c.clientToggle(t, gtx)
			},
		},
		buttons: []*comp.Button{
			&c.connectBtn, &c.release, &c.sendMsg, &c.applyScope, &c.advertBtn,
			&c.refreshBtn, &c.runCmd, &c.serveBtn, &c.stopServeBtn, &c.dropBtn,
			&c.addChan,
		},
	}
	radioBar := actionBar{fields: radioFields, buttons: radioButtons}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return bar.layout(t, gtx) }),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return radioBar.layout(t, gtx) }),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return c.tcpChip.Layout(t, gtx, "TCP socket", "", c.serveKind != "serial", t.P.Accent)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return c.ptyChip.Layout(t, gtx, "PTY device", "", c.serveKind == "serial", t.P.Accent)
				}),
			)
		}),
	)
}
