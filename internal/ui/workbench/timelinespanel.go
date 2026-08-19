// The Timelines panel: one lane per node of a chosen metric over the run.
//
// Distinct from the packet timeline - that one is events, this one is counters.
// When a run has flood shapes to show it draws those instead: small multiples,
// one histogram per arm.
package workbench

import (
	"fmt"
	"image"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"

	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/ui/comp"
	"github.com/MeshBench/meshbench/internal/ui/theme"
)

func armsHaveShape(arms []state.ArmSummary) bool {
	for _, a := range arms {
		if len(a.PerSecond) > 0 {
			return true
		}
	}
	return false
}

// floodShapes draws one histogram per arm: receptions in each second after the
// burst, summed over that arm's seeds.
//
// Small multiples rather than an overlay. Four floods drawn on one axis make a
// solid block; side by side the eye does the work, which is the whole reason to
// plot a shape instead of quoting a total.
//
// Every panel shares one scale, taken from the loudest second in any arm. A
// per-panel scale would draw the arm that delivered least exactly like the one
// that delivered most.
func floodShapes(t *theme.Theme, gtx layout.Context, arms []state.ArmSummary) layout.Dimensions {
	peak, secs := 1, 0
	for _, a := range arms {
		if len(a.PerSecond) > secs {
			secs = len(a.PerSecond)
		}
		for _, v := range a.PerSecond {
			if v > peak {
				peak = v
			}
		}
	}

	children := []layout.FlexChild{
		layout.Rigid(comp.Text(t, t.Sz.Caption, t.P.Dim, fmt.Sprintf(
			"0 to %d s after the burst, peak %d receptions/s, summed over seeds",
			secs, peak))),
		layout.Rigid(layout.Spacer{Height: t.Sp.S}.Layout),
	}
	for _, a := range arms {
		a := a
		if len(a.PerSecond) == 0 {
			continue
		}
		children = append(children,
			layout.Rigid(comp.OneLine(t, t.Sz.Caption, t.P.Faint, a.Arm, false)),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return histogram(t, gtx, a.PerSecond, peak)
			}),
			layout.Rigid(layout.Spacer{Height: t.Sp.S}.Layout),
		)
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
}

// histogram draws one arm's flood as bars on a shared scale.
func histogram(t *theme.Theme, gtx layout.Context, vals []int, peak int) layout.Dimensions {
	h := gtx.Dp(unit.Dp(64))
	w := gtx.Constraints.Max.X
	if len(vals) == 0 || w <= 0 {
		return layout.Dimensions{Size: image.Pt(w, h)}
	}
	comp.FillRect(gtx, image.Pt(w, h), t.P.Sunk)
	bar := w / len(vals)
	if bar < 1 {
		bar = 1
	}
	for i, v := range vals {
		if v <= 0 {
			continue
		}
		bh := v * h / peak
		if bh < 1 {
			bh = 1
		}
		off := op.Offset(image.Pt(i*bar, h-bh)).Push(gtx.Ops)
		// One pixel of air between bars, so a run of equal seconds reads as
		// several rather than as one wide block.
		comp.FillRect(gtx, image.Pt(max(bar-1, 1), bh), t.P.Accent)
		off.Pop()
	}
	return layout.Dimensions{Size: image.Pt(w, h)}
}

// timelinesPanel is one lane per node of a chosen metric over the run.
//
// Sent and heard per node, so a node that stopped relaying shows as a lane
// that goes flat while its neighbours keep climbing.
type timelinesPanel struct {
	tb   comp.Table
	init bool
}

func (p *timelinesPanel) Draw(t *theme.Theme, gtx layout.Context, s *state.Snapshot) layout.Dimensions {
	if !p.init {
		p.tb.Cols = []comp.Column{
			{Title: "node", Width: 190, Sortable: true},
			{Title: "sent", Width: 70, Right: true, Mono: true, Sortable: true, Numeric: true},
			{Title: "heard", Width: 70, Right: true, Mono: true, Sortable: true, Numeric: true},
			{Title: "relative", Mono: true},
		}
		p.tb.SortCol, p.tb.SortDesc, p.init = 1, true, true
	}
	// A sweep's floods take precedence over the running counters: in the Bench
	// this panel is being read against the matrix beside it, and per-node
	// totals answer a different question from the shape of a flood.
	if s != nil && armsHaveShape(s.Experiment) {
		return floodShapes(t, gtx, s.Experiment)
	}
	if s == nil || len(s.Scores) == 0 {
		return layout.Center.Layout(gtx, comp.Text(t, t.Sz.Body, t.P.Dim,
			"no counters yet - play the simulation"))
	}
	most := 0
	for _, v := range s.Scores {
		if v.Sent > most {
			most = v.Sent
		}
	}
	rows := make([]comp.Row, 0, len(s.Scores))
	for _, v := range s.Scores {
		rows = append(rows, comp.Row{Key: v.Name, Cells: []string{
			v.Name, fmt.Sprintf("%d", v.Sent), fmt.Sprintf("%d", v.Heard),
			bar(v.Sent, most),
		}})
	}
	p.tb.SetRows(rows)
	return p.tb.Layout(t, gtx, nil)
}

// bar is a text bar, which survives being copied out of the interface in a way
// a drawn one does not.
func bar(v, most int) string {
	if most <= 0 {
		return ""
	}
	const width = 28
	n := v * width / most
	out := make([]byte, 0, width)
	for i := 0; i < width; i++ {
		if i < n {
			out = append(out, '#')
		} else {
			out = append(out, ' ')
		}
	}
	return string(out)
}
