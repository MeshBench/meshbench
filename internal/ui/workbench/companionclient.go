// The companion client: channels and contacts on the left, the conversation
// in the middle, and how to send at the bottom.
//
// It reads decoded state - state.Companion - rather than the console tail,
// which is the whole difference between this and what it replaces. A channel
// is a name and an index, a contact carries a hop count, and a message has a
// sender and a time. None of that survives being formatted into a terminal
// line, which is why the previous version could only show one.
package workbench

import (
	"fmt"
	"strings"
	"time"

	"gioui.org/layout"
	"gioui.org/op"

	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/ui/comp"
	"github.com/MeshBench/meshbench/internal/ui/theme"
)

// clientPane draws the client, or says why it cannot.
//
// Settings replaces the whole of this, rail included: it is the node's radio
// form, not a conversation, and a channel list beside it would only invite
// picking a channel that Apply then has nothing to do with.
func (c *companionTab) clientPane(t *theme.Theme, gtx layout.Context, cs state.Companion, connected bool) layout.Dimensions {
	if !connected {
		return centreNote(t, gtx,
			"not connected - the workbench is not touching this node's port.\n"+
				"Connect to claim it, or serve it over TCP for your own client.")
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return c.clientToggle(t, gtx)
		}),
		layout.Rigid(comp.HRule(t)),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			if c.clientSettings {
				return c.radioPane(t, gtx, cs)
			}
			return layout.Flex{}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					gtx.Constraints.Min.X = gtx.Dp(186)
					gtx.Constraints.Max.X = gtx.Dp(186)
					return c.rail(t, gtx, cs)
				}),
				layout.Rigid(comp.VRule(t)),
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					return layout.UniformInset(t.Sp.S).Layout(gtx,
						func(gtx layout.Context) layout.Dimensions {
							return c.conversation(t, gtx, cs)
						})
				}),
			)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if c.clientSettings {
				return layout.Dimensions{}
			}
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(comp.HRule(t)),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return c.composer(t, gtx, cs)
				}),
			)
		}),
	)
}

// clientToggle switches Client between the conversation and the node's radio
// settings - the one thing a phone app also has a screen for, so it does not
// need a mode of its own beside the one that already speaks for the radio.
func (c *companionTab) clientToggle(t *theme.Theme, gtx layout.Context) layout.Dimensions {
	return layout.Inset{Left: t.Sp.S, Right: t.Sp.S, Top: t.Sp.XXS, Bottom: t.Sp.XXS}.
		Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return c.click("client:messages").Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return modeChip(t, gtx, "Messages", !c.clientSettings)
					})
				}),
				layout.Rigid(layout.Spacer{Width: t.Sp.XXS}.Layout),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return c.click("client:settings").Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return modeChip(t, gtx, "Settings", c.clientSettings)
					})
				}),
			)
		})
}

// rail is the channel list and the contact list.
func (c *companionTab) rail(t *theme.Theme, gtx layout.Context, cs state.Companion) layout.Dimensions {
	var kids []layout.Widget
	kids = append(kids, railHeading(t, "Channels"))
	if len(cs.Channels) == 0 {
		kids = append(kids, railNote(t, "none read yet"))
	}
	for i := range cs.Channels {
		ch := cs.Channels[i]
		sel := ch.Index == c.channelIdx
		ck := c.click(fmt.Sprintf("chan:%d", ch.Index))
		kids = append(kids, func(gtx layout.Context) layout.Dimensions {
			return ck.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return railRow(t, gtx, ch.Name, unreadLabel(ch.Unread), sel)
			})
		})
	}
	kids = append(kids, func(gtx layout.Context) layout.Dimensions {
		return layout.Inset{Left: t.Sp.S, Right: t.Sp.S, Top: t.Sp.XXS}.Layout(gtx,
			func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						return c.newChan.Layout(t, gtx)
					}),
					layout.Rigid(layout.Spacer{Width: t.Sp.XS}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return c.addChan.Layout(t, gtx)
					}),
				)
			})
	})
	kids = append(kids, layout.Spacer{Height: t.Sp.S}.Layout)
	kids = append(kids, railHeading(t, "Contacts"))
	if len(cs.Contacts) == 0 {
		kids = append(kids, railNote(t,
			"none yet - a companion learns these from adverts"))
	}
	for i := range cs.Contacts {
		ct := cs.Contacts[i]
		kids = append(kids, func(gtx layout.Context) layout.Dimensions {
			return railRow(t, gtx, ct.Name, hopsLabel(ct.Hops), false)
		})
	}
	kids = append(kids, func(gtx layout.Context) layout.Dimensions {
		return layout.Inset{Top: t.Sp.S, Left: t.Sp.S, Right: t.Sp.S}.Layout(gtx,
			func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return c.advertBtn.Layout(t, gtx)
					}),
					layout.Rigid(layout.Spacer{Width: t.Sp.XS}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return c.refreshBtn.Layout(t, gtx)
					}),
				)
			})
	})
	return comp.List(t, &c.railList, len(kids), func(gtx layout.Context, i int) layout.Dimensions {
		return kids[i](gtx)
	})(gtx)
}

