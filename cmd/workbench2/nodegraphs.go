package main

import (
	"fmt"
	"image"
	"image/color"

	"gioui.org/f32"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"

	"github.com/A13xB0/meshcoresim/internal/gui/comp"
	"github.com/A13xB0/meshcoresim/internal/gui/state"
	"github.com/A13xB0/meshcoresim/internal/gui/theme"
)

// Graphs for the selected node.
//
// Three, because the three questions about a node that a table answers badly
// are all "has it always been like that": is its memory climbing, is it working
// or idle, and is it still sending. A number in a cell says what is true now;
// these say whether now is unusual.

// nodeGraphs draws the selected node's history under the table.
func nodeGraphs(t *theme.Theme, gtx layout.Context, s *state.Snapshot) layout.Dimensions {
	ser := s.Series
	if ser.Name == "" || len(ser.CPU) < 2 {
		return comp.Text(t, t.Sz.Caption, t.P.Faint,
			"select a node to graph its memory, processor time and packets")(gtx)
	}
	h := gtx.Dp(52)
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(comp.SectionTitle(t, ser.Name)),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{}.Layout(gtx,
				graph(t, h, "memory", t.P.Accent, asFloats64(ser.RSS), func(v int64) string { return siBytes(v) }),
				graph(t, h, "cpu", t.P.Warn, ser.CPU, func(v int64) string {
					return fmt.Sprintf("%.1f%%", float64(v)/100)
				}),
				graph(t, h, "sent", t.P.Good, asFloatsInt(ser.Sent), func(v int64) string {
					return fmt.Sprintf("%d", v)
				}),
			)
		}),
	)
}

// graph is one sparkline with its peak labelled.
//
// The peak rather than the latest value, because the reason to look at a graph
// rather than the table beside it is to see what it did when you were not
// watching.
func graph(t *theme.Theme, h int, label string, col color.NRGBA, vs []float64,
	fmtPeak func(int64) string) layout.FlexChild {

	return layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
		w := gtx.Constraints.Max.X - gtx.Dp(t.Sp.S)
		if w < 20 || len(vs) < 2 {
			return layout.Dimensions{Size: image.Pt(gtx.Constraints.Max.X, h)}
		}
		hi := 0.0
		for _, v := range vs {
			if v > hi {
				hi = v
			}
		}
		size := image.Pt(w, h)
		comp.FillRect(gtx, size, theme.Alpha(t.P.Sunk, 0.9))

		if hi > 0 {
			// A filled area rather than a line: at 52 pixels tall a line is
			// mostly antialiasing, and the shape is what carries the meaning.
			var p clip.Path
			p.Begin(gtx.Ops)
			p.MoveTo(f32.Pt(0, float32(h)))
			for i, v := range vs {
				x := float32(i) / float32(len(vs)-1) * float32(w)
				y := float32(h) - float32(v/hi)*float32(h-2)
				p.LineTo(f32.Pt(x, y))
			}
			p.LineTo(f32.Pt(float32(w), float32(h)))
			p.Close()
			paint.FillShape(gtx.Ops, theme.Alpha(col, 0.45),
				clip.Outline{Path: p.End()}.Op())
		}

		off := op.Offset(image.Pt(gtx.Dp(4), gtx.Dp(2))).Push(gtx.Ops)
		comp.Mono(t, t.Sz.Caption, t.P.Dim, label+"  "+fmtPeak(int64(hi)))(unboundedCtx(gtx))
		off.Pop()
		return layout.Dimensions{Size: image.Pt(gtx.Constraints.Max.X, h)}
	})
}

// unboundedCtx lets a label size itself rather than filling the graph.
func unboundedCtx(gtx layout.Context) layout.Context {
	gtx.Constraints.Min = image.Point{}
	gtx.Constraints.Max = image.Pt(1<<14, 1<<14)
	return gtx
}

func asFloats64(v []int64) []float64 {
	out := make([]float64, len(v))
	for i := range v {
		out[i] = float64(v[i])
	}
	return out
}

func asFloatsInt(v []int) []float64 {
	out := make([]float64, len(v))
	for i := range v {
		out[i] = float64(v[i])
	}
	return out
}
