// link.pair: the cut-through and both margins for exactly the pair asked
// about - two nodes, two places on the ground, or one of each.
//
// Deliberately independent of the engine. The engine's link table drops pairs
// whose weaker margin is far negative, and it does not exist at all before a
// warm - which are precisely the moments somebody points at two repeaters and
// asks "why don't these hear each other". This path always answers, from the
// same model the chart draws, and says what it assumed.
package links

import (
	"context"
	"fmt"
	"math"

	"github.com/MeshBench/meshbench/internal/app/session"
	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/rf/terrain"
	"github.com/MeshBench/meshbench/internal/study/linkbudget"
	"github.com/MeshBench/meshbench/internal/study/pathview"
	"github.com/MeshBench/meshbench/internal/world/scenario"
)

// pairEnd is one end of an asked-about link: a scenario node, or a synthetic
// mast standing at a clicked place.
type pairEnd struct {
	n     scenario.Node
	label string
}

// pairEndDefaultAGL is the mast a clicked place gets: head height, because
// the question a ground click asks is usually "could someone standing here
// reach that", and a height can be given explicitly when it is not.
const pairEndDefaultAGL = 2.0

// pairEndOf reads one endpoint from a verb's parameters: a node's name, or a
// map with lat and lon (and optionally height_m).
func pairEndOf(s *session.Sim, v any) (pairEnd, error) {
	if name, ok := v.(string); ok && name != "" {
		return nodeEnd(s, name)
	}
	m, ok := v.(map[string]any)
	if !ok {
		return pairEnd{}, fmt.Errorf("an endpoint is a node's name, or {lat, lon}")
	}
	if name, ok := session.StringField(m, "node"); ok && name != "" {
		return nodeEnd(s, name)
	}
	lat, okLat := session.NumField(m, "lat")
	lon, okLon := session.NumField(m, "lon")
	if !okLat || !okLon {
		return pairEnd{}, fmt.Errorf("an endpoint is a node's name, or {lat, lon}")
	}
	agl := pairEndDefaultAGL
	if v, ok := session.NumField(m, "height_m"); ok && v > 0 {
		agl = v
	}
	n := scenario.Node{
		Name:     fmt.Sprintf("%.4f, %.4f", lat, lon),
		Kind:     scenario.Kind("companion"),
		Position: scenario.LatLon{Lat: lat, Lon: lon},
	}
	n.HeightAGLm = agl
	n.TxPowerDBm = 22
	// The mast radios what the mesh radios, taken from the nearest real
	// node, so its margin is about the ground rather than about a spreading
	// factor nobody chose.
	if near := nearestNode(s, lat, lon); near != nil {
		n.Radio = near.Radio
		n.TxPowerDBm = near.TxPowerDBm
	}
	return pairEnd{n: n, label: n.Name}, nil
}

func nodeEnd(s *session.Sim, name string) (pairEnd, error) {
	for i := range s.Nodes() {
		if s.Nodes()[i].Name == name {
			return pairEnd{n: s.Nodes()[i], label: name}, nil
		}
	}
	return pairEnd{}, fmt.Errorf("no node called %q", name)
}

// nearestNode is the closest scenario node to a place, or nil with none.
func nearestNode(s *session.Sim, lat, lon float64) *scenario.Node {
	best, bestD := -1, math.Inf(1)
	for i := range s.Nodes() {
		dLat := s.Nodes()[i].Position.Lat - lat
		dLon := s.Nodes()[i].Position.Lon - lon
		if d := dLat*dLat + dLon*dLon; d < bestD {
			best, bestD = i, d
		}
	}
	if best < 0 {
		return nil
	}
	return &s.Nodes()[best]
}

// pairResult is what the worker hands back to the store goroutine: the
// picture and the budgets, together, so they cannot disagree.
type pairResult struct {
	profile *state.Profile
	budgets []state.Budget
}

// pairProfile computes the cut-through and both margins for a pair on a
// worker. Everything the worker needs is captured here, on the store's
// goroutine, so it reads no Sim state of its own.
func pairProfile(s *session.Sim, st *state.Store, a, b pairEnd) {
	freq := s.FreqMHz()
	assumedFreq := ""
	if freq <= 0 {
		// Two places can be asked about before any scenario is loaded; the
		// band is assumed and said, rather than refused.
		freq = 868
		assumedFreq = ", 868 MHz assumed"
	}
	terr := s.Terrain()
	excess := s.ExcessLossDB()
	go func() {
		ctx := context.Background()
		cut, err := pathview.Analyse(terr,
			a.n.Position.Lat, a.n.Position.Lon, a.n.HeightAGLm,
			b.n.Position.Lat, b.n.Position.Lon, b.n.HeightAGLm, freq, 256)
		if err != nil {
			_, _ = st.Do(ctx, "link.pair_set", (*pairResult)(nil))
			_, _ = st.Do(ctx, "ui.said", "link: "+err.Error())
			return
		}
		pts := make([]terrain.Point, 0, len(cut.Samples))
		for _, sm := range cut.Samples {
			pts = append(pts, terrain.Point{DistM: sm.DistM, HeightM: sm.GroundM})
		}
		// The margins come from exactly the model the chart draws - free
		// space plus the Bullington edges plus the calibrated excess - so
		// the picture and the numbers cannot tell different stories.
		loss := terrain.FSPLdB(cut.DistanceKm, freq) +
			terrain.MultiEdgeLossDB(pts, a.n.HeightAGLm, b.n.HeightAGLm, freq) +
			excess
		ab := linkbudget.OneWayDB(a.n, b.n, loss)
		ba := linkbudget.OneWayDB(b.n, a.n, loss)
		p := session.StateProfile(cut, a.label, b.label, ab, ba,
			a.n.HeightAGLm, b.n.HeightAGLm, freq)
		p.Assumed = fmt.Sprintf(
			"bare earth + %.1f dB excess, default noise floor%s", excess, assumedFreq)
		_, _ = st.Do(ctx, "link.pair_set", &pairResult{
			profile: p,
			budgets: []state.Budget{
				{From: a.label, To: b.label, MarginDB: ab,
					Terms: session.TermsOf(linkbudget.Terms(a.n, b.n, loss))},
				{From: b.label, To: a.label, MarginDB: ba,
					Terms: session.TermsOf(linkbudget.Terms(b.n, a.n, loss))},
			},
		})
	}()
}

