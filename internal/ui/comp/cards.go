// Cards, stat cells and the controls the Configuration page is built from.
//
// A settings page reads as a grid of labelled values with the reason each one
// matters underneath, not as a table. These are the primitives that shape
// needs, kept in comp so the next page that wants a card does not invent its
// own corner radius.
package comp

import (
	"image"
	"image/color"

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

// Card draws a titled surface around its content: rounded, bordered, padded.
func Card(t *theme.Theme, title string, content layout.Widget) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		macro := op.Record(gtx.Ops)
		dims := layout.UniformInset(t.Sp.M).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					if title == "" {
						return layout.Dimensions{}
					}
					l := material.Label(t.M, t.Sz.Section, title)
					l.Color = t.P.Ink
					l.Font.Weight = font.Medium
					return layout.Inset{Bottom: t.Sp.S}.Layout(gtx, l.Layout)
				}),
				layout.Rigid(content),
			)
		})
		call := macro.Stop()
		RoundRect(gtx, dims.Size, 8, t.P.Panel)
		Border(gtx, dims.Size, 8, 1, t.P.Rule)
		call.Add(gtx.Ops)
		return dims
	}
}

// StatCell is one labelled value with the reason it matters underneath.
//
// The caption is not decoration: it is the "why it matters" line the old
// table carried, kept because a number without its reason is a number the
// reader has to already understand.
func StatCell(t *theme.Theme, label, value, caption string) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(Text(t, t.Sz.Label, t.P.Dim, label)),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				l := material.Label(t.M, t.Sz.Title, value)
				l.Color = t.P.Ink
				l.Font = t.Mono
				l.MaxLines = 1
				l.Truncator = "..."
				return layout.Inset{Top: t.Sp.XXS, Bottom: t.Sp.XXS}.Layout(gtx, l.Layout)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if caption == "" {
					return layout.Dimensions{}
				}
				l := material.Label(t.M, t.Sz.Caption, caption)
				l.Color = t.P.Faint
				return l.Layout(gtx)
			}),
		)
	}
}

// CellGrid lays cells out in as many equal columns as the width holds.
//
// A wrap grid rather than a table: cells are self-contained, so making the
// window narrower folds them downward instead of clipping them.
func CellGrid(t *theme.Theme, gtx layout.Context, min unit.Dp, cells []layout.Widget) layout.Dimensions {
	cols := gtx.Constraints.Max.X / gtx.Dp(min)
	if cols < 1 {
		cols = 1
	}
	if cols > len(cells) {
		cols = len(cells)
	}
	var rows []layout.FlexChild
	for start := 0; start < len(cells); start += cols {
		end := start + cols
		if end > len(cells) {
			end = len(cells)
		}
		row := cells[start:end]
		rows = append(rows, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			var cols2 []layout.FlexChild
			for _, c := range row {
				c := c
				cols2 = append(cols2, layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{
						Right: t.Sp.M, Top: t.Sp.S, Bottom: t.Sp.S,
					}.Layout(gtx, c)
				}))
			}
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Start}.Layout(gtx, cols2...)
		}))
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx, rows...)
}

// Pill is a small status capsule: a coloured dot and a word, tinted with the
// state it reports.
func Pill(t *theme.Theme, c color.NRGBA, label string) layout.Widget {
	return pillWith(t, c, label, func(gtx layout.Context) layout.Dimensions {
		r := gtx.Dp(3)
		defer clip.Ellipse{Max: image.Pt(2*r, 2*r)}.Push(gtx.Ops).Pop()
		paint.ColorOp{Color: c}.Add(gtx.Ops)
		paint.PaintOp{}.Add(gtx.Ops)
		return layout.Dimensions{Size: image.Pt(2*r+gtx.Dp(t.Sp.XS), 2*r)}
	})
}

// pillWith is the capsule itself, whatever leads it.
func pillWith(t *theme.Theme, c color.NRGBA, label string, mark layout.Widget) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		macro := op.Record(gtx.Ops)
		dims := layout.Inset{
			Top: t.Sp.XS, Bottom: t.Sp.XS, Left: t.Sp.M, Right: t.Sp.M,
		}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(mark),
				layout.Rigid(Text(t, t.Sz.Label, c, label)),
			)
		})
		call := macro.Stop()
		// A capsule, not RoundRect with a huge radius: Gio does not clamp
		// corner radii, and a radius bigger than the rectangle renders as
		// smears of the fill colour across the window.
		rr := dims.Size.Y / 2
		func() {
			defer clip.RRect{Rect: image.Rectangle{Max: dims.Size},
				NE: rr, NW: rr, SE: rr, SW: rr}.Push(gtx.Ops).Pop()
			paint.ColorOp{Color: theme.Alpha(c, 0.13)}.Add(gtx.Ops)
			paint.PaintOp{}.Add(gtx.Ops)
		}()
		call.Add(gtx.Ops)
		return dims
	}
}

// Arrow is which way a drawn arrowhead points.
type Arrow int

const (
	ArrowUp Arrow = iota
	ArrowDown
	ArrowLeft
	ArrowRight
)

// ArrowPill is a Pill whose leading mark is an arrowhead rather than a dot.
//
// Drawn rather than typed. The arrows in the character set are not in every
// font: on this machine two of the four came back as coloured boxes, which is
// the same failure the transport's symbols and the chevron were drawn to
// avoid. A direction that renders as a box is worse than no direction at all.
func ArrowPill(t *theme.Theme, c color.NRGBA, dir Arrow, label string) layout.Widget {
	return pillWith(t, c, label, func(gtx layout.Context) layout.Dimensions {
		return arrowHead(t, gtx, c, dir)
	})
}

