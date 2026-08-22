// The title bar a window without the compositor's own draws.
//
// Under Wayland the only always-on-top a client can ask for is a
// wlr-layer-shell window (the Gio fork's app.LayerShell), and the protocol
// forbids the compositor from decorating one: no title bar, no taskbar
// entry, nothing to drag by. So the window draws its own - this bar - and
// dragging it moves the window by moving its margins, which is the one
// mechanism the protocol offers. There is no minimise: the protocol has no
// such state for a layer surface, and the bar says so by not drawing one.
package comp

import (
	"image"
	"image/color"

	"gioui.org/f32"
	"gioui.org/io/event"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/widget"

	"github.com/MeshBench/meshbench/internal/ui/theme"
)

// TitleBar is a window's chrome when no compositor gave it any: the title,
// a drag handle across the bar, and maximise and close glyphs at the right.
// The glyphs are drawn rather than taken from a font, for the same reason
// the menu's are.
//
// A TitleBar is its own pointer identity - never copy one, and never build
// it per frame; the drag handoff depends on the address staying put. It
// collects its own glyph clicks at the top of Layout, because a
// widget.Clickable consumes its click events when it is laid out; ask
// CloseClicked and MaximiseClicked after Layout, the same as Drag.
type TitleBar struct {
	// Title is drawn at the left.
	Title string
	// Maximised draws the restore glyph rather than the maximise one.
	Maximised bool

	close    widget.Clickable
	maximise widget.Clickable
	// closePressed and maximisePressed carry a glyph's click from Layout,
	// where it is read, to the ask that follows it.
	closePressed    bool
	maximisePressed bool
	drag            struct {
		held  bool
		last  f32.Point
		moved image.Point
	}
}

// CloseClicked reports one press of the close glyph.
func (b *TitleBar) CloseClicked() bool {
	v := b.closePressed
	b.closePressed = false
	return v
}

// MaximiseClicked reports one press of the maximise (or, when maximised,
// restore) glyph.
func (b *TitleBar) MaximiseClicked() bool {
	v := b.maximisePressed
	b.maximisePressed = false
	return v
}

// Drag is how far the bar was dragged since the last ask, in pixels. The
// caller turns that into the window's new place; a maximised window has no
// place to be dragged to, and the drag is simply not reported while the
// maximise glyph reads restore - the same rule a decorated window follows.
func (b *TitleBar) Drag() image.Point {
	d := b.drag.moved
	b.drag.moved = image.Pt(0, 0)
	return d
}

// Layout draws the bar: the title and drag handle filling the width, the
// glyphs at the right.
func (b *TitleBar) Layout(t *theme.Theme, gtx layout.Context) layout.Dimensions {
	// The glyphs' clicks are read here, before their Clickables are laid
	// out and consume the events themselves.
	if b.close.Clicked(gtx) {
		b.closePressed = true
	}
	if b.maximise.Clicked(gtx) {
		b.maximisePressed = true
	}
	for {
		ev, ok := gtx.Event(pointer.Filter{
			Target: b,
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
			b.drag.held, b.drag.last = true, e.Position
		case pointer.Drag:
			if !b.drag.held {
				break
			}
			d := e.Position.Sub(b.drag.last)
			b.drag.moved = b.drag.moved.Add(image.Pt(int(d.X), int(d.Y)))
			// The caller moves the window by d, so the pointer's place in
			// it moves by -d. Expecting that here stops the compositor's
			// correction event - same pointer, new surface-relative place -
			// from being read as a drag straight back.
			b.drag.last = e.Position.Sub(d)
		case pointer.Release, pointer.Cancel:
			b.drag.held = false
		}
	}

	bar := image.Pt(gtx.Constraints.Max.X, gtx.Dp(t.RowHeight()))
	FillRect(gtx, bar, t.P.Panel)
	FillRect(gtx, image.Pt(bar.X, gtx.Dp(1)), t.P.Rule)

	// The bar is only its own height tall; without this the glyphs, centred
	// in the cross axis, would sit in the middle of whatever space the bar
	// was given rather than in the bar.
	gtx.Constraints.Max.Y = bar.Y
	gtx.Constraints.Min.Y = 0

	var kids []layout.FlexChild
	kids = append(kids, layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
		// The handle is the title's half of the bar, so the glyphs are not
		// part of it: a click on close that also started a drag would be a
		// window that jumped as it closed.
		defer clip.Rect{Max: image.Pt(gtx.Constraints.Max.X, bar.Y)}.Push(gtx.Ops).Pop()
		event.Op(gtx.Ops, b)
		return layout.Inset{Left: t.Sp.M}.Layout(gtx,
			func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints.Max.Y = bar.Y
				return layout.W.Layout(gtx, Text(t, t.Sz.Label, t.P.Dim, b.Title))
			})
	}))
	kids = append(kids,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return glyphButton(&b.maximise, gtx, bar.Y, t, func(gtx layout.Context, t *theme.Theme) {
				maximiseGlyph(gtx, t, b.Maximised)
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return glyphButton(&b.close, gtx, bar.Y, t, closeGlyph)
		}),
	)
	dims := layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.
		Layout(gtx, kids...)
	dims.Size = bar
	return dims
}

