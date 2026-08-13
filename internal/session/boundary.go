// The study area, and what it decides.
//
// A boundary is not the firmware's region concept - it decides which nodes are
// in the study, not which packets are forwarded - and confusing the two is how
// somebody concludes the RF model is broken. Both words appear in this
// application, so both are spelled out wherever they meet.
package session

import (
	"context"
	"fmt"
	"math"
	"strings"

	"github.com/A13xB0/meshcoresim/internal/boundary"
	"github.com/A13xB0/meshcoresim/internal/gui/state"
	"github.com/A13xB0/meshcoresim/internal/scenario"
)

func registerBoundary(st *state.Store, s *Sim) {
	// boundary.set: search for a place by name.
	st.Handle("boundary.set", func(w *state.World, p any) (any, error) {
		q, _ := stringField(p, "query")
		if strings.TrimSpace(q) == "" {
			return nil, fmt.Errorf("boundary.set needs a place to search for")
		}
		c := &boundary.Client{}
		found, err := c.Search(context.Background(), q)
		if err != nil {
			return nil, err
		}
		out := make([]map[string]any, 0, len(found))
		names := make([]string, 0, len(found))
		for _, f := range found {
			out = append(out, map[string]any{"name": f.Name, "kind": f.Kind})
			names = append(names, f.Name)
		}
		s.foundAreas = found
		w.Say(fmt.Sprintf("%d places match %q", len(found), q))
		return map[string]any{"found": out, "names": names}, nil
	})

	// boundary.accept: take one of the matches into the study area. The
	// chosen set unions, because a study area is often two council areas
	// rather than one.
	st.Handle("boundary.accept", func(w *state.World, p any) (any, error) {
		name, _ := stringField(p, "name")
		if name == "" {
			return nil, fmt.Errorf("boundary.accept needs a name")
		}
		// The geometry the search already returned.
		//
		// It went through the client's disk cache before, which is empty
		// unless a CacheDir was set - and none was, so every accept failed
		// with "no boundary for that", having just been handed the boundary.
		var chosen []scenario.Boundary
		matched := name
		for _, f := range s.foundAreas {
			// What the search offered, not only what was typed: searching
			// "Scotland" returns "Alba / Scotland", and refusing the thing it
			// just offered is a dead end with no way out from the panel.
			if strings.EqualFold(f.Name, name) ||
				strings.Contains(strings.ToLower(f.Name), strings.ToLower(name)) {
				chosen, matched = f.Boundaries, f.Name
				break
			}
		}
		if len(chosen) == 0 {
			var offered []string
			for _, f := range s.foundAreas {
				offered = append(offered, f.Name)
			}
			if len(offered) == 0 {
				return nil, fmt.Errorf("no boundary for %q; search for it first", name)
			}
			return nil, fmt.Errorf("no boundary matching %q; the search offered: %s",
				name, strings.Join(offered, ", "))
		}
		name = matched
		area := state.Area{Name: name}
		for _, b := range chosen {
			for _, r := range b.Rings {
				var ring []state.Point
				for _, pt := range r {
					ring = append(ring, state.Point{Lat: pt.Lat, Lon: pt.Lon})
				}
				area.Rings = append(area.Rings, ring)
			}
		}
		w.Areas = append(w.Areas, area)
		s.areas = append(s.areas, chosen...)
		w.Say("study area now includes " + name)
		return map[string]any{"accepted": name, "areas": len(w.Areas)}, nil
	})

	// boundary.prune: remove what is outside, with the margin kept, because a
	// node just outside still interferes with one just inside.
	st.Handle("boundary.prune", func(w *state.World, p any) (any, error) {
		if len(s.areas) == 0 {
			return nil, fmt.Errorf("no study area accepted yet")
		}
		margin := float64(w.MarginKm)
		if v, ok := numField(p, "margin_km"); ok && v >= 0 {
			margin = v
		}
		kept := make([]scenario.Node, 0, len(s.nodes))
		for _, n := range s.nodes {
			if withinAny(s.areas, n.Position.Lat, n.Position.Lon, margin) {
				kept = append(kept, n)
			}
		}
		removed := len(s.nodes) - len(kept)
		if removed == 0 {
			return map[string]any{"removed": 0, "nodes": len(kept)}, nil
		}
		s.buildSeeded(kept, s.freqMHz, s.seed)
		w.Nodes = stateNodes(kept)
		w.Links = nil
		s.warm(st, len(kept))
		w.Say(fmt.Sprintf("removed %d nodes outside the study area", removed))
		return map[string]any{"removed": removed, "nodes": len(kept)}, nil
	})
}

// withinAny reports whether a position is inside any boundary, or within the
// margin of one.
//
// The margin is kept because a node just outside the study area still
// interferes with one just inside it, and pruning it away makes the mesh look
// quieter than it is.
func withinAny(areas []scenario.Boundary, lat, lon, marginKm float64) bool {
	r := scenario.Region{Boundaries: areas}
	if r.Contains(scenario.LatLon{Lat: lat, Lon: lon}) {
		return true
	}
	if marginKm <= 0 {
		return false
	}
	// A ring of test points at the margin: cheap, and enough to keep a node
	// whose own position is outside but whose neighbourhood is not.
	const steps = 12
	dLat := marginKm / 111.32
	for i := 0; i < steps; i++ {
		a := 2 * math.Pi * float64(i) / steps
		dLon := dLat / math.Max(0.05, math.Cos(lat*math.Pi/180))
		if r.Contains(scenario.LatLon{
			Lat: lat + dLat*math.Sin(a), Lon: lon + dLon*math.Cos(a)}) {
			return true
		}
	}
	return false
}
