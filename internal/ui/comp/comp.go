// Package comp is the component library: the widgets every view is built from.
//
// Each one takes a *theme.Theme and reads every colour and measurement from
// it, so a density or palette change is a change to the theme and nothing
// else. None of these draw a literal colour.
package comp

import (
	"image"
	"image/color"
	"strings"

	"gioui.org/f32"
	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"github.com/MeshBench/meshbench/internal/ui/theme"
)

// Fill paints the whole of the current constraints.
func Fill(gtx layout.Context, c color.NRGBA) {
	defer clip.Rect{Max: gtx.Constraints.Max}.Push(gtx.Ops).Pop()
	paint.ColorOp{Color: c}.Add(gtx.Ops)
	paint.PaintOp{}.Add(gtx.Ops)
}

// FillRect paints a given size.
func FillRect(gtx layout.Context, sz image.Point, c color.NRGBA) layout.Dimensions {
	defer clip.Rect{Max: sz}.Push(gtx.Ops).Pop()
	paint.ColorOp{Color: c}.Add(gtx.Ops)
	paint.PaintOp{}.Add(gtx.Ops)
	return layout.Dimensions{Size: sz}
}

// RoundRect paints a rounded rectangle, for surfaces and buttons.
func RoundRect(gtx layout.Context, sz image.Point, r unit.Dp, c color.NRGBA) {
	rr := cornerRadius(gtx, sz, r)
	defer clip.RRect{Rect: image.Rectangle{Max: sz}, NE: rr, NW: rr, SE: rr, SW: rr}.
		Push(gtx.Ops).Pop()
	paint.ColorOp{Color: c}.Add(gtx.Ops)
	paint.PaintOp{}.Add(gtx.Ops)
}

// cornerRadius clamps a corner to what the box can actually hold.
//
// A radius larger than half the shorter side has no valid rounded rectangle
// to describe, and Gio does not reject it: the arcs overrun each other and
// the resulting path sprays stray strokes across the whole panel, nowhere
// near the widget that asked for them. Clamping here rather than at each call
// site because "round this completely" is a reasonable thing to ask for - a
// pill is a rectangle with a huge radius - and every caller getting it right
// individually is not something that survives.
func cornerRadius(gtx layout.Context, sz image.Point, r unit.Dp) int {
	rr := gtx.Dp(r)
	half := sz.X
	if sz.Y < half {
		half = sz.Y
	}
	half /= 2
	if rr > half {
		rr = half
	}
	if rr < 0 {
		rr = 0
	}
	return rr
}

// Border strokes a rounded outline without filling it.
func Border(gtx layout.Context, sz image.Point, r unit.Dp, w unit.Dp, c color.NRGBA) {
	rr := cornerRadius(gtx, sz, r)
	shape := clip.RRect{Rect: image.Rectangle{Max: sz}, NE: rr, NW: rr, SE: rr, SW: rr}
	paint.FillShape(gtx.Ops, c,
		clip.Stroke{Path: shape.Path(gtx.Ops), Width: float32(gtx.Dp(w))}.Op())
}

// Text is a label at a named role in the type scale.
func Text(t *theme.Theme, size unit.Sp, c color.NRGBA, s string) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		l := material.Label(t.M, size, s)
		l.Color = c
		return l.Layout(gtx)
	}
}

// OneLine is text that is cut off rather than wrapped.
//
// For anything in a fixed-width column. A wrapped cell is taller than the row
// it is in, so it draws over the row below and the table looks broken - which
// is what the firmware table did the first time it met a version name with a
// suffix on it.
func OneLine(t *theme.Theme, size unit.Sp, c color.NRGBA, s string, mono bool) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		l := material.Label(t.M, size, s)
		l.Color = c
		l.MaxLines = 1
		l.Truncator = "..."
		if mono {
			l.Font = t.Mono
		}
		return l.Layout(gtx)
	}
}

// Mono is text in the monospace face, for anything that lines up in a column
// or is copied into a config file.
func Mono(t *theme.Theme, size unit.Sp, c color.NRGBA, s string) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		l := material.Label(t.M, size, s)
		l.Color = c
		l.Font = t.Mono
		return l.Layout(gtx)
	}
}

// SectionTitle is the label above a group of controls.
//
// The UI face in sentence case, not monospace capitals. Uppercase mono reads
// as a machine label shouting a category; a panel title is a name.
func SectionTitle(t *theme.Theme, s string) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		l := material.Label(t.M, t.Sz.Section, s)
		l.Color = t.P.Dim
		l.Font.Weight = font.Medium
		return l.Layout(gtx)
	}
}

// SectionRule is a small caption with a rule running out from it, for dividing
// one pane into named parts.
//
// Distinct from SectionTitle, which names a whole panel: this one is a caption
// inside a pane, and the rule is what makes it a divider rather than a heading.
// Here rather than copied into each pane that wants one, because two copies of
// a divider drift in exactly the way nobody notices until they are side by side.
func SectionRule(t *theme.Theme, label string) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		return layout.Inset{Top: t.Sp.S, Bottom: t.Sp.XS}.Layout(gtx,
			func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(Text(t, t.Sz.Caption, t.P.Dim, strings.ToUpper(label))),
					layout.Rigid(layout.Spacer{Width: t.Sp.S}.Layout),
					layout.Flexed(1, HRule(t)),
				)
			})
	}
}

