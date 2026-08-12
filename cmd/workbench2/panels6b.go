// Three more of P6's panels, each a projection of what the snapshot carries.
package main

import (
	"fmt"
	"sort"

	"gioui.org/layout"

	"github.com/A13xB0/meshcoresim/internal/gui/comp"
	"github.com/A13xB0/meshcoresim/internal/gui/state"
	"github.com/A13xB0/meshcoresim/internal/gui/theme"
)

// fleetPanel is what is deployed and what it is running (6.24).
//
// Grouped by firmware rather than listed by node, because the question a fleet
// panel answers is "what is out there", and three hundred rows saying
// repeater-v1.17.0 answer it worse than one row saying 272.
type fleetPanel struct {
	tb   comp.Table
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
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(comp.SectionTitle(t,
			fmt.Sprintf("%d nodes in %d builds", len(s.Nodes), len(rows)))),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return p.tb.Layout(t, gtx, nil)
		}),
	)
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
