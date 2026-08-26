// The card in a board's slot, in the tab where the board is drawn.
//
// Here rather than in Settings because a card is hardware: it is the thing in
// the slot beside the screen and the buttons, and the question people arrive
// with - "is there a card in this one" - is asked while looking at the board.
//
// A slot is not a fitted card. Two of the same handheld in one network, one
// with storage and one without, is an ordinary thing to want, and the board
// profile can only say that the slot exists.
package workbench

import (
	"fmt"

	"gioui.org/layout"

	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/ui/comp"
	"github.com/MeshBench/meshbench/internal/ui/shell"
	"github.com/MeshBench/meshbench/internal/ui/theme"
)

// cardControls is the slot's row: fitted or not, which file, and erasing it.
type cardControls struct {
	fitted comp.Check
	wipe   comp.Button
	pick   comp.Button
	reset  comp.Button
	built  bool
	// confirm is the wipe's second press. Erasing a card is not undoable and
	// the firmware's settings live on it.
	confirm bool
	// seeded is the node the switch was last filled from, so the switch
	// follows the window rather than keeping the last node's answer.
	seeded string
}

func (c *cardControls) build() {
	c.fitted.Label = "card in the slot"
	c.wipe.Label, c.wipe.Kind = "erase card", comp.Secondary
	c.pick.Label, c.pick.Kind = "use another file...", comp.Secondary
	c.reset.Label, c.reset.Kind = "back to its own", comp.Secondary
	c.built = true
}

// card is the whole section, drawn under the board's facts.
func (p *nodeWindowPanel) card(t *theme.Theme, gtx layout.Context,
	s *state.Snapshot) layout.Dimensions {

	c := &p.cardCtl
	if !c.built {
		c.build()
	}
	n, found := nodeInSnapshot(s, p.node)
	if !found || !n.CardSlot {
		// A board with no slot says so once, rather than offering controls
		// for storage it has nowhere to put.
		return layout.Dimensions{}
	}
	if c.seeded != p.node {
		c.seeded = p.node
		c.fitted.Bool.Value = n.CardFitted
		c.confirm = false
	}
	p.cardClicks(gtx, n)

	own := !n.CardShared
	where := n.CardFile
	if where == "" {
		where = "(none)"
	}
	note := "64 MB, kept beside this node's flash and named after it"
	if !own {
		note = "a file this node was handed, which outlives its working directory"
	}
	if n.CardRequired {
		note = "this build will not get far without storage, so the slot is " +
			"filled whatever the switch says"
	}

	c.wipe.Label = "erase card"
	if c.confirm {
		c.wipe.Label = "sure? this is not undoable"
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(layout.Spacer{Height: t.Sp.M}.Layout),
		layout.Rigid(comp.Text(t, t.Sz.Body, t.P.Ink, "Card slot")),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: t.Sp.XS}.Layout(gtx,
				func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return c.fitted.LayoutSwitch(t, gtx)
						}),
						layout.Rigid(layout.Spacer{Width: t.Sp.S}.Layout),
						layout.Rigid(comp.Text(t, t.Sz.Caption, t.P.Dim,
							cardStateLabel(n))),
					)
				})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: t.Sp.XS}.Layout(gtx,
				comp.Text(t, t.Sz.Caption, t.P.Faint, note))
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: t.Sp.XS}.Layout(gtx,
				comp.Mono(t, t.Sz.Caption, t.P.Dim, where))
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: t.Sp.S}.Layout(gtx,
				func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return layout.Inset{Right: t.Sp.S}.Layout(gtx,
								func(gtx layout.Context) layout.Dimensions {
									return c.pick.Layout(t, gtx)
								})
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							if own {
								return layout.Dimensions{}
							}
							return layout.Inset{Right: t.Sp.S}.Layout(gtx,
								func(gtx layout.Context) layout.Dimensions {
									return c.reset.Layout(t, gtx)
								})
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return c.wipe.Layout(t, gtx)
						}),
					)
				})
		}),
	)
}

// cardStateLabel says what the slot holds, and who decided.
func cardStateLabel(n state.Node) string {
	switch {
	case n.CardRequired:
		return "fitted - required by this build"
	case n.CardFitted:
		return "fitted"
	default:
		return "empty"
	}
}

// cardClicks runs the controls. Each goes through the verb a script would use.
func (p *nodeWindowPanel) cardClicks(gtx layout.Context, n state.Node) {
	c := &p.cardCtl
	if c.fitted.Bool.Update(gtx) && p.OnDo != nil {
		p.OnDo("node.card", map[string]any{
			"node": p.node, "fitted": c.fitted.Bool.Value,
		})
	}
	if c.wipe.Click.Clicked(gtx) {
		// Asked twice, in place. A card holds the firmware's own settings and
		// there is no way back from erasing it.
		if !c.confirm {
			c.confirm = true
		} else if p.OnDo != nil {
			c.confirm = false
			p.OnDo("node.card", map[string]any{"node": p.node, "wipe": true})
		}
	}
	if c.reset.Click.Clicked(gtx) && p.OnDo != nil {
		p.OnDo("node.card", map[string]any{"node": p.node, "file": ""})
	}
	if c.pick.Click.Clicked(gtx) && shell.Browse != nil {
		start := n.CardFile
		node := p.node
		do := p.OnDo
		go func() {
			got, err := shell.Browse(
				fmt.Sprintf("A card for %s", node), start,
				shell.PathAsk{Kind: shell.PathFile, FilterName: "Card images",
					Extensions: []string{"img", "bin", "iso"}})
			if err != nil || got == "" || do == nil {
				return
			}
			do("node.card", map[string]any{"node": node, "file": got})
		}()
	}
}

// nodeInSnapshot is one node's row, by name.
func nodeInSnapshot(s *state.Snapshot, name string) (state.Node, bool) {
	if s == nil {
		return state.Node{}, false
	}
	for i := range s.Nodes {
		if s.Nodes[i].Name == name {
			return s.Nodes[i], true
		}
	}
	return state.Node{}, false
}

// cardAuditRow is the slot's controls with none of its prose.
//
// For the flat audit layout only. The section as drawn is three paragraphs
// tall, and stacked with every other pane it pushed the console's send button
// off the bottom of the audit's canvas - which the audit then reports as a
// dead control, correctly and uselessly.
func (p *nodeWindowPanel) cardAuditRow(t *theme.Theme, gtx layout.Context,
	s *state.Snapshot) layout.Dimensions {

	c := &p.cardCtl
	if !c.built {
		c.build()
	}
	n, found := nodeInSnapshot(s, p.node)
	if !found || !n.CardSlot {
		return layout.Dimensions{}
	}
	p.cardClicks(gtx, n)
	return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return c.fitted.LayoutSwitch(t, gtx)
		}),
		layout.Rigid(layout.Spacer{Width: t.Sp.S}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return c.pick.Layout(t, gtx)
		}),
		layout.Rigid(layout.Spacer{Width: t.Sp.S}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if !n.CardShared {
				return layout.Dimensions{}
			}
			return c.reset.Layout(t, gtx)
		}),
		layout.Rigid(layout.Spacer{Width: t.Sp.S}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return c.wipe.Layout(t, gtx)
		}),
	)
}
