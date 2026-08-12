// Planning: what would it take to connect these two (6.23).
//
// Through internal/planning, whose search returns the fewest *new* sites
// rather than the fewest hops. Existing infrastructure is free, and a five-hop
// route over four repeaters that already exist is a better answer than a
// three-hop route needing two new masts - which is the opposite of what a
// plain shortest-path search returns.
package main

import (
	"fmt"

	"gioui.org/layout"

	"github.com/A13xB0/meshcoresim/internal/coverage"
	"github.com/A13xB0/meshcoresim/internal/gui/comp"
	"github.com/A13xB0/meshcoresim/internal/gui/state"
	"github.com/A13xB0/meshcoresim/internal/gui/theme"
	"github.com/A13xB0/meshcoresim/internal/linkbudget"
	"github.com/A13xB0/meshcoresim/internal/planning"
)

// planPanel shows the routes between the two selected nodes.
type planPanel struct {
	tb   comp.Table
	init bool
	run  comp.Button
	// OnRun asks the store to search.
	OnRun func()
}

func (p *planPanel) Draw(t *theme.Theme, gtx layout.Context, s *state.Snapshot) layout.Dimensions {
	if !p.init {
		p.tb.Cols = []comp.Column{
			{Title: "route", Width: 80, Right: true, Mono: true},
			{Title: "new sites", Width: 96, Right: true, Mono: true, Sortable: true},
			{Title: "hops", Width: 70, Right: true, Mono: true},
			{Title: "longest hop", Width: 110, Right: true, Mono: true, Sortable: true},
			{Title: "through"},
		}
		p.tb.SortCol, p.init = 1, true
		p.run.Label, p.run.Kind = "find routes between the two selected nodes", comp.Primary
	}
	if p.run.Click.Clicked(gtx) && p.OnRun != nil {
		p.OnRun()
	}
	body := func(gtx layout.Context) layout.Dimensions {
		if s == nil || len(s.Routes) == 0 {
			return layout.Center.Layout(gtx, comp.Text(t, t.Sz.Body, t.P.Dim,
				"select two nodes, then search - existing repeaters are reused for free"))
		}
		rows := make([]comp.Row, 0, len(s.Routes))
		for i, r := range s.Routes {
			rows = append(rows, comp.Row{
				Key: fmt.Sprintf("%d", i),
				Cells: []string{
					fmt.Sprintf("%d", i+1),
					fmt.Sprintf("%d", r.NewSites),
					fmt.Sprintf("%d", r.Hops),
					fmt.Sprintf("%.1f km", r.LongestHopKm),
					r.Through,
				},
			})
		}
		p.tb.SetRows(rows)
		return p.tb.Layout(t, gtx, nil)
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(comp.SectionTitle(t, "fewest new sites, not fewest hops")),
		layout.Flexed(1, body),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.X = 0
			return p.run.Layout(t, gtx)
		}),
	)
}

// budgetChecker answers "can these two work" with the same link budget the map
// draws, so a planned route and a drawn link cannot disagree.
type budgetChecker struct {
	sim *sim
}

func (c budgetChecker) Works(a, b planning.Site) bool {
	// A planned site has no antenna of its own yet, so it is checked as the
	// modest thing somebody would actually build: the first node's radio at
	// the mast height the search was given.
	if len(c.sim.nodes) == 0 {
		return false
	}
	ref := c.sim.nodes[0]
	loss, ok := coverage.LossBetween(c.sim.terrain(),
		a.Lat, a.Lon, heightOf(a, ref.HeightAGLm),
		b.Lat, b.Lon, heightOf(b, ref.HeightAGLm),
		869.618, 120)
	if !ok {
		return false
	}
	return linkbudget.MarginDB(ref, ref, loss) >= 0
}

// routesBetween searches for ways to connect two named nodes.
func (s *sim) routesBetween(from, to string) ([]state.Route, error) {
	find := func(name string) (planning.Site, bool) {
		for _, n := range s.nodes {
			if n.Name == name {
				return planning.Site{
					Lat: n.Position.Lat, Lon: n.Position.Lon,
					HeightAGLm: n.HeightAGLm, Existing: true, Name: n.Name,
				}, true
			}
		}
		return planning.Site{}, false
	}
	a, ok1 := find(from)
	b, ok2 := find(to)
	if !ok1 || !ok2 {
		return nil, fmt.Errorf("need two selected nodes to plan between")
	}
	existing := make([]planning.Site, 0, len(s.nodes))
	for _, n := range s.nodes {
		existing = append(existing, planning.Site{
			Lat: n.Position.Lat, Lon: n.Position.Lon,
			HeightAGLm: n.HeightAGLm, Existing: true, Name: n.Name,
		})
	}
	routes, err := planning.Bridge(a, b, s.terrain(), budgetChecker{sim: s},
		planning.BridgeOptions{
			Existing: existing, MastHeightM: 15, MaxNew: 3,
			CandidateStep: 0.05, Alternatives: 4,
		})
	if err != nil {
		return nil, err
	}
	out := make([]state.Route, 0, len(routes))
	for _, r := range routes {
		through := ""
		for i, site := range r.Sites {
			name := site.Name
			if name == "" {
				name = "new mast"
			}
			if i > 0 {
				through += " -> "
			}
			through += name
		}
		out = append(out, state.Route{
			NewSites: r.NewSites, Hops: len(r.Sites) - 1,
			LongestHopKm: r.LongestHopKm, Through: through,
		})
	}
	return out, nil
}

// heightOf is a site's mast height, falling back to the reference node's when
// the search has not given one - a candidate hilltop with no height is a
// hilltop somebody would put a mast on, not a radio lying in the heather.
func heightOf(s planning.Site, fallback float64) float64 {
	if s.HeightAGLm > 0 {
		return s.HeightAGLm
	}
	return fallback
}
