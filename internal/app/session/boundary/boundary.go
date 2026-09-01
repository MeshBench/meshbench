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
	"net/http"
	"strings"
	"time"

	"github.com/MeshBench/meshbench/internal/app/session"
	"github.com/MeshBench/meshbench/internal/app/state"
	worldb "github.com/MeshBench/meshbench/internal/world/boundary"
	"github.com/MeshBench/meshbench/internal/world/scenario"
)

// searchDeadline is how long the store's goroutine may be spent on a place
// search. Generous enough for Nominatim to return a national coastline over a
// slow link, short enough that a hung geocoder is a refusal rather than a
// workbench that stops answering.
const searchDeadline = 30 * time.Second

func registerBoundary(st *state.Store, s *session.Sim) {
	st.HandleSpec("boundary.set", state.Spec{
		What: "look a place up in the gazetteer and offer what it matched, " +
			"which is the search half of choosing a study area and changes " +
			"nothing on its own",
		Params: []state.Param{
			{Name: "query", Type: state.ParamString, Required: true, Primary: true,
				What: "the place to search for; blank or whitespace is refused " +
					"rather than answered with everything"},
		},
		Returns: []string{"found", "names"},
		Answers: "`found` is a row per match with its name and kind, and " +
			"`names` the same names on their own, for handing straight to " +
			"boundary.accept. Nothing joins the study area until one is " +
			"accepted, and the names are the gazetteer's own: a search for " +
			"Scotland comes back as \"Alba / Scotland\". It needs the network " +
			"and gives the geocoder thirty seconds.",
		Example: &state.Example{
			Params:   map[string]any{"query": "Fife"},
			What:     "find a council area to study",
			Runnable: false,
		},
	}, func(w *state.World, p any) (any, error) {
		q, _ := session.StringField(p, "query")
		if strings.TrimSpace(q) == "" {
			return nil, fmt.Errorf("boundary.set needs a place to search for")
		}
		// A deadline, because this handler runs on the store's goroutine and
		// every other verb in the session queues behind it. context.Background
		// and http.DefaultClient both wait for ever, so a geocoder that accepts
		// the connection and then says nothing froze the whole workbench with
		// no job in the list to cancel and nothing on screen to say why.
		//
		// Both halves are needed: the context bounds the call, and the client's
		// own timeout bounds a body that arrives one byte at a time.
		ctx, cancel := context.WithTimeout(context.Background(), searchDeadline)
		defer cancel()
		c := &worldb.Client{HTTP: &http.Client{Timeout: searchDeadline}}
		found, err := c.Search(ctx, q)
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

	st.HandleSpec("boundary.accept", state.Spec{
		What: "take one of the places the search offered into the study area, " +
			"which unions rather than replaces: Scotland and Ireland is two " +
			"accepts and then one prune, not two calls where the second wins",
		Params: []state.Param{
			{Name: "name", Type: state.ParamString, Required: true, Primary: true,
				What: "which of the offered places to take, matched without " +
					"regard to case and on any part of the offered name, so " +
					"\"Scotland\" takes \"Alba / Scotland\"; an empty one is " +
					"refused, and one that matches nothing is refused with the " +
					"list of what was offered"},
		},
		Returns: []string{"accepted", "areas"},
		Answers: "`accepted` is the gazetteer's own name for what was taken, " +
			"which is not always what was asked for, and `areas` how many the " +
			"study area now holds. Accepting the same place twice is answered " +
			"rather than refused, and does not stack it. This changes what is " +
			"measured, never what is loaded: the nodes outside stay until " +
			"boundary.prune.",
		Example: &state.Example{
			Params:   map[string]any{"name": "Fife"},
			What:     "add a searched place to the study area",
			Runnable: false,
		},
	}, func(w *state.World, p any) (any, error) {
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

	st.HandleSpec("boundary.remove", state.Spec{
		What: "take one area back out of a study area built from several, so a " +
			"wrong accept costs one call rather than starting the search over",
		Params: []state.Param{
			{Name: "name", Type: state.ParamString, Required: true, Primary: true,
				What: "the area to drop, matched whole and without regard to " +
					"case; an empty one is refused, and one that names no " +
					"accepted area is refused with the list of what there is"},
		},
		Returns: []string{"removed", "areas"},
		Answers: "`areas` is how many are left. Like accepting, this changes " +
			"what is measured and not what is loaded: nodes pruned away on the " +
			"old area do not come back.",
		Example: &state.Example{
			Params:   map[string]any{"name": "Fife"},
			What:     "drop an area from the study without starting again",
			Runnable: false,
		},
	}, func(w *state.World, p any) (any, error) {
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

	st.HandleSpec("boundary.prune", state.Spec{
		What: "delete the nodes outside the study area, keeping a margin " +
			"because a node just outside the border still relays to and " +
			"interferes with one just inside it and dropping it makes the mesh " +
			"behave better than reality",
		Params: []state.Param{
			{Name: "margin_km", Type: state.ParamNumber, Primary: true,
				What: "how far outside a node may be and still be kept; absent " +
					"or negative leaves it at the session's own margin"},
		},
		Returns: []string{"removed", "nodes"},
		Answers: "`nodes` is how many are left. Removing none is answered with " +
			"zero and touches nothing; removing any rebuilds the engine and " +
			"empties the link matrix while every remaining pair is measured " +
			"again. It is refused when no study area has been accepted, rather " +
			"than treated as an area that contains nothing.",
		Example: &state.Example{
			Params:   map[string]any{"margin_km": 15},
			What:     "cut an imported network down to the study area",
			Runnable: false,
		},
	}, func(w *state.World, p any) (any, error) {
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
