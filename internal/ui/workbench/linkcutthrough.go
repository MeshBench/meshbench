// The Link panel's picture: ground, sight line, Fresnel band, knife edges.
//
// Split from the panel itself, which is about the words around it and the
// shape the panel takes at the width it has been given.
package workbench

import (
	"fmt"
	"image"

	"gioui.org/f32"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"

	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/ui/comp"
	"github.com/MeshBench/meshbench/internal/ui/theme"
)

// edgeLabelW is the room "-28.9 dB" needs, and so how far apart two edges
// have to be before both can say what they cost.
const edgeLabelW = unit.Dp(58)

// exaggeration is how much taller the metres are drawn than the kilometres.
func exaggeration(sz image.Point, p *state.Profile) float64 {
	w, h := float64(sz.X), float64(sz.Y)
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
	// Clipped to its own box. The chart draws edge labels beyond its top and
	// a rule its full height, and in a rail those landed on the words above
	// and below it - which is what a panel that has run out of room looks
	// like when nothing bounds it.
	defer clip.Rect{Max: sz}.Push(gtx.Ops).Pop()
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
	// The masts: each end stands on its own ground, so an antenna's height
	// above it is visible rather than implied - wb1 drew these and the port
	// dropped them.
	var masts clip.Path
	masts.Begin(gtx.Ops)
	for _, sm := range []state.ProfileSample{p.Samples[0], last} {
		masts.MoveTo(f32.Pt(X(sm.DistM), Y(sm.BulgedM)))
		masts.LineTo(f32.Pt(X(sm.DistM), Y(sm.LOSm)))
	}
	paint.FillShape(gtx.Ops, t.P.Ink, clip.Stroke{Path: masts.End(), Width: 2}.Op())
	for _, sm := range []state.ProfileSample{p.Samples[0], last} {
		func() {
			r := gtx.Dp(3)
			cx, cy := int(X(sm.DistM)), int(Y(sm.LOSm))
			defer clip.Ellipse(imageRect4(cx-r, cy-r, cx+r, cy+r)).Push(gtx.Ops).Pop()
			paint.ColorOp{Color: t.P.Ink}.Add(gtx.Ops)
			paint.PaintOp{}.Add(gtx.Ops)
		}()
	}

	drawWorstAndEdges(t, gtx, p, X, Y, h)
	return layout.Dimensions{Size: sz}
}

// drawWorstAndEdges marks where the path is decided and what each obstruction
// costs: a full-height rule at the worst intrusion into the first Fresnel
// zone, amber while the path clears and red when it does not, and a line down
// to every knife edge with its loss at the top.
func drawWorstAndEdges(t *theme.Theme, gtx layout.Context, p *state.Profile,
	X func(float64) float32, Y func(float64) float32, h float32) {

	if p.Worst.DistM > 0 || p.Worst.FresnelPct != 0 {
		col := t.P.Warn
		if p.Worst.Blocked {
			col = t.P.Bad
		}
		x := X(p.Worst.DistM)
		var rule clip.Path
		rule.Begin(gtx.Ops)
		rule.MoveTo(f32.Pt(x, 0))
		rule.LineTo(f32.Pt(x, h))
		paint.FillShape(gtx.Ops, theme.Alpha(col, 0.55),
			clip.Stroke{Path: rule.End(), Width: 1}.Op())
		off := op.Offset(imagePtXY(int(x)+4, int(h)-gtx.Dp(16))).Push(gtx.Ops)
		comp.Mono(t, t.Sz.Caption, col,
			fmt.Sprintf("worst: %.0f%% F1", p.Worst.FresnelPct))(unbounded2(gtx))
		off.Pop()
	}
	// Labelled only where the last label has been left behind. Six edges in a
	// 340dp rail printed their costs on top of each other, which reads as a
	// rendering fault rather than as a crowded axis; the marks still stand at
	// every edge, and the costs are all listed under the chart in any case.
	labelled := float32(-1 << 20)
	for _, e := range p.Edges {
		x := X(e.DistM)
		var mark clip.Path
		mark.Begin(gtx.Ops)
		mark.MoveTo(f32.Pt(x, float32(gtx.Dp(14))))
		mark.LineTo(f32.Pt(x, groundYAt(p, e.DistM, Y)))
		paint.FillShape(gtx.Ops, theme.Alpha(t.P.Warn, 0.7),
			clip.Stroke{Path: mark.End(), Width: 1}.Op())
		if x-labelled < float32(gtx.Dp(edgeLabelW)) {
			continue
		}
		labelled = x
		off := op.Offset(imagePtXY(int(x)+4, 0)).Push(gtx.Ops)
		comp.Mono(t, t.Sz.Caption, t.P.Warn,
			fmt.Sprintf("-%.1f dB", e.LossDB))(unbounded2(gtx))
		off.Pop()
	}
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
