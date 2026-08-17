// The companion tab of a node window: the messages, contacts and radio panes
// a real client would show, driven over the companion protocol.
package workbench

import (
	"fmt"
	"strconv"

	"gioui.org/layout"
	"gioui.org/widget"
	"github.com/MeshBench/meshbench/internal/gui/comp"
	"github.com/MeshBench/meshbench/internal/gui/state"
	"github.com/MeshBench/meshbench/internal/gui/theme"
)

// companionSubTab is which pane of the companion tab is showing.
type companionSubTab int

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
	var lines []string
	if s != nil {
		lines = s.Consoles[c.node]
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
