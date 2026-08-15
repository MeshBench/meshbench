// Three more of P6's panels, each a projection of what the snapshot carries.
package workbench

import (
	"fmt"
	"image"
	"sort"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"

	"github.com/MeshBench/meshbench/internal/gui/comp"
	"github.com/MeshBench/meshbench/internal/gui/state"
	"github.com/MeshBench/meshbench/internal/gui/theme"
)

// fleetPanel is what is deployed and what it is running (6.24).
//
// Grouped by firmware rather than listed by node, because the question a fleet
// panel answers is "what is out there", and three hundred rows saying
// repeater-v1.17.0 answer it worse than one row saying 272.
type fleetPanel struct {
	tb  comp.Table
	rep comp.Table
	// open is the reply whose nodes are being listed. Fifty-six rows all
	// saying "OK - Advert sent" is not fifty-six pieces of information; the
	// answer is one line and the interesting part is who did not give it.
	open string
	back comp.Button
	init bool
}

func (p *fleetPanel) Draw(t *theme.Theme, gtx layout.Context, s *state.Snapshot) layout.Dimensions {
	if !p.init {
		p.tb.Cols = []comp.Column{
			{Title: "firmware", Width: 220, Mono: true, Sortable: true},
			{Title: "kind", Width: 170, Sortable: true},
			{Title: "nodes", Width: 90, Right: true, Mono: true, Sortable: true},
			{Title: "with regions", Width: 120, Right: true, Mono: true},
			{Title: "example"},
		}
		p.rep.Cols = []comp.Column{
			{Title: "nodes", Width: 90, Right: true, Mono: true, Sortable: true},
			{Title: "said", Mono: true},
		}
		p.back.Label, p.back.Kind = "all replies", comp.Quiet
		p.tb.SortCol, p.tb.SortDesc, p.init = 2, true, true
	}
	if s == nil {
		return layout.Dimensions{}
	}
	type group struct {
		count, withRegions int
		example            string
	}
	groups := map[string]*group{}
	for i := range s.Nodes {
		n := &s.Nodes[i]
		fw := n.Firmware
		if fw == "" {
			// Distinct from a node whose firmware is unknown to us: this one
			// has none, and saying "unknown" would invent a question.
			fw = "none"
		}
		key := fw + "\x00" + n.Kind
		g := groups[key]
		if g == nil {
			g = &group{example: n.Name}
			groups[key] = g
		}
		g.count++
		if len(n.Regions) > 0 {
			g.withRegions++
		}
	}
	keys := make([]string, 0, len(groups))
	for k := range groups {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	rows := make([]comp.Row, 0, len(keys))
	for _, k := range keys {
		g := groups[k]
		fw, kind := splitKey(k)
		rows = append(rows, comp.Row{Key: k, Cells: []string{
			fw, kind, fmt.Sprintf("%d", g.count),
			fmt.Sprintf("%d", g.withRegions), g.example,
		}})
	}
	p.tb.SetRows(rows)

	// What they said, under what they are.
	//
	// A command sent to forty nodes with no reply shown is indistinguishable
	// from one that went nowhere, which is exactly how this panel read.
	// Grouped by what was said, because that is the shape of the answer.
	said := map[string][]string{}
	var order []string
	for _, r := range s.FleetReplies {
		if _, seen := said[r.Reply]; !seen {
			order = append(order, r.Reply)
		}
		said[r.Reply] = append(said[r.Reply], r.Node)
	}
	sort.SliceStable(order, func(i, j int) bool {
		return len(said[order[i]]) > len(said[order[j]])
	})
	if _, still := said[p.open]; !still {
		p.open = ""
	}
	if p.back.Click.Clicked(gtx) {
		p.open = ""
	}

	var replies []comp.Row
	if p.open == "" {
		p.rep.Cols[0].Title, p.rep.Cols[1].Title = "nodes", "said"
		for _, reply := range order {
			replies = append(replies, comp.Row{Key: reply, Cells: []string{
				fmt.Sprintf("%d", len(said[reply])), reply,
			}})
		}
	} else {
		// One group, opened: who said it.
		p.rep.Cols[0].Title, p.rep.Cols[1].Title = "node", "said"
		for _, n := range said[p.open] {
			replies = append(replies, comp.Row{Key: n, Cells: []string{n, p.open}})
		}
	}
	p.rep.SetRows(replies)

	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(comp.SectionTitle(t,
			fmt.Sprintf("%d nodes in %d builds", len(s.Nodes), len(rows)))),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return p.tb.Layout(t, gtx, nil)
		}),
		layout.Rigid(comp.SectionTitle(t, repliesTitle(s, p.open))),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			if len(replies) == 0 {
				return layout.Center.Layout(gtx, comp.Text(t, t.Sz.Caption, t.P.Faint,
					"replies appear here, one row per node, in the firmware's own words"))
			}
			return p.rep.Layout(t, gtx, func(key string) {
				// Selecting a group opens it; selecting a node inside one
				// does nothing, because there is nothing further to open.
				if p.open == "" {
					p.open = key
				}
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if p.open == "" {
				return layout.Dimensions{}
			}
			return p.back.Layout(t, gtx)
		}),
	)
}