func registerLinkPair(st *state.Store, s *session.Sim) {
	st.Handle("link.pair", func(w *state.World, p any) (any, error) {
		m, ok := p.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("link.pair needs {a, b} endpoints")
		}
		a, err := pairEndOf(s, m["a"])
		if err != nil {
			return nil, err
		}
		b, err := pairEndOf(s, m["b"])
		if err != nil {
			return nil, err
		}
		if a.label == b.label {
			return nil, fmt.Errorf("both ends are %s - a link needs two places", a.label)
		}
		// The ground between these two ends, not the network's: this verb
		// answers about places nobody has put a node on, and the cut-through it
		// draws is only a cut through anything if the tiles under it are here.
		//
		// Said rather than refused, unlike the rasters: this path exists to
		// answer before a warm has happened, and the profile it draws is
		// visibly flat when there is nothing under it.
		under := s.GroundUnder([]scenario.Node{a.n, b.n})
		session.NoteGround(w, "link.pair", under)
		pairProfile(s, st, a, b)
		// The two labels and the ground, and a note saying where the rest of
		// the answer lands: the cut-through and both margins are computed on
		// a worker, because the terrain can reach the network, so they cannot
		// come back in this reply. Same shape as console.type and
		// console.read.
		return map[string]any{"from": a.label, "to": b.label,
			"ground": under.Map(),
			"note":   "the cut-through and both margins land when the worker finishes; read them with link.result",
		}, nil
	})

	st.HandleInternal("link.pair_set", func(w *state.World, p any) (any, error) {
		r, ok := p.(*pairResult)
		if !ok {
			return nil, session.WrongCallback("link.pair_set")
		}
		if r == nil {
			w.LinkProfile = nil
			return nil, nil
		}
		w.LinkProfile = r.profile
		w.Budgets = r.budgets
		return map[string]any{"from": r.profile.From, "to": r.profile.To,
			"km": r.profile.DistanceKm, "edges": len(r.profile.Edges)}, nil
	})
}

// registerLinkResult adds the read half of link.pair.
//
// link.pair starts a worker and returns the two labels: the cut-through and
// both margins land in the snapshot through the internal link.pair_set, where
// only a panel could reach them. So the question the verb exists to answer -
// why do these two hear each other, or not - could be asked from a script and
// not read back. Both directions especially: a reply that does not say which
// direction is wrong even when the arithmetic is right.
//
// A separate read rather than a synchronous link.pair, because the analysis
// asks the terrain and the terrain can reach the network. Same shape as
// console.type and console.read, and link.pair's note now says so.
func registerLinkResult(st *state.Store, _ *session.Sim) {
	st.Handle("link.result", func(w *state.World, _ any) (any, error) {
		p := w.LinkProfile
		if p == nil {
			return nil, fmt.Errorf(
				"no link has been analysed yet: ask for one with link.pair or " +
					"link.profile, then read it here")
		}
		out := map[string]any{
			"from": p.From, "to": p.To, "km": p.DistanceKm,
			// The two margins from the profile itself, which always agree
			// with its ends. w.Budgets is written by link.pair and not by
			// link.profile, so it can be empty after the selection route and
			// stale after a different pair - and a breakdown for one link
			// under the headline of another is two answers reported as one.
			"a_to_b_db": p.AtoB, "b_to_a_db": p.BtoA,
			"verdict": p.Verdict, "assumed": p.Assumed,
			"edges": len(p.Edges), "samples": len(p.Samples),
			"worst_at_km": p.Worst.DistM / 1000,
		}
		if dirs := budgetsFor(p, w.Budgets); dirs != nil {
			out["directions"] = dirs
		} else {
			out["note"] = "the per-term breakdown is computed by link.pair; " +
				"the two margins above are the profile's own"
		}
		return out, nil
	})
}

// budgetsFor is the per-direction breakdown for exactly this profile, or nil
// where what is held is for some other pair or for none.
func budgetsFor(p *state.Profile, budgets []state.Budget) []map[string]any {
	if len(budgets) != 2 {
		return nil
	}
	ends := map[string]bool{p.From: true, p.To: true}
	for _, b := range budgets {
		if !ends[b.From] || !ends[b.To] || b.From == b.To {
			return nil
		}
	}
	dirs := make([]map[string]any, 0, 2)
	for _, b := range budgets {
		terms := make([]map[string]any, 0, len(b.Terms))
		for _, t := range b.Terms {
			terms = append(terms, map[string]any{"name": t.Name, "db": t.DB})
		}
		dirs = append(dirs, map[string]any{
			"from": b.From, "to": b.To, "margin_db": b.MarginDB, "terms": terms,
		})
	}
	return dirs
}