// glyphButton is one glyph cell: a clickable square the height of the bar,
// tinted under the pointer the way the quiet buttons are, the glyph drawn
// rather than taken from a font - a font is a file a machine may not have.
func glyphButton(c *widget.Clickable, gtx layout.Context, h int, t *theme.Theme, glyph func(layout.Context, *theme.Theme)) layout.Dimensions {
	return c.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		cell := image.Pt(h, h)
		macro := op.Record(gtx.Ops)
		glyph(gtx, t)
		call := macro.Stop()
		if c.Hovered() {
			RoundRect(gtx, cell, 6, theme.Alpha(t.P.Ink, 0.07))
		}
		gs := gtx.Dp(11)
		off := op.Offset(image.Pt((cell.X-gs)/2, (cell.Y-gs)/2)).Push(gtx.Ops)
		call.Add(gtx.Ops)
		off.Pop()
		return layout.Dimensions{Size: cell}
	})
}

// closeGlyph is the cross.
func closeGlyph(gtx layout.Context, t *theme.Theme) {
	stroke(gtx, t.P.Dim, func(p *clip.Path) {
		const m = 8.5
		s := float32(gtx.Dp(11)) / 2
		p.MoveTo(f32.Pt(s-m/2, s-m/2))
		p.LineTo(f32.Pt(s+m/2, s+m/2))
		p.MoveTo(f32.Pt(s+m/2, s-m/2))
		p.LineTo(f32.Pt(s-m/2, s+m/2))
	})
}

// maximiseGlyph is the square, or two squares when maximised means restore.
func maximiseGlyph(gtx layout.Context, t *theme.Theme, maximised bool) {
	const m = 9
	s := float32(gtx.Dp(11)) / 2
	if maximised {
		stroke(gtx, t.P.Dim, func(p *clip.Path) {
			p.MoveTo(f32.Pt(s-m/2+2, s-m/2))
			p.LineTo(f32.Pt(s+m/2+2, s-m/2))
			p.LineTo(f32.Pt(s+m/2+2, s+m/2))
			p.LineTo(f32.Pt(s-m/2+2, s+m/2))
			p.Close()
			p.MoveTo(f32.Pt(s-m/2, s-m/2+2))
			p.LineTo(f32.Pt(s+m/2, s-m/2+2))
			p.LineTo(f32.Pt(s+m/2, s+m/2+2))
			p.LineTo(f32.Pt(s-m/2, s+m/2+2))
			p.Close()
		})
		return
	}
	stroke(gtx, t.P.Dim, func(p *clip.Path) {
		p.MoveTo(f32.Pt(s-m/2, s-m/2))
		p.LineTo(f32.Pt(s+m/2, s-m/2))
		p.LineTo(f32.Pt(s+m/2, s+m/2))
		p.LineTo(f32.Pt(s-m/2, s+m/2))
		p.Close()
	})
}

// stroke fills a path's outline, at the width the glyphs read at.
func stroke(gtx layout.Context, c color.NRGBA, build func(*clip.Path)) {
	var p clip.Path
	p.Begin(gtx.Ops)
	build(&p)
	w := float32(gtx.Dp(1.25))
	if w < 1 {
		w = 1
	}
	paint.FillShape(gtx.Ops, c, clip.Stroke{Path: p.End(), Width: w}.Op())
}