// repliesTitle says what the replies are answers to, because a table of "OK"
// with no question above it is not an answer to anything.
func repliesTitle(s *state.Snapshot, open string) string {
	if s.FleetCommand == "" {
		return "replies"
	}
	if open != "" {
		n := 0
		for _, r := range s.FleetReplies {
			if r.Reply == open {
				n++
			}
		}
		return fmt.Sprintf("the %d that said this to %q", n, s.FleetCommand)
	}
	return fmt.Sprintf("%d replied to %q", len(s.FleetReplies), s.FleetCommand)
}

func splitKey(k string) (a, b string) {
	for i := 0; i < len(k); i++ {
		if k[i] == 0 {
			return k[:i], k[i+1:]
		}
	}
	return k, ""
}

// boundaryPanel is the study area and what it contains (6.22).
type boundaryPanel struct {
	tb   comp.Table
	init bool
}

func (p *boundaryPanel) Draw(t *theme.Theme, gtx layout.Context, s *state.Snapshot) layout.Dimensions {
	if !p.init {
		p.tb.Cols = []comp.Column{
			{Title: "area", Width: 260, Sortable: true},
			{Title: "rings", Width: 80, Right: true, Mono: true},
			{Title: "holes", Width: 80, Right: true, Mono: true},
			{Title: "points", Right: true, Mono: true},
		}
		p.init = true
	}
	if s == nil {
		return layout.Dimensions{}
	}
	if len(s.Areas) == 0 {
		return layout.Center.Layout(gtx, comp.Text(t, t.Sz.Body, t.P.Dim,
			"no boundary in this network - accept one to bound a study"))
	}
	rows := make([]comp.Row, 0, len(s.Areas))
	for _, a := range s.Areas {
		pts := 0
		for _, r := range a.Rings {
			pts += len(r)
		}
		for _, h := range a.Holes {
			pts += len(h)
		}
		rows = append(rows, comp.Row{Key: a.Name, Cells: []string{
			a.Name, fmt.Sprintf("%d", len(a.Rings)),
			fmt.Sprintf("%d", len(a.Holes)), fmt.Sprintf("%d", pts),
		}})
	}
	p.tb.SetRows(rows)
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(comp.SectionTitle(t, "study area")),
		layout.Rigid(comp.Text(t, t.Sz.Caption, t.P.Dim, fmt.Sprintf(
			"nodes within %g km outside the boundary are simulated too, "+
				"because a repeater just over the line is still heard", s.MarginKm))),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return p.tb.Layout(t, gtx, nil)
		}),
	)
}

// timelinesPanel is one lane per node of a chosen metric over the run (6.16).
//
// Distinct from the packet timeline: that one is events, this one is counters.
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
			{Title: "sent", Width: 70, Right: true, Mono: true, Sortable: true},
			{Title: "heard", Width: 70, Right: true, Mono: true, Sortable: true},
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