// Rule is a one-pixel divider, horizontal or vertical.
func HRule(t *theme.Theme) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		return FillRect(gtx, image.Pt(gtx.Constraints.Max.X, 1), t.P.Rule)
	}
}

func VRule(t *theme.Theme) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		return FillRect(gtx, image.Pt(1, gtx.Constraints.Max.Y), t.P.Rule)
	}
}

// Spacer eats the remaining space on an axis, for pushing things apart.
func Spacer(gtx layout.Context) layout.Dimensions {
	return layout.Dimensions{Size: image.Pt(gtx.Constraints.Max.X, 0)}
}

// Inset is spacing from the scale, never a literal.
func Inset(t *theme.Theme, d unit.Dp, w layout.Widget) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		return layout.UniformInset(d).Layout(gtx, w)
	}
}

// ButtonKind selects a button's weight. Primary is the one thing a panel most
// wants done; Quiet is for actions that should not compete.
type ButtonKind int

const (
	Primary ButtonKind = iota
	Secondary
	Quiet
	Destructive
)

// Button is a labelled button that reads its colours from the theme.
//
// Disabled carries a reason rather than being silently inert: a control that
// cannot be used and does not say why is how an operator concludes the
// application is broken.
type Button struct {
	Click    widget.Clickable
	Kind     ButtonKind
	Label    string
	Disabled bool
	Reason   string
}

// Layout draws the button, and beside it the reason it cannot be pressed. The
// reason is drawn every frame rather than on hover: a control that is dim and
// silent is the one an operator reports as broken, and a caption that appears
// only under the pointer is not there for the person scanning the page.
func (b *Button) Layout(t *theme.Theme, gtx layout.Context) layout.Dimensions {
	if b.Disabled && b.Reason != "" {
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(b.layoutButton(t)),
			layout.Rigid(layout.Spacer{Width: t.Sp.S}.Layout),
			layout.Rigid(Text(t, t.Sz.Caption, t.P.Faint, b.Reason)),
		)
	}
	return b.layoutButton(t)(gtx)
}

func (b *Button) layoutButton(t *theme.Theme) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions { return b.draw(t, gtx) }
}

func (b *Button) draw(t *theme.Theme, gtx layout.Context) layout.Dimensions {
	fg, bg, border := t.P.Ink, color.NRGBA{}, color.NRGBA{}
	switch b.Kind {
	case Primary:
		fg, bg = t.P.AccentInk, t.P.Accent
	case Secondary:
		fg, border = t.P.Ink, t.P.Rule
	case Quiet:
		fg = t.P.Dim
	case Destructive:
		fg, border = t.P.Bad, theme.Alpha(t.P.Bad, 0.5)
	}
	if b.Disabled {
		fg = theme.Alpha(fg, 0.4)
		bg = theme.Alpha(bg, 0.3)
		border = theme.Alpha(border, 0.4)
	}
	if b.Click.Hovered() && !b.Disabled {
		switch b.Kind {
		case Primary:
			bg = theme.Alpha(bg, 0.85)
		default:
			bg = theme.Alpha(t.P.Ink, 0.07)
		}
	}

	pad := layout.Inset{
		Top: t.Sp.S, Bottom: t.Sp.S, Left: t.Sp.M, Right: t.Sp.M,
	}
	// Disabled outside the Clickable, not inside it. Inside, the clickable
	// still registered its own pointer area against the live context and
	// pressed perfectly while drawn faded - a Remove button that refused in
	// appearance only.
	if b.Disabled {
		gtx = gtx.Disabled()
	}
	return b.Click.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		macro := op.Record(gtx.Ops)
		dims := pad.Layout(gtx, Text(t, t.Sz.Body, fg, b.Label))
		call := macro.Stop()
		if bg.A > 0 {
			RoundRect(gtx, dims.Size, 6, bg)
		}
		if border.A > 0 {
			Border(gtx, dims.Size, 6, 1, border)
		}
		call.Add(gtx.Ops)
		return dims
	})
}

// Field is a text input with an optional unit suffix and a validation state.
type Field struct {
	Editor widget.Editor
	Label  string
	Hint   string
	Suffix string
	Error  string
}

// Layout draws the field around its own editor.
func (f *Field) Layout(t *theme.Theme, gtx layout.Context) layout.Dimensions {
	return f.LayoutEditor(t, gtx, &f.Editor)
}