// railHeading is one of the two section labels in the rail.
func railHeading(t *theme.Theme, s string) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		return layout.Inset{Left: t.Sp.S, Right: t.Sp.S, Top: t.Sp.XS, Bottom: t.Sp.XXS}.
			Layout(gtx, comp.Text(t, t.Sz.Caption, t.P.Faint, strings.ToUpper(s)))
	}
}

func railNote(t *theme.Theme, s string) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		return layout.Inset{Left: t.Sp.S, Right: t.Sp.S, Bottom: t.Sp.XS}.
			Layout(gtx, comp.Text(t, t.Sz.Caption, t.P.Faint, s))
	}
}

// railRow is one channel or one contact: a name on the left and its number on
// the right, picked out when it is the one being read.
func railRow(t *theme.Theme, gtx layout.Context, name, right string, sel bool) layout.Dimensions {
	macro := op.Record(gtx.Ops)
	ink := t.P.Dim
	if sel {
		ink = t.P.Accent
	}
	dims := layout.Inset{Left: t.Sp.S, Right: t.Sp.S, Top: t.Sp.XXS, Bottom: t.Sp.XXS}.
		Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
				layout.Flexed(1, comp.OneLine(t, t.Sz.Caption, ink, name, false)),
				layout.Rigid(comp.Mono(t, t.Sz.Caption, t.P.Faint, right)),
			)
		})
	call := macro.Stop()
	if sel {
		comp.RoundRect(gtx, dims.Size, 4, theme.Alpha(t.P.Accent, 0.12))
	}
	call.Add(gtx.Ops)
	return dims
}

func unreadLabel(n int) string {
	if n <= 0 {
		return ""
	}
	return fmt.Sprintf("%d", n)
}

// hopsLabel says how far away a contact is, and says so honestly when the
// node has not worked out a path to them.
func hopsLabel(h int) string {
	switch {
	case h < 0:
		return "no path"
	case h == 0:
		return "direct"
	case h == 1:
		return "1 hop"
	}
	return fmt.Sprintf("%d hops", h)
}

// conversation is the messages on the selected channel.
func (c *companionTab) conversation(t *theme.Theme, gtx layout.Context, cs state.Companion) layout.Dimensions {
	var msgs []state.CompanionMessage
	for _, m := range cs.Messages {
		if m.Channel && m.ChannelIdx == c.channelIdx {
			msgs = append(msgs, m)
		}
	}
	title := channelName(cs, c.channelIdx)
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Bottom: t.Sp.XS}.Layout(gtx,
				func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
						layout.Rigid(comp.Text(t, t.Sz.Section, t.P.Ink, title)),
						layout.Flexed(1, comp.Spacer),
						layout.Rigid(comp.Text(t, t.Sz.Caption, t.P.Faint,
							fmt.Sprintf("channel %d  ·  %d messages", c.channelIdx, len(msgs)))),
					)
				})
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			if len(msgs) == 0 {
				return centreNote(t, gtx,
					"nothing on this channel yet.\n"+
						"Everything here came out of the firmware - there are no invented replies.")
			}
			c.convoList.ScrollToEnd = true
			return comp.List(t, &c.convoList, len(msgs), func(gtx layout.Context, i int) layout.Dimensions {
				return messageRow(t, gtx, msgs[i])
			})(gtx)
		}),
	)
}

// channelName is what to call the selected channel, falling back to its index
// when the node has not told us a name for that slot.
func channelName(cs state.Companion, idx uint8) string {
	for _, ch := range cs.Channels {
		if ch.Index == idx {
			return ch.Name
		}
	}
	return fmt.Sprintf("channel %d", idx)
}

