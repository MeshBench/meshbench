// The study area, and what it decides.
//
// A boundary is not the firmware's region concept - it decides which nodes are
// in the study, not which packets are forwarded - and confusing the two is how
// somebody concludes the RF model is broken. Both words appear in this
// application, so both are spelled out wherever they meet.

package boundary

import (
	"context"
	"fmt"
	"math"
	"strings"

	"github.com/MeshBench/meshbench/internal/app/session"
	"github.com/MeshBench/meshbench/internal/app/state"
	worldb "github.com/MeshBench/meshbench/internal/world/boundary"
	"github.com/MeshBench/meshbench/internal/world/scenario"
)

func registerBoundary(st *state.Store, s *session.Sim) {
	// boundary.set: search for a place by name.
	st.Handle("boundary.set", func(w *state.World, p any) (any, error) {
		q, _ := session.StringField(p, "query")
		if strings.TrimSpace(q) == "" {
			return nil, fmt.Errorf("boundary.set needs a place to search for")
		}
		c := &worldb.Client{}
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
		s.SetFoundAreas(found)
		w.Say(fmt.Sprintf("%d places match %q", len(found), q))
		return map[string]any{"found": out, "names": names}, nil
	})

	// boundary.accept: take one of the matches into the study area. The
	// chosen set unions, because a study area is often two council areas
	// rather than one.
	st.Handle("boundary.accept", func(w *state.World, p any) (any, error) {
		name, _ := session.StringField(p, "name")
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
		for _, f := range s.FoundAreas() {
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
			for _, f := range s.FoundAreas() {
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
		// Once each. Accepting the same place twice used to stack it, and a
		// study area listed as "Fife, Fife, Fife" says nothing three times.
		for _, a := range w.Areas {
			if strings.EqualFold(a.Name, name) {
				w.Say(name + " is already in the study area")
				return map[string]any{"accepted": name, "areas": len(w.Areas)}, nil
			}
		}
		w.Areas = append(w.Areas, area)
		s.SetAreas(append(s.Areas(), chosen...))
		w.Say(fmt.Sprintf("study area now includes %s - %d in all", name, len(w.Areas)))
		return map[string]any{"accepted": name, "areas": len(w.Areas)}, nil
	})

	// boundary.remove: take one area back out of the study.
	//
	// A study area is built up from several places - Scotland and Ireland, or
	// four council areas - so there has to be a way to take one out again
	// without starting over. It changes what is measured, never what is
	// loaded: the nodes stay until something prunes them.
	st.Handle("boundary.remove", func(w *state.World, p any) (any, error) {
		name, _ := session.StringField(p, "name")
		if name == "" {
			return nil, fmt.Errorf("boundary.remove needs the name of an area")
		}
		kept := w.Areas[:0]
		found := false
		for _, a := range w.Areas {
			if strings.EqualFold(a.Name, name) {
				found, name = true, a.Name
				continue
			}
			kept = append(kept, a)
		}
		if !found {
			var have []string
			for _, a := range w.Areas {
				have = append(have, a.Name)
			}
			if len(have) == 0 {
				return nil, fmt.Errorf("the study area is empty")
			}
			return nil, fmt.Errorf("no area called %q; there is %s",
				name, strings.Join(have, ", "))
		}
		w.Areas = kept
		// The geometry follows the list it is drawn from, by the name each
		// boundary carries, or the two would disagree about the study.
		bounds := s.Areas()[:0]
		for _, b := range s.Areas() {
			if !strings.EqualFold(b.Name, name) {
				bounds = append(bounds, b)
			}
		}
		s.SetAreas(bounds)
		w.Say(fmt.Sprintf("%s is no longer in the study area - %d left", name, len(w.Areas)))
		return map[string]any{"removed": name, "areas": len(w.Areas)}, nil
	})

	// boundary.prune: remove what is outside, with the margin kept, because a
	// node just outside still interferes with one just inside.
	st.Handle("boundary.prune", func(w *state.World, p any) (any, error) {
		if len(s.Areas()) == 0 {
			return nil, fmt.Errorf("no study area accepted yet")
		}
		margin := float64(w.MarginKm)
		if v, ok := session.NumField(p, "margin_km"); ok && v >= 0 {
			margin = v
		}
		kept := make([]scenario.Node, 0, len(s.Nodes()))
		for _, n := range s.Nodes() {
			if withinAny(s.Areas(), n.Position.Lat, n.Position.Lon, margin) {
				kept = append(kept, n)
			}
		}
		removed := len(s.Nodes()) - len(kept)
		if removed == 0 {
			return map[string]any{"removed": 0, "nodes": len(kept)}, nil
		}
		s.BuildSeeded(kept, s.FreqMHz(), s.Seed())
		w.Nodes = session.StateNodes(kept)
		w.Links = nil
		s.Warm(st, len(kept))
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
