// The Fleet panel: what is deployed, and what it is running.
package workbench

import (
	"fmt"
	"sort"

	"gioui.org/layout"

	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/ui/comp"
	"github.com/MeshBench/meshbench/internal/ui/theme"
)

// fleetPanel is what is deployed and what it is running.
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
