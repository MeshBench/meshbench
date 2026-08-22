// Choosing which build one node runs.
//
// Two places ask the same question - the firmware cell in the Nodes running
// table, and the Settings tab of a node's own window - so there is one control
// and both open it. A second copy would be a second set of behaviours to keep
// in step, and the first thing to drift would be which builds it offers.
package workbench

import (
	"fmt"
	"image"
	"strings"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/widget"

	"github.com/MeshBench/meshbench/internal/ui/comp"
	"github.com/MeshBench/meshbench/internal/ui/theme"
)

// buildPicker is the "what should this node run?" list.
//
// A filtered list rather than a row of buttons, because the number of
// installed builds is however many somebody has installed - thirty-nine on
// this machine, and nine already overflowed a row. A control that works until
// you install a tenth is not a control.
type buildPicker struct {
	// node is whose firmware is being chosen, and "" when nothing is being
	// chosen. The one piece of state that says whether this is on screen.
	node   string
	builds []string
	btns   []comp.Button
	read   bool
	list   widget.List
	// filter narrows the list, because there are dozens.
	filter comp.Field
	shown  []int
	cancel comp.Button

	// OnPick is given the node and the build it should run.
	OnPick func(node, version string)
}

// open asks which build this node should run.
func (p *buildPicker) open(node string) { p.node = node }

// showing reports whether the list is up, which is what tells a panel to draw
// it over itself and to give up the space underneath.
func (p *buildPicker) showing() bool { return p.node != "" }

// shut puts it away, filter and all: a filter left behind from last time
// greets the next node with a list that appears to be missing builds.
func (p *buildPicker) shut() {
	p.node, p.filter.Editor = "", widget.Editor{}
}

// load reads the machine's library, once.
//
// Called from wherever the picker is drawn rather than from a panel's own
// setup, so a panel that grows this control does not also have to remember to
// fill it.
func (p *buildPicker) load() {
	if p.read {
		return
	}
	p.read = true
	p.builds = installedBuilds()
	p.btns = make([]comp.Button, len(p.builds))
	for i := range p.btns {
		p.btns[i].Label = p.builds[i]
		p.btns[i].Kind = comp.Secondary
	}
}

// body is the card: who it is about, a filter, and the builds.
func (p *buildPicker) body(t *theme.Theme) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		p.load()
		if p.node == "" {
			return comp.Text(t, t.Sz.Caption, t.P.Faint,
				"click a firmware cell to change what that node runs")(gtx)
		}
		if p.cancel.Label == "" {
			p.cancel.Label, p.cancel.Kind = "cancel", comp.Quiet
		}
		if p.cancel.Click.Clicked(gtx) {
			p.shut()
			return layout.Dimensions{}
		}
		for i := range p.btns {
			if p.btns[i].Click.Clicked(gtx) && p.OnPick != nil {
				p.OnPick(p.node, p.builds[i])
				p.shut()
				return layout.Dimensions{}
			}
		}

		// Which builds survive the box. A horizontal row of thirty-nine
		// buttons is not a control, and it put cancel where the first click
		// lands - so choosing a build closed the list instead.
		want := strings.ToLower(strings.TrimSpace(p.filter.Editor.Text()))
		shown := p.shown[:0]
		for i := range p.builds {
			if want == "" || strings.Contains(strings.ToLower(p.builds[i]), want) {
				shown = append(shown, i)
			}
		}
		p.shown = shown

		p.list.Axis = layout.Vertical
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(comp.Text(t, t.Sz.Body, t.P.Ink,
						"What should "+p.node+" run?")),
					layout.Flexed(1, comp.Spacer),
					btn(t, &p.cancel),
				)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				p.filter.Hint = fmt.Sprintf("filter %d builds", len(p.builds))
				p.filter.Editor.SingleLine = true
				return p.filter.Layout(t, gtx)
			}),
			layout.Rigid(layout.Spacer{Height: t.Sp.XS}.Layout),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				if len(shown) == 0 {
					return layout.Center.Layout(gtx, comp.Text(t, t.Sz.Caption,
						t.P.Faint, "nothing matches that"))
				}
				return comp.List(t, &p.list, len(shown),
					func(gtx layout.Context, i int) layout.Dimensions {
						return layout.Inset{Bottom: t.Sp.XS}.Layout(gtx,
							func(gtx layout.Context) layout.Dimensions {
								return p.btns[shown[i]].Layout(t, gtx)
							})
					})(gtx)
			}),
		)
	}
}

// overlay draws the card over whatever asked for it, centred and dimmed.
//
// Over rather than inside: the first attempt was a rigid child after a flexed
// table, so the table took the space and the list was laid out at zero height
// - open, invisible, and impossible to click. Choosing a build was unreachable
// by pointing at anything, which is the only way a person would try.
func (p *buildPicker) overlay(t *theme.Theme, gtx layout.Context) layout.Dimensions {
	comp.FillRect(gtx, gtx.Constraints.Max, theme.Alpha(t.P.Ground, 0.86))
	return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		w, h := gtx.Dp(520), gtx.Dp(420)
		if w > gtx.Constraints.Max.X {
			w = gtx.Constraints.Max.X
		}
		if h > gtx.Constraints.Max.Y {
			h = gtx.Constraints.Max.Y
		}
		gtx.Constraints.Min = image.Pt(w, h)
		gtx.Constraints.Max = image.Pt(w, h)
		macro := op.Record(gtx.Ops)
		dims := layout.UniformInset(t.Sp.M).Layout(gtx, p.body(t))
		call := macro.Stop()
		comp.RoundRect(gtx, dims.Size, 6, t.P.Panel)
		comp.Border(gtx, dims.Size, 6, 1, t.P.Rule)
		call.Add(gtx.Ops)
		return dims
	})
}
