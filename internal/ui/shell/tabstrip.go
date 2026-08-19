// The strip of tabs above a region, and the affordances on it.
//
// A region can hold more than one panel now, so it needs a way to say which
// ones and to choose between them. The strip is also where a panel leaves
// from: closed with the cross, sent to its own window with the pane glyph.
//
// These are view-changing controls - they move nothing in the world - so they
// are plain clickables with their own test rather than audited widgets.
package shell

import (
	"image"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/widget"

	"github.com/MeshBench/meshbench/internal/ui/comp"
	"github.com/MeshBench/meshbench/internal/ui/theme"
)

// tabClick is the pooled clickable for one tab of one region. Pooled by name
// and region, because a widget rebuilt every frame never sees the release
// that completes its press.
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
	kids := make([]layout.FlexChild, 0, len(c.Tabs)*2+2)
	for i, name := range c.Tabs {
		i, name := i, name
		key := regionKey(sh.View, ref) + "/" + name
		click := sh.tabClick(key)
		if click.Clicked(gtx) {
			c.Active = i
			sh.focus = ref
		}
		closer := sh.tabClick(key + "\x00close")
		if closer.Clicked(gtx) {
			sh.Undock(name)
			return sh.tabStrip(t, gtx, ref)
		}
		active := i == c.Active
		kids = append(kids, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return click.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				fg := t.P.Dim
				if active {
					fg = t.P.Ink
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
				call.Add(gtx.Ops)
				return dims
			})
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
