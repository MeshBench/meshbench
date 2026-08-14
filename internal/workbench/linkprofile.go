// The Link panel: wb1's cut-through, drawn.
//
// The ground with the earth's curvature in it, the sight line, the first
// Fresnel zone as a band, and the knife edges labelled with what each costs -
// the picture that turns "the link is bad" into "that ridge at 4.2 km costs
// you 31 dB".
package workbench

import (
	"fmt"
	"image"
	"strings"

	"gioui.org/f32"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"

	"github.com/MeshBench/meshbench/internal/gui/comp"
	"github.com/MeshBench/meshbench/internal/gui/state"
	"github.com/MeshBench/meshbench/internal/gui/theme"
)

type linkPanel struct{}

func (linkPanel) Draw(t *theme.Theme, gtx layout.Context, s *state.Snapshot) layout.Dimensions {
	if s == nil || s.LinkProfile == nil || len(s.LinkProfile.Samples) < 2 {
		return layout.Center.Layout(gtx, comp.Text(t, t.Sz.Body, t.P.Dim,
			"select a pair - or one node, for its strongest link - and the "+
				"ground between them is cut through here"))
	}
	p := s.LinkProfile
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(comp.SectionTitle(t, p.From+"  <->  "+p.To)),
				layout.Rigid(layout.Spacer{Width: t.Sp.M}.Layout),
				layout.Rigid(comp.Mono(t, t.Sz.Caption, t.P.Dim,
					fmt.Sprintf("%.1f km", p.DistanceKm))),
				layout.Flexed(1, comp.Spacer),
				layout.Rigid(marginText(t, p.From, p.To, p.AtoB)),
				layout.Rigid(layout.Spacer{Width: t.Sp.M}.Layout),
				layout.Rigid(marginText(t, p.To, p.From, p.BtoA)),
			)
		}),
		layout.Rigid(layout.Spacer{Height: t.Sp.XS}.Layout),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return drawCutThrough(t, gtx, p)
		}),
		layout.Rigid(layout.Spacer{Height: t.Sp.XS}.Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			exag := exaggeration(gtx, p)
			return comp.OneLine(t, t.Sz.Caption, t.P.Faint, fmt.Sprintf(
				"terrain includes earth curvature; the band is the first Fresnel "+
					"zone   |   %.1f km, %.0f-%.0f m   |   vertical exaggeration x%.1f",
				p.DistanceKm, p.LowM, p.HighM, exag), false)(gtx)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if len(p.Edges) == 0 {
				return comp.Text(t, t.Sz.Caption, t.P.Faint,
					"no obstruction costs more than a decibel")(gtx)
			}
			parts := make([]string, 0, len(p.Edges))
			for _, e := range p.Edges {
				parts = append(parts, fmt.Sprintf("%.1f km -%.1f dB", e.DistM/1000, e.LossDB))
			}
			return comp.OneLine(t, t.Sz.Caption, t.P.Warn,
				"edges:  "+strings.Join(parts, "   "), false)(gtx)
		}),
		layout.Rigid(comp.Text(t, t.Sz.Caption, t.P.Dim, p.Verdict)),
	)
}

func marginText(t *theme.Theme, from, to string, db float64) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical, Alignment: layout.End}.Layout(gtx,
			layout.Rigid(comp.Text(t, t.Sz.Caption, t.P.Faint, from+" -> "+to)),
			layout.Rigid(comp.Mono(t, t.Sz.Caption, verdictColour(t, db),
				fmt.Sprintf("%+.1f dB", db))),
		)
	}
}

// exaggeration is how much taller the metres are drawn than the kilometres.
func exaggeration(gtx layout.Context, p *state.Profile) float64 {
	w, h := float64(gtx.Constraints.Max.X), float64(gtx.Constraints.Max.Y)
	if w <= 0 || h <= 0 || p.HighM <= p.LowM {
		return 1
	}
	mPerPxX := p.DistanceKm * 1000 / w
	mPerPxY := (p.HighM - p.LowM) / h
	if mPerPxY <= 0 {
		return 1
	}
	return mPerPxX / mPerPxY
}

