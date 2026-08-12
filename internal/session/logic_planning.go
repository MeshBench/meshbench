package session

import (
	"fmt"
	"github.com/A13xB0/meshcoresim/internal/coverage"
	"github.com/A13xB0/meshcoresim/internal/gui/state"
	"github.com/A13xB0/meshcoresim/internal/linkbudget"
	"github.com/A13xB0/meshcoresim/internal/planning"
)

// budgetChecker answers "can these two work" with the same link budget the map
// draws, so a planned route and a drawn link cannot disagree.
type budgetChecker struct {
	Sim *Sim
}

func (c budgetChecker) Works(a, b planning.Site) bool {
	// A planned site has no antenna of its own yet, so it is checked as the
	// modest thing somebody would actually build: the first node's radio at
	// the mast height the search was given.
	if len(c.Sim.nodes) == 0 {
		return false
	}
	ref := c.Sim.nodes[0]
	loss, ok := coverage.LossBetween(c.Sim.terrain(),
		a.Lat, a.Lon, heightOf(a, ref.HeightAGLm),
		b.Lat, b.Lon, heightOf(b, ref.HeightAGLm),
		869.618, 120)
	if !ok {
		return false
	}
	return linkbudget.MarginDB(ref, ref, loss) >= 0
}

// routesBetween searches for ways to connect two named nodes.
func (s *Sim) routesBetween(from, to string) ([]state.Route, error) {
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
	routes, err := planning.Bridge(a, b, s.terrain(), budgetChecker{Sim: s},
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
