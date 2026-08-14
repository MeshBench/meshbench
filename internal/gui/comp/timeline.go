package comp

import (
	"fmt"
	"image"
	"math"
	"sort"

	"gioui.org/f32"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"

	"github.com/MeshBench/meshbench/internal/gui/state"
	"github.com/MeshBench/meshbench/internal/gui/theme"
)

// Timeline is packets on a shared time axis, one lane per node (6.6).
//
// The shared axis is the whole point. A relay is a transmission at one node
// and a reception at several others a few milliseconds later, and the only way
// to see that it was a relay rather than a fresh message is to see the two
// events line up vertically.
type Timeline struct {
	// Window is how much simulated time is shown, in milliseconds. Ending at
	// the most recent event rather than at the current time, so a paused run
	// still shows the traffic that led to the pause.
	WindowMs uint32
	lanes    []string
	laneOf   map[string]int
	forSeq   uint64
}

// Layout draws the timeline for one snapshot.
func (tl *Timeline) Layout(t *theme.Theme, gtx layout.Context, s *state.Snapshot) layout.Dimensions {
	sz := gtx.Constraints.Max
	defer clip.Rect{Max: sz}.Push(gtx.Ops).Pop()
	paint.FillShape(gtx.Ops, t.P.Sunk, clip.Rect{Max: sz}.Op())

	if s == nil || len(s.Events) == 0 {
		return layout.Center.Layout(gtx, Text(t, t.Sz.Body, t.P.Dim,
			"no traffic yet - play the simulation, or originate a packet"))
	}
	if tl.WindowMs == 0 {
		tl.WindowMs = 20000
	}
	tl.buildLanes(s)

	gutter := gtx.Dp(150)
	if gutter > sz.X/3 {
		gutter = sz.X / 3
	}
	plotW := sz.X - gutter - gtx.Dp(t.Sp.M)
	if plotW < 40 || len(tl.lanes) == 0 {
		return layout.Dimensions{Size: sz}
	}

	// The window ends at the newest event, not at now: a run paused after
	// something interesting should still be showing it.
	end := s.Events[len(s.Events)-1].AtMs
	start := uint32(0)
	if end > tl.WindowMs {
		start = end - tl.WindowMs
	}
	span := float32(end - start)
	if span <= 0 {
		span = 1
	}

	laneH := sz.Y / max(1, len(tl.lanes))
	if laneH < gtx.Dp(9) {
		laneH = gtx.Dp(9)
	}
	x := func(at uint32) float32 {
		return float32(gutter) + float32(at-start)/span*float32(plotW)
	}

	tl.laneRules(t, gtx, sz, gutter, laneH)
	tl.marks(t, gtx, s, start, x, laneH, sz)
	tl.laneNames(t, gtx, laneH, gutter)
	tl.axis(t, gtx, sz, start, end, gutter, plotW)
	return layout.Dimensions{Size: sz}
}

// buildLanes decides the lane order, and keeps it while the run does.
//
// Busiest first, so the lanes worth reading are at the top, and recomputed
// only when the snapshot changes: a lane order that shuffles between frames
// makes a timeline unreadable however correct each frame is.
func (tl *Timeline) buildLanes(s *state.Snapshot) {
	if s.Seq == tl.forSeq && tl.laneOf != nil {
		return
	}
	tl.forSeq = s.Seq
	count := map[string]int{}
	for i := range s.Events {
		count[s.Events[i].From]++
		if s.Events[i].To != "" {
			count[s.Events[i].To]++
		}
	}
	tl.lanes = tl.lanes[:0]
	for name := range count {
		tl.lanes = append(tl.lanes, name)
	}
	sort.Slice(tl.lanes, func(i, j int) bool {
		if count[tl.lanes[i]] != count[tl.lanes[j]] {
			return count[tl.lanes[i]] > count[tl.lanes[j]]
		}
		return tl.lanes[i] < tl.lanes[j]
	})
	tl.laneOf = make(map[string]int, len(tl.lanes))
	for i, n := range tl.lanes {
		tl.laneOf[n] = i
	}
}

