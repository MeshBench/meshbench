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
	st.HandleSpec("link.pair", state.Spec{
		What: "answer why two particular places do or do not hear each other, " +
			"without the engine or a warm link matrix, which are exactly what " +
			"is missing at the moment somebody points at two masts and asks",
		Params: []state.Param{
			{Name: "a", Type: state.ParamObject, Required: true,
				What: "one end: a node's name as a bare string or as {node}, " +
					"or a place as {lat, lon} with an optional height_m that " +
					"defaults to 2 m head height; anything else is refused, as " +
					"is a name this network has not got"},
			{Name: "b", Type: state.ParamObject, Required: true,
				What: "the other end, in the same two forms; refused when it " +
					"labels the same place as a, since a link needs two"},
		},
		Returns: []string{"from", "to"},
		Answers: "It answers with the two labels as soon as the worker starts. " +
			"The cut-through and both margins arrive later through the internal " +
			"`link.pair_set`, and there are two margins because there are two " +
			"answers: each end's gain is evaluated on the bearing towards the " +
			"other, so A to B and B to A can differ by tens of decibels on a " +
			"beam. Both are best cases - bare earth, the calibrated excess " +
			"loss, a default noise floor and no multipath - which is what the " +
			"profile's assumption line says. A clicked place with no scenario " +
			"loaded is priced at 868 MHz, and says so.",
		Example: &state.Example{
			Params: map[string]any{"a": "West Lomond", "b": "Dunfermline"},
			What:   "ask why two repeaters do or do not hear each other",
		},
	}, func(w *state.World, p any) (any, error) {
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
		pairProfile(s, st, a, b)
		return map[string]any{"from": a.label, "to": b.label}, nil
	})

	st.HandleInternalSpec("link.pair_set", state.Spec{
		What: "take the finished cut-through and its two budgets into the " +
			"snapshot in one go, so the panel's picture and its margins are " +
			"always of the same pair and the same model",
		Returns: []string{"from", "to", "km", "edges"},
		Answers: "Answers nothing at all when the analysis could not run, " +
			"having cleared the profile: the reason has already been said on " +
			"the status line by the worker.",
	}, func(w *state.World, p any) (any, error) {
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
