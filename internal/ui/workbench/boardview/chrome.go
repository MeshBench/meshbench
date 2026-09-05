// The window's chrome: the bar across the top, the counts along the bottom, and
// the small drawing pieces the three regions share.
//
// Split from panel.go because that file is about what the window is and this
// one is about what it looks like; the left column is in rail.go and the middle
// in table.go, which is where they got too long to sit together.
package boardview

import (
	"fmt"
	"image"
	"image/color"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"

	"github.com/MeshBench/meshbench/internal/app/state"
	hw "github.com/MeshBench/meshbench/internal/firmware/board"
	"github.com/MeshBench/meshbench/internal/ui/comp"
	"github.com/MeshBench/meshbench/internal/ui/theme"
)

// header names the node and the board and says what it is doing.
//
// "Board view", beside the node view, the packet view and the map view, which
// is what this tree calls a window given over to one thing.
//
// Not "board check": that is taken - it is the capability probe, said that way
// in compatibility.md and shortcomings.md and measured one boot at a time - and
// a check is less than this is anyway. The window draws the panel live at any
// whole scale, takes taps and keys on it, drives everything the board has
// wired, and says why each line matters in the profile's own words.
func (p *Panel) header(t *theme.Theme, gtx layout.Context, b hw.Board,
	st *state.NodeStat) layout.Dimensions {

	backend := "not running"
	if st != nil && st.Backend != "" {
		backend = st.Backend
	}
	state := t.P.Faint
	word := "stopped"
	if st != nil && st.Running {
		state, word = t.P.Good, "running"
	}
	return onPanel(t, gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.UniformInset(t.Sp.S).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(comp.Text(t, t.Sz.Body, t.P.Ink, "Board view · "+p.Node)),
				layout.Rigid(layout.Spacer{Width: t.Sp.M}.Layout),
				layout.Rigid(comp.Mono(t, t.Sz.Caption, t.P.Dim,
					fmt.Sprintf("%s · %s · %s · %s", b.MCU, b.Vendor, b.Radio, backend))),
				layout.Flexed(1, spacer),
				layout.Rigid(comp.Pill(t, state, word)),
			)
		})
	})
}

// status is the two counts worth acting on, and nothing else.
func (p *Panel) status(t *theme.Theme, gtx layout.Context, st *state.NodeStat) layout.Dimensions {
	return onPanel(t, gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Inset{Left: t.Sp.S, Right: t.Sp.S, Top: t.Sp.XS,
			Bottom: t.Sp.XS}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			kids := []layout.FlexChild{
				layout.Rigid(comp.Mono(t, t.Sz.Caption, t.P.Faint,
					fmt.Sprintf("%d checked", p.counts.Total))),
			}
			if p.counts.Diverged > 0 {
				kids = append(kids, layout.Rigid(layout.Spacer{Width: t.Sp.M}.Layout),
					layout.Rigid(comp.Mono(t, t.Sz.Caption, t.P.Bad,
						fmt.Sprintf("%d diverged", p.counts.Diverged))))
			}
			if p.counts.Silent > 0 {
				kids = append(kids, layout.Rigid(layout.Spacer{Width: t.Sp.M}.Layout),
					layout.Rigid(comp.Mono(t, t.Sz.Caption, t.P.Warn,
						fmt.Sprintf("%d silent", p.counts.Silent))))
			}
			if p.counts.NotModelled > 0 {
				kids = append(kids, layout.Rigid(layout.Spacer{Width: t.Sp.M}.Layout),
					layout.Rigid(comp.Mono(t, t.Sz.Caption, t.P.Faint,
						fmt.Sprintf("%d not modelled", p.counts.NotModelled))))
			}
			kids = append(kids, layout.Flexed(1, spacer))
			if st != nil && st.Radio.Reported {
				kids = append(kids, layout.Rigid(comp.Mono(t, t.Sz.Caption, t.P.Good,
					modeWord(st.Radio.Mode))))
			}
			return layout.Flex{Alignment: layout.Middle}.Layout(gtx, kids...)
		})
	})
}

func modeWord(m uint8) string {
	switch m {
	case 1:
		return "rx"
	case 2:
		return "tx"
	case 3:
		return "cad"
	}
	return "standby"
}

// ---- small shared pieces ----

func spacer(gtx layout.Context) layout.Dimensions {
	return layout.Dimensions{Size: image.Pt(gtx.Constraints.Max.X, 0)}
}

// onPanel paints a surface behind a region, under whatever it draws.
func onPanel(t *theme.Theme, gtx layout.Context, w layout.Widget) layout.Dimensions {
	macro := op.Record(gtx.Ops)
	dims := w(gtx)
	call := macro.Stop()
	wide := dims.Size.X
	if gtx.Constraints.Min.X > wide {
		wide = gtx.Constraints.Min.X
	}
	comp.FillRect(gtx, image.Pt(wide, dims.Size.Y), t.P.Panel)
	call.Add(gtx.Ops)
	return dims
}

func vRule(t *theme.Theme) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		sz := image.Pt(gtx.Dp(unit.Dp(1)), gtx.Constraints.Max.Y)
		paint.FillShape(gtx.Ops, t.P.Rule, clip.Rect{Max: sz}.Op())
		return layout.Dimensions{Size: sz}
	}
}

func hRule(t *theme.Theme) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		sz := image.Pt(gtx.Constraints.Max.X, gtx.Dp(unit.Dp(1)))
		paint.FillShape(gtx.Ops, t.P.Rule, clip.Rect{Max: sz}.Op())
		return layout.Dimensions{Size: sz}
	}
}

func dot(gtx layout.Context, c color.NRGBA) layout.Dimensions {
	d := gtx.Dp(unit.Dp(6))
	r := d / 2
	paint.FillShape(gtx.Ops, c, clip.RRect{
		Rect: image.Rectangle{Max: image.Pt(d, d)}, NE: r, NW: r, SE: r, SW: r,
	}.Op(gtx.Ops))
	return layout.Dimensions{Size: image.Pt(d, d)}
}

// orDash is what a cell says when there is nothing to say, which is never a
// zero and never an empty cell somebody reads as a value.
func orDash(s string) string {
	if s == "" {
		return "—"
	}
	return s
}

func upper(s string) string {
	out := []rune(s)
	for i, r := range out {
		if r >= 'a' && r <= 'z' {
			out[i] = r - 32
		}
	}
	return string(out)
}

var _ = hw.PinNone