// LayoutEditor draws the same chrome around an editor somebody else owns.
//
// Take the editor by pointer, always. Gio identifies a widget by its address,
// so an editor copied into a fresh struct each frame is a different widget
// every frame: the click focuses one and the keystrokes arrive at another. The
// node filter drew perfectly and accepted nothing.
func (f *Field) LayoutEditor(t *theme.Theme, gtx layout.Context, ed *widget.Editor) layout.Dimensions {
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if f.Label == "" {
				return layout.Dimensions{}
			}
			return layout.Inset{Bottom: t.Sp.XS}.Layout(gtx,
				Text(t, t.Sz.Label, t.P.Dim, f.Label))
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			line := t.P.Rule
			if f.Error != "" {
				line = t.P.Bad
			} else if gtx.Focused(ed) {
				line = t.P.Accent
			}
			macro := op.Record(gtx.Ops)
			dims := layout.Inset{
				Top: t.Sp.S, Bottom: t.Sp.S, Left: t.Sp.S, Right: t.Sp.S,
			}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						e := material.Editor(t.M, ed, f.Hint)
						e.TextSize = t.Sz.Body
						e.Color = t.P.Ink
						e.HintColor = t.P.Faint
						return e.Layout(gtx)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						if f.Suffix == "" {
							return layout.Dimensions{}
						}
						return Mono(t, t.Sz.Caption, t.P.Faint, f.Suffix)(gtx)
					}),
				)
			})
			call := macro.Stop()
			RoundRect(gtx, dims.Size, 5, t.P.Sunk)
			Border(gtx, dims.Size, 5, 1, line)
			call.Add(gtx.Ops)
			return dims
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if f.Error == "" {
				return layout.Dimensions{}
			}
			return layout.Inset{Top: t.Sp.XS}.Layout(gtx,
				Text(t, t.Sz.Caption, t.P.Bad, f.Error))
		}),
	)
}

// Check is a labelled checkbox whose label is part of the hit target.
type Check struct {
	Bool  widget.Bool
	Label string
}

// Layout draws the box and its tick.
//
// Drawn rather than material.CheckBox, which renders a filled square with the
// tick knocked out of it - so the tick is a hole showing whatever the box
// happens to sit on, and on a panel that is panel-coloured. A tick is a mark
// somebody made, not a gap, and it has to be the same ink as every other label
// on a filled accent shape.
//
// The same widget.Bool underneath, so everything that finds and presses
// checkboxes - the control audit included - finds this too.
func (c *Check) Layout(t *theme.Theme, gtx layout.Context) layout.Dimensions {
	return c.Bool.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		// Hugging the content, not the column it sits in: a flex given a
		// minimum fills it, and the clickable this returns to is sized from
		// what it reports - so a box that claimed the full width took the
		// presses meant for whatever was beside it. The node filter stopped
		// accepting typing that way.
		gtx.Constraints.Min = image.Point{}
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return c.box(t, gtx)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if c.Label == "" {
					return layout.Dimensions{}
				}
				return layout.Inset{Left: t.Sp.S}.Layout(gtx,
					Text(t, t.Sz.Body, t.P.Ink, c.Label))
			}),
		)
	})
}

// box is the square: filled in the accent when it is on, outlined when it is
// off, with the tick stroked across it in the accent's own ink.
func (c *Check) box(t *theme.Theme, gtx layout.Context) layout.Dimensions {
	// The square is drawn at 20dp inside a 34dp cell, which is the footprint
	// material.CheckBox occupied - its 26dp icon in the 4/3 cell it keeps for
	// the hover halo. Matched deliberately: this is a drop-in, and a control
	// that changed height would reflow every panel that has one, which is how
	// a fixed-coordinate test found the node filter had moved.
	cell := gtx.Dp(26) * 4 / 3
	side := gtx.Dp(20)
	off := (cell - side) / 2
	defer op.Offset(image.Pt(off, off)).Push(gtx.Ops).Pop()
	sz := image.Pt(side, side)
	rr := gtx.Dp(3)
	shape := clip.RRect{Rect: image.Rectangle{Max: sz}, NE: rr, NW: rr, SE: rr, SW: rr}

	edge := t.P.Rule
	if c.Bool.Hovered() {
		edge = t.P.Accent
	}
	if c.Bool.Value {
		fill := t.P.Accent
		if c.Bool.Hovered() {
			fill = theme.Alpha(fill, 0.85)
		}
		func() {
			defer shape.Push(gtx.Ops).Pop()
			paint.ColorOp{Color: fill}.Add(gtx.Ops)
			paint.PaintOp{}.Add(gtx.Ops)
		}()
	} else {
		paint.FillShape(gtx.Ops, edge, clip.Stroke{
			Path:  shape.Path(gtx.Ops),
			Width: float32(gtx.Dp(1.5)),
		}.Op())
	}

	if c.Bool.Value {
		// Two strokes, proportioned off the side so the tick keeps its shape
		// at any density: down to the low point at two fifths across, then up
		// and out to the top right.
		f := func(n float32) float32 { return float32(side) * n }
		var p clip.Path
		p.Begin(gtx.Ops)
		p.MoveTo(f32.Pt(f(0.24), f(0.52)))
		p.LineTo(f32.Pt(f(0.42), f(0.71)))
		p.LineTo(f32.Pt(f(0.77), f(0.31)))
		paint.FillShape(gtx.Ops, t.P.AccentInk, clip.Stroke{
			Path:  p.End(),
			Width: float32(gtx.Dp(2)),
		}.Op())
	}
	return layout.Dimensions{Size: image.Pt(cell, cell)}
}