func (tl *Timeline) laneRules(t *theme.Theme, gtx layout.Context, sz image.Point,
	gutter, laneH int) {

	for i := range tl.lanes {
		y := i*laneH + laneH/2
		if y > sz.Y {
			break
		}
		paint.FillShape(gtx.Ops, theme.Alpha(t.P.Rule, 0.5), clip.Rect{
			Min: image.Pt(gutter, y), Max: image.Pt(sz.X, y+1)}.Op())
	}
}

// marks draws one tick per event, coloured by kind.
func (tl *Timeline) marks(t *theme.Theme, gtx layout.Context, s *state.Snapshot,
	start uint32, x func(uint32) float32, laneH int, sz image.Point) {

	kinds := []struct {
		kind string
		col  colorNRGBA
		h    float32
	}{
		{"tx", t.P.Accent, 0.42},
		{"rx", t.P.Good, 0.30},
		{"miss", t.P.Bad, 0.22},
	}
	for _, k := range kinds {
		var path clip.Path
		path.Begin(gtx.Ops)
		n := 0
		for i := range s.Events {
			e := &s.Events[i]
			if e.Kind != k.kind || e.AtMs < start {
				continue
			}
			// A reception belongs in the lane of whoever heard it; everything
			// else in the lane of whoever did it.
			who := e.From
			if e.Kind != "tx" && e.To != "" {
				who = e.To
			}
			lane, ok := tl.laneOf[who]
			if !ok {
				continue
			}
			cx := x(e.AtMs)
			cy := float32(lane*laneH + laneH/2)
			half := float32(laneH) * k.h
			if half < 2 {
				half = 2
			}
			segment(&path, f32.Pt(cx, cy-half), f32.Pt(cx, cy+half), 2)
			n++
		}
		spec := path.End()
		if n > 0 {
			paint.FillShape(gtx.Ops, theme.Alpha(k.col, 0.9),
				clip.Outline{Path: spec}.Op())
		}
	}
}

func (tl *Timeline) laneNames(t *theme.Theme, gtx layout.Context, laneH, gutter int) {
	for i, name := range tl.lanes {
		y := i*laneH + laneH/2 - gtx.Dp(7)
		if y > gtx.Constraints.Max.Y {
			break
		}
		if laneH < gtx.Dp(12) {
			// Below this the names would overlap, and a column of overlapping
			// names is worse than lanes read off the plot.
			break
		}
		off := op.Offset(image.Pt(gtx.Dp(t.Sp.S), y)).Push(gtx.Ops)
		g := gtx
		g.Constraints.Min = image.Point{}
		g.Constraints.Max = image.Pt(gutter-gtx.Dp(t.Sp.M), laneH)
		OneLine(t, t.Sz.Caption, t.P.Dim, name, false)(g)
		off.Pop()
	}
}

// axis is the time scale along the bottom.
func (tl *Timeline) axis(t *theme.Theme, gtx layout.Context, sz image.Point,
	start, end uint32, gutter, plotW int) {

	y := sz.Y - gtx.Dp(16)
	paint.FillShape(gtx.Ops, t.P.Rule, clip.Rect{
		Min: image.Pt(gutter, y), Max: image.Pt(sz.X, y+1)}.Op())

	// Ticks on a round number of seconds, chosen so there are about six.
	spanMs := float64(end - start)
	if spanMs <= 0 {
		return
	}
	step := niceDistance(spanMs / 6)
	for at := math.Ceil(float64(start)/step) * step; at <= float64(end); at += step {
		px := gutter + int(float64(plotW)*(at-float64(start))/spanMs)
		paint.FillShape(gtx.Ops, t.P.Rule, clip.Rect{
			Min: image.Pt(px, y), Max: image.Pt(px+1, y+gtx.Dp(4))}.Op())
		off := op.Offset(image.Pt(px+2, y+gtx.Dp(4))).Push(gtx.Ops)
		Mono(t, t.Sz.Caption, t.P.Faint, fmt.Sprintf("%.1fs", at/1000))(unbounded(gtx))
		off.Pop()
	}
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
