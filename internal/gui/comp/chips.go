// Chips: the small capsules a panel filters and tabs with.
package comp

import (
	"image"
	"image/color"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/widget"

	"github.com/A13xB0/meshcoresim/internal/gui/theme"
)

// Chip is one filter or tab capsule: a label, an optional count, and whether
// it is the active one. The identity lives in the struct so a chip rebuilt
// every frame keeps the press that is in flight.
type Chip struct {
	Click widget.Clickable
}

// Layout draws the chip and reports its size. col tints the active state;
// zero means the accent.
func (c *Chip) Layout(t *theme.Theme, gtx layout.Context, label, count string,
	active bool, col color.NRGBA) layout.Dimensions {
	if col.A == 0 {
		col = t.P.Accent
	}
	return c.Click.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		ink, line := t.P.Dim, t.P.Rule
		if active {
			ink, line = t.P.Ink, theme.Alpha(col, 0.8)
		} else if c.Click.Hovered() {
			ink = t.P.Ink
		}
		macro := op.Record(gtx.Ops)
		dims := layout.Inset{
			Top: t.Sp.XS, Bottom: t.Sp.XS, Left: t.Sp.S, Right: t.Sp.S,
		}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			kids := []layout.FlexChild{
				layout.Rigid(Text(t, t.Sz.Caption, ink, label)),
			}
			if count != "" {
				kids = append(kids, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{Left: t.Sp.XS}.Layout(gtx,
						Mono(t, t.Sz.Caption, t.P.Faint, count))
				}))
			}
			return layout.Flex{Alignment: layout.Middle}.Layout(gtx, kids...)
		})
		call := macro.Stop()
		rr := dims.Size.Y / 2
		func() {
			defer clip.RRect{Rect: image.Rectangle{Max: dims.Size},
				NE: rr, NW: rr, SE: rr, SW: rr}.Push(gtx.Ops).Pop()
			if active {
				paint.ColorOp{Color: theme.Alpha(col, 0.14)}.Add(gtx.Ops)
				paint.PaintOp{}.Add(gtx.Ops)
			}
		}()
		strokeCapsule(gtx, dims.Size, rr, line)
		call.Add(gtx.Ops)
		return dims
	})
}

// strokeCapsule outlines a capsule of the given size.
func strokeCapsule(gtx layout.Context, sz image.Point, rr int, col color.NRGBA) {
	shape := clip.RRect{Rect: image.Rectangle{Max: sz}, NE: rr, NW: rr, SE: rr, SW: rr}
	paint.FillShape(gtx.Ops, col,
		clip.Stroke{Path: shape.Path(gtx.Ops), Width: 1}.Op())
}

// ClassColour is the one place an event class becomes a colour, so the pill
// in the table, the chip above it and the cards all agree.
func ClassColour(t *theme.Theme, class string) color.NRGBA {
	switch class {
	case "sent":
		return t.P.Accent
	case "received":
		return t.P.Good
	case "interference":
		return t.P.Warn
	case "floor":
		return t.NodeColour(theme.Observer)
	case "half-duplex":
		return t.P.Dim
	}
	return t.P.Dim
}

// ClassLabel is the class in a person's words.
func ClassLabel(class string) string {
	switch class {
	case "sent":
		return "Sent"
	case "received":
		return "Received"
	case "half-duplex":
		return "Half duplex"
	case "interference":
		return "Interference"
	case "floor":
		return "Below floor"
	}
	return class
}

// Dot is a small filled circle, for compact type columns and keys.
func Dot(col color.NRGBA, r int) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		defer clip.Ellipse(image.Rect(0, 0, 2*r, 2*r)).Push(gtx.Ops).Pop()
		paint.ColorOp{Color: col}.Add(gtx.Ops)
		paint.PaintOp{}.Add(gtx.Ops)
		return layout.Dimensions{Size: image.Pt(2*r, 2*r)}
	}
}

// TypePill is the coloured dot and word a table's type column shows.
func TypePill(t *theme.Theme, class string) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		col := ClassColour(t, class)
		return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				r := gtx.Dp(3)
				defer clip.Ellipse(image.Rect(0, 0, 2*r, 2*r)).Push(gtx.Ops).Pop()
				paint.ColorOp{Color: col}.Add(gtx.Ops)
				paint.PaintOp{}.Add(gtx.Ops)
				return layout.Dimensions{Size: image.Pt(2*r+gtx.Dp(t.Sp.XS), 2*r)}
			}),
			layout.Rigid(Text(t, t.Sz.Caption, t.P.Ink, ClassLabel(class))),
		)
	}
}