// drawCutThrough paints the chart into whatever space the panel offers.
func drawCutThrough(t *theme.Theme, gtx layout.Context, p *state.Profile) layout.Dimensions {
	sz := gtx.Constraints.Max
	w, h := float32(sz.X), float32(sz.Y)
	maxD := float32(p.Samples[len(p.Samples)-1].DistM)
	lo, hi := float32(p.LowM), float32(p.HighM)
	if maxD <= 0 || hi <= lo {
		return layout.Dimensions{Size: sz}
	}
	X := func(d float64) float32 { return float32(d) / maxD * w }
	Y := func(m float64) float32 { return h - (float32(m)-lo)/(hi-lo)*h }

	// The ground, filled to the floor and stroked along its top.
	var ground clip.Path
	ground.Begin(gtx.Ops)
	ground.MoveTo(f32.Pt(0, h))
	for _, sm := range p.Samples {
		ground.LineTo(f32.Pt(X(sm.DistM), Y(sm.BulgedM)))
	}
	ground.LineTo(f32.Pt(w, h))
	ground.Close()
	paint.FillShape(gtx.Ops, theme.Alpha(t.P.Good, 0.25),
		clip.Outline{Path: ground.End()}.Op())
	var crest clip.Path
	crest.Begin(gtx.Ops)
	crest.MoveTo(f32.Pt(0, Y(p.Samples[0].BulgedM)))
	for _, sm := range p.Samples[1:] {
		crest.LineTo(f32.Pt(X(sm.DistM), Y(sm.BulgedM)))
	}
	paint.FillShape(gtx.Ops, theme.Alpha(t.P.Good, 0.8),
		clip.Stroke{Path: crest.End(), Width: 1.5}.Op())

	// The first Fresnel zone, as the band the radio actually needs clear.
	var band clip.Path
	band.Begin(gtx.Ops)
	band.MoveTo(f32.Pt(X(p.Samples[0].DistM), Y(p.Samples[0].LOSm)))
	for _, sm := range p.Samples[1:] {
		band.LineTo(f32.Pt(X(sm.DistM), Y(sm.LOSm-sm.FresnelM)))
	}
	for i := len(p.Samples) - 1; i >= 0; i-- {
		sm := p.Samples[i]
		band.LineTo(f32.Pt(X(sm.DistM), Y(sm.LOSm+sm.FresnelM)))
	}
	band.Close()
	paint.FillShape(gtx.Ops, theme.Alpha(t.P.Accent, 0.15),
		clip.Outline{Path: band.End()}.Op())

	// The sight line, endpoint to endpoint.
	var los clip.Path
	los.Begin(gtx.Ops)
	los.MoveTo(f32.Pt(X(p.Samples[0].DistM), Y(p.Samples[0].LOSm)))
	last := p.Samples[len(p.Samples)-1]
	los.LineTo(f32.Pt(X(last.DistM), Y(last.LOSm)))
	paint.FillShape(gtx.Ops, t.P.Ink, clip.Stroke{Path: los.End(), Width: 1.5}.Op())
	for _, sm := range []state.ProfileSample{p.Samples[0], last} {
		func() {
			r := gtx.Dp(3)
			cx, cy := int(X(sm.DistM)), int(Y(sm.LOSm))
			defer clip.Ellipse(imageRect4(cx-r, cy-r, cx+r, cy+r)).Push(gtx.Ops).Pop()
			paint.ColorOp{Color: t.P.Ink}.Add(gtx.Ops)
			paint.PaintOp{}.Add(gtx.Ops)
		}()
	}

	// The edges: a line from the top to the obstruction, its cost at the top.
	for _, e := range p.Edges {
		x := X(e.DistM)
		var mark clip.Path
		mark.Begin(gtx.Ops)
		mark.MoveTo(f32.Pt(x, float32(gtx.Dp(14))))
		mark.LineTo(f32.Pt(x, groundYAt(p, e.DistM, Y)))
		paint.FillShape(gtx.Ops, theme.Alpha(t.P.Warn, 0.7),
			clip.Stroke{Path: mark.End(), Width: 1}.Op())
		off := op.Offset(imagePtXY(int(x)+4, 0)).Push(gtx.Ops)
		comp.Mono(t, t.Sz.Caption, t.P.Warn,
			fmt.Sprintf("-%.1f dB", e.LossDB))(unbounded2(gtx))
		off.Pop()
	}
	return layout.Dimensions{Size: sz}
}

// groundYAt finds the drawn ground at a distance, for the edge markers.
func groundYAt(p *state.Profile, distM float64, Y func(float64) float32) float32 {
	best, bd := p.Samples[0], distM
	for _, sm := range p.Samples {
		if d := absF(sm.DistM - distM); d < bd {
			best, bd = sm, d
		}
	}
	return Y(best.BulgedM)
}

func absF(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}

func imageRect4(x0, y0, x1, y1 int) image.Rectangle {
	return image.Rect(x0, y0, x1, y1)
}

// unbounded2 lets a label size itself.
func unbounded2(gtx layout.Context) layout.Context {
	gtx.Constraints.Min = imagePtXY(0, 0)
	gtx.Constraints.Max = imagePtXY(1<<14, 1<<14)
	return gtx
}
