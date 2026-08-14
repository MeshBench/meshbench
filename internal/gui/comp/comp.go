// Package comp is the component library: the widgets every view is built from.
//
// Each one takes a *theme.Theme and reads every colour and measurement from
// it, so a density or palette change is a change to the theme and nothing
// else. None of these draw a literal colour.
package comp

import (
	"image"
	"image/color"

	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"github.com/MeshBench/meshbench/internal/gui/theme"
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
	rr := gtx.Dp(r)
	defer clip.RRect{Rect: image.Rectangle{Max: sz}, NE: rr, NW: rr, SE: rr, SW: rr}.
		Push(gtx.Ops).Pop()
	paint.ColorOp{Color: c}.Add(gtx.Ops)
	paint.PaintOp{}.Add(gtx.Ops)
}

// Border strokes a rounded outline without filling it.
func Border(gtx layout.Context, sz image.Point, r unit.Dp, w unit.Dp, c color.NRGBA) {
	rr := gtx.Dp(r)
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

func (b *Button) Layout(t *theme.Theme, gtx layout.Context) layout.Dimensions {
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
	return b.Click.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		if b.Disabled {
			gtx = gtx.Disabled()
		}
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

func (c *Check) Layout(t *theme.Theme, gtx layout.Context) layout.Dimensions {
	cb := material.CheckBox(t.M, &c.Bool, c.Label)
	cb.Color = t.P.Ink
	cb.IconColor = t.P.Accent
	cb.TextSize = t.Sz.Body
	return cb.Layout(gtx)
}