// messageRow is one line of conversation, with its receipt underneath when
// there is one.
func messageRow(t *theme.Theme, gtx layout.Context, m state.CompanionMessage) layout.Dimensions {
	who := t.P.Dim
	if m.Mine {
		who = t.P.Accent
	}
	return layout.Inset{Top: t.Sp.XXS, Bottom: t.Sp.XXS}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Alignment: layout.Start}.Layout(gtx,
						comp.Fixed(gtx, 52, comp.Mono(t, t.Sz.Caption, t.P.Faint, clockOf(m.At))),
						comp.Fixed(gtx, 104, comp.OneLine(t, t.Sz.Caption, who, m.From, false)),
						layout.Flexed(1, comp.Text(t, t.Sz.Caption, t.P.Ink, m.Text)),
					)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					r := receiptOf(m)
					if r == "" {
						return layout.Dimensions{}
					}
					return layout.Inset{Left: t.Sp.L}.Layout(gtx,
						comp.Text(t, t.Sz.Caption, t.P.Faint, r))
				}),
			)
		})
}

// receiptOf is how a message that arrived got here, which a real phone cannot
// tell you and this can, because the simulator watched the whole path.
//
// Only for one that arrived. What became of a message this client sent is not
// known here: nothing correlates a companion message with the ledger, so a
// sent line carries no second line rather than an empty one.
func receiptOf(m state.CompanionMessage) string {
	// Hops below zero is "did not arrive by flood" - the count is unknown,
	// not negative - and zero is a message with nothing to explain.
	if m.Mine || m.Hops <= 0 {
		return ""
	}
	return fmt.Sprintf("↳ %d hops  ·  %+.1f dB", m.Hops, m.SNRdB)
}

func clockOf(at time.Time) string {
	if at.IsZero() {
		return ""
	}
	return at.Format("15:04")
}

// composer is the scope, the path hash size, the message box and Send.
//
// Scope sits here rather than in the radio settings because it is a property
// of the message being sent, not of the application - and because a companion
// has no command line to set it from, which an earlier version learned the
// hard way by setting it with a repeater command that went nowhere.
//
// The hash size is here for a different reason, and the interface should not
// pretend otherwise: it is a node preference the firmware reads at send time,
// not a field on the packet. Choosing it beside the message is how it is
// used - "send this one with three-byte hashes and watch what comes back" -
// so the session sets the preference and sends in that order, and the note
// beside the control says the setting stays.
func (c *companionTab) composer(t *theme.Theme, gtx layout.Context, cs state.Companion) layout.Dimensions {
	return layout.UniformInset(t.Sp.S).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(comp.Text(t, t.Sz.Caption, t.P.Faint, "scope ")),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						gtx.Constraints.Max.X = gtx.Dp(120)
						return c.scope.Layout(t, gtx)
					}),
					layout.Rigid(layout.Spacer{Width: t.Sp.XS}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return c.applyScope.Layout(t, gtx)
					}),
					layout.Rigid(layout.Spacer{Width: t.Sp.S}.Layout),
					layout.Flexed(1, comp.OneLine(t, t.Sz.Caption, t.P.Faint, scopeNote(cs), false)),
				)
			}),
			layout.Rigid(layout.Spacer{Height: t.Sp.XS}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(comp.Text(t, t.Sz.Caption, t.P.Faint, "path hash ")),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return c.hashPicker(t, gtx)
					}),
					layout.Rigid(layout.Spacer{Width: t.Sp.S}.Layout),
					layout.Flexed(1, comp.OneLine(t, t.Sz.Caption, t.P.Faint,
						hashComposerNote(c.pathHash, cs.PathHashBytes), false)),
				)
			}),
			layout.Rigid(layout.Spacer{Height: t.Sp.XS}.Layout),
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
		)
	})
}

// hashComposerNote says whether the choice differs from the node's, and that
// choosing one changes the node rather than only this message.
func hashComposerNote(chosen, atNode int) string {
	switch {
	case chosen == atNode:
		return "bytes of hash each hop adds"
	case atNode == 0:
		return fmt.Sprintf("the node has not said which it uses - sending will set it to %d, and it stays", chosen)
	}
	return fmt.Sprintf("the node is set to %d - sending will change it to %d, and it stays",
		atNode, chosen)
}

// scopeNote says what the node holds, which is not always what was typed into
// the box beside it - the node is the one that decides.
func scopeNote(cs state.Companion) string {
	switch {
	case !cs.Scoped:
		return "not read from the node yet"
	case cs.Scope == "":
		return "the node sends unscoped"
	}
	return "the node sends under " + cs.Scope
}

// centreNote is an empty state, said in the middle of the space it fills.
func centreNote(t *theme.Theme, gtx layout.Context, s string) layout.Dimensions {
	return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		var kids []layout.FlexChild
		for _, line := range strings.Split(s, "\n") {
			kids = append(kids, layout.Rigid(comp.Text(t, t.Sz.Caption, t.P.Faint, line)))
		}
		return layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle}.Layout(gtx, kids...)
	})
}