// arrowHead is one small filled triangle.
func arrowHead(t *theme.Theme, gtx layout.Context, c color.NRGBA, dir Arrow) layout.Dimensions {
	d := gtx.Dp(7)
	f := float32(d)
	var a, b, tip f32.Point
	switch dir {
	case ArrowUp:
		a, b, tip = f32.Pt(0, f), f32.Pt(f, f), f32.Pt(f/2, 0)
	case ArrowDown:
		a, b, tip = f32.Pt(0, 0), f32.Pt(f, 0), f32.Pt(f/2, f)
	case ArrowLeft:
		a, b, tip = f32.Pt(f, 0), f32.Pt(f, f), f32.Pt(0, f/2)
	default:
		a, b, tip = f32.Pt(0, 0), f32.Pt(0, f), f32.Pt(f, f/2)
	}
	var p clip.Path
	p.Begin(gtx.Ops)
	p.MoveTo(a)
	p.LineTo(b)
	p.LineTo(tip)
	p.Close()
	paint.FillShape(gtx.Ops, c, clip.Outline{Path: p.End()}.Op())
	return layout.Dimensions{Size: image.Pt(d+gtx.Dp(t.Sp.XS), d)}
}

// Switch is a Check drawn as a pill toggle.
//
// The same widget.Bool underneath, so everything that finds and presses
// checkboxes - the control audit included - finds this too; only the drawing
// differs.
func (c *Check) LayoutSwitch(t *theme.Theme, gtx layout.Context) layout.Dimensions {
	return c.Bool.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		trackW, trackH := gtx.Dp(34), gtx.Dp(18)
		knob := trackH - gtx.Dp(4)
		return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				track := t.P.Rule
				if c.Bool.Value {
					track = t.P.Accent
				}
				if c.Bool.Hovered() {
					track = theme.Alpha(track, 0.85)
				}
				sz := image.Pt(trackW, trackH)
				rr := trackH / 2
				func() {
					defer clip.RRect{Rect: image.Rectangle{Max: sz},
						NE: rr, NW: rr, SE: rr, SW: rr}.Push(gtx.Ops).Pop()
					paint.ColorOp{Color: track}.Add(gtx.Ops)
					paint.PaintOp{}.Add(gtx.Ops)
				}()
				x := gtx.Dp(2)
				if c.Bool.Value {
					x = trackW - knob - gtx.Dp(2)
				}
				func() {
					defer op.Offset(image.Pt(x, (trackH-knob)/2)).Push(gtx.Ops).Pop()
					defer clip.Ellipse{Max: image.Pt(knob, knob)}.Push(gtx.Ops).Pop()
					knobC := t.P.Ink
					if c.Bool.Value {
						knobC = t.P.AccentInk
					}
					paint.ColorOp{Color: knobC}.Add(gtx.Ops)
					paint.PaintOp{}.Add(gtx.Ops)
				}()
				return layout.Dimensions{Size: sz}
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

// Dropdown shows the current choice and asks for a new one when pressed.
//
// The first real dropdown in the application, and deliberately not a menu of
// its own: pressing it hands the choosing to whatever overlay the caller
// already has - the shell's chooser - so there is exactly one way anything
// here picks from a list.
type Dropdown struct {
	Btn Button
	// Label is what the control is called; Value is the current choice.
	Label string
	Value string
	// OnOpen is called when the control is pressed. The caller opens its
	// chooser and eventually writes Value back.
	OnOpen func()
}

func (d *Dropdown) Layout(t *theme.Theme, gtx layout.Context) layout.Dimensions {
	if d.Btn.Click.Clicked(gtx) && d.OnOpen != nil {
		d.OnOpen()
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if d.Label == "" {
				return layout.Dimensions{}
			}
			return layout.Inset{Bottom: t.Sp.XS}.Layout(gtx,
				Text(t, t.Sz.Label, t.P.Dim, d.Label))
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return d.Btn.Click.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				line := t.P.Rule
				if d.Btn.Click.Hovered() {
					line = t.P.Accent
				}
				macro := op.Record(gtx.Ops)
				dims := layout.Inset{
					Top: t.Sp.S, Bottom: t.Sp.S, Left: t.Sp.S, Right: t.Sp.S,
				}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Horizontal, Alignment: layout.Middle}.Layout(gtx,
						layout.Flexed(1, OneLine(t, t.Sz.Body, t.P.Ink, d.Value, false)),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return chevronDown(t, gtx)
						}),
					)
				})
				call := macro.Stop()
				RoundRect(gtx, dims.Size, 5, t.P.Sunk)
				Border(gtx, dims.Size, 5, 1, line)
				call.Add(gtx.Ops)
				return dims
			})
		}),
	)
}

// chevronDown is a small drawn triangle, the transport's approach: a glyph is
// drawn, never borrowed from an icon font that may not be installed.
func chevronDown(t *theme.Theme, gtx layout.Context) layout.Dimensions {
	w, h := gtx.Dp(8), gtx.Dp(5)
	var p clip.Path
	p.Begin(gtx.Ops)
	p.MoveTo(f32.Pt(0, 0))
	p.LineTo(f32.Pt(float32(w), 0))
	p.LineTo(f32.Pt(float32(w)/2, float32(h)))
	p.Close()
	paint.FillShape(gtx.Ops, t.P.Dim, clip.Outline{Path: p.End()}.Op())
	return layout.Dimensions{Size: image.Pt(w, h)}
}

var _ = widget.Bool{}
