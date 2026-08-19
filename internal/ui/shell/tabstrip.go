// The strip of tabs above a region, and the affordances on it.
//
// A region can hold more than one panel now, so it needs a way to say which
// ones and to choose between them. The strip is also where a panel leaves
// from: dragged to another region, closed with the cross, or sent to a window
// of its own with the pane glyph.
//
// The tabs handle their own pointer events rather than being clickables,
// because a tab is both a button and something to pick up, and a clickable
// swallows the press a drag begins with.
//
// These are view-changing controls - they move nothing in the world - so they
// have their own tests rather than joining the audit of widgets that do.
package shell

import (
	"image"

	"gioui.org/f32"
	"gioui.org/io/event"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/widget"

	"github.com/MeshBench/meshbench/internal/ui/comp"
	"github.com/MeshBench/meshbench/internal/ui/theme"
)

// tabTag is the pooled pointer tag for one tab of one region. Pooled by
// region and name, because a tag rebuilt every frame never sees the release
// that completes its press.
func (sh *Shell) tabTag(key string) *tabHandle {
	if sh.tabTags == nil {
		sh.tabTags = map[string]*tabHandle{}
	}
	h := sh.tabTags[key]
	if h == nil {
		h = &tabHandle{}
		sh.tabTags[key] = h
	}
	return h
}

// tabHandle is one tab's identity to the pointer.
type tabHandle struct{ pressed bool }

// tabClick is the pooled clickable for a strip's own buttons - the cross and
// the pane glyph - which are ordinary presses.
func (sh *Shell) tabClick(key string) *widget.Clickable {
	if sh.tabClicks == nil {
		sh.tabClicks = map[string]*widget.Clickable{}
	}
	c := sh.tabClicks[key]
	if c == nil {
		c = &widget.Clickable{}
		sh.tabClicks[key] = c
	}
	return c
}

// tabStrip draws one region's tabs and handles them.
//
// It returns nothing: what it changes is the arrangement, which the rest of
// the frame reads.
func (sh *Shell) tabStrip(t *theme.Theme, gtx layout.Context, ref regionRef) layout.Dimensions {
	c := sh.regionAt(sh.View, ref)
	if c == nil {
		return layout.Dimensions{}
	}
	focused := sh.focus == ref
	origin := sh.regionRect[ref].Min
	threshold := float32(gtx.Dp(dragThresholdDp))
	// x accumulates along the strip, so a pointer position inside a tab can
	// be turned into one in the window's own coordinates - which is what the
	// drop test asks about.
	tabX := 0

	kids := make([]layout.FlexChild, 0, len(c.Tabs)*2+2)
	for i, name := range c.Tabs {
		i, name := i, name
		key := regionKey(sh.View, ref) + "/" + name
		h := sh.tabTag(key)
		startX := tabX
		toWindow := func(p f32.Point) f32.Point {
			return f32.Pt(float32(origin.X+startX)+p.X, float32(origin.Y)+p.Y)
		}
		for {
			ev, ok := gtx.Event(pointer.Filter{
				Target: h,
				Kinds:  pointer.Press | pointer.Drag | pointer.Release | pointer.Cancel,
			})
			if !ok {
				break
			}
			e, ok := ev.(pointer.Event)
			if !ok {
				continue
			}
			switch e.Kind {
			case pointer.Press:
				h.pressed = true
				sh.startTabDrag(name, ref, toWindow(e.Position))
			case pointer.Drag:
				sh.moveTabDrag(toWindow(e.Position), threshold)
			case pointer.Release:
				// A press that never travelled is a click: show this tab.
				// One that did is a move, and the drop decides where to.
				if !sh.dropTabDrag() && h.pressed {
					c.Active, sh.focus = i, ref
				}
				h.pressed = false
			case pointer.Cancel:
				h.pressed = false
				sh.cancelTabDrag()
			}
		}
		closer := sh.tabClick(key + "\x00close")
		if closer.Clicked(gtx) {
			sh.Undock(name)
			return sh.tabStrip(t, gtx, ref)
		}
		active := i == c.Active
		carried := sh.drag != nil && sh.drag.moved && sh.drag.Name == name
		kids = append(kids, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			fg := t.P.Dim
			if active {
				fg = t.P.Ink
			}
			if carried {
				// The tab it came from, ghosted: the panel has not left yet,
				// and a tab that vanishes mid-drag reads as one that was
				// dropped somewhere already.
				fg = t.P.Faint
			}
			in := layout.Inset{Left: t.Sp.S, Right: t.Sp.XS, Top: t.Sp.XS, Bottom: t.Sp.XS}
			macro := op.Record(gtx.Ops)
			dims := in.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(comp.Text(t, t.Sz.Caption, fg, name)),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						// The cross belongs to the tab being looked at:
						// a cross on every tab is a row of crosses.
						if !active {
							return layout.Spacer{Width: t.Sp.S}.Layout(gtx)
						}
						return closer.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return glyphBox(t, gtx, "close")
						})
					}),
				)
			})
			call := macro.Stop()
			if active {
				comp.RoundRect(gtx, dims.Size, 5, t.P.Panel)
			}
			// The tab's own area first, over its whole size, so a press
			// anywhere on it can start a drag - and the content after it, so
			// the cross inside keeps the small area it needs. Registering the
			// tab last would put it over the cross and the cross would never
			// be pressed again.
			area := clip.Rect{Max: dims.Size}.Push(gtx.Ops)
			event.Op(gtx.Ops, h)
			call.Add(gtx.Ops)
			area.Pop()
			tabX = startX + dims.Size.X
			return dims
		}))
	}
	kids = append(kids, layout.Flexed(1, comp.Spacer))
	// Sending the shown panel to its own window: the secondary way to see a
	// panel, kept one click away rather than being the only way in.
	if shown := c.shown(); shown != "" {
		if p := sh.Panels[shown]; p != nil && p.Windowable {
			out := sh.tabClick(regionKey(sh.View, ref) + "\x00pop")
			if out.Clicked(gtx) && sh.OnPopOut != nil {
				sh.OnPopOut(shown)
			}
			kids = append(kids, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return out.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return glyphBox(t, gtx, "window")
				})
			}))
		}
	}
	return layout.Inset{Left: t.Sp.XS, Right: t.Sp.XS}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions {
			dims := layout.Flex{Alignment: layout.Middle}.Layout(gtx, kids...)
			// The focused region's strip carries an accent underline, so
			// "where will the next panel land" is answerable by looking.
			if focused {
				h := gtx.Dp(2)
				off := op.Offset(image.Pt(0, dims.Size.Y-h)).Push(gtx.Ops)
				comp.FillRect(gtx, image.Pt(dims.Size.X, h), t.P.Accent)
				off.Pop()
			}
			return dims
		})
}

// regionKey names a region for widget pooling, stable across frames.
func regionKey(v View, r regionRef) string {
	return v.String() + ":" + itoa(r.Row) + ":" + itoa(r.Col)
}
