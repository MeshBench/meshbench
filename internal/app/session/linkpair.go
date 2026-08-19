// link.pair: the cut-through and both margins for exactly the pair asked
// about - two nodes, two places on the ground, or one of each.
//
// Deliberately independent of the engine. The engine's link table drops pairs
// whose weaker margin is far negative, and it does not exist at all before a
// warm - which are precisely the moments somebody points at two repeaters and
// asks "why don't these hear each other". This path always answers, from the
// same model the chart draws, and says what it assumed.
package session

import (
	"context"
	"fmt"
	"math"

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
func (s *Sim) pairEndOf(v any) (pairEnd, error) {
	if name, ok := v.(string); ok && name != "" {
		return s.nodeEnd(name)
	}
	m, ok := v.(map[string]any)
	if !ok {
		return pairEnd{}, fmt.Errorf("an endpoint is a node's name, or {lat, lon}")
	}
	if name, ok := stringField(m, "node"); ok && name != "" {
		return s.nodeEnd(name)
	}
	lat, okLat := numField(m, "lat")
	lon, okLon := numField(m, "lon")
	if !okLat || !okLon {
		return pairEnd{}, fmt.Errorf("an endpoint is a node's name, or {lat, lon}")
	}
	agl := pairEndDefaultAGL
	if v, ok := numField(m, "height_m"); ok && v > 0 {
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
	if near := s.nearestNode(lat, lon); near != nil {
		n.Radio = near.Radio
		n.TxPowerDBm = near.TxPowerDBm
	}
	return pairEnd{n: n, label: n.Name}, nil
}

func (s *Sim) nodeEnd(name string) (pairEnd, error) {
	for i := range s.nodes {
		if s.nodes[i].Name == name {
			return pairEnd{n: s.nodes[i], label: name}, nil
		}
	}
	return pairEnd{}, fmt.Errorf("no node called %q", name)
}

// nearestNode is the closest scenario node to a place, or nil with none.
func (s *Sim) nearestNode(lat, lon float64) *scenario.Node {
	best, bestD := -1, math.Inf(1)
	for i := range s.nodes {
		dLat := s.nodes[i].Position.Lat - lat
		dLon := s.nodes[i].Position.Lon - lon
		if d := dLat*dLat + dLon*dLon; d < bestD {
			best, bestD = i, d
		}
	}
	if best < 0 {
		return nil
	}
	return &s.nodes[best]
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
func (s *Sim) pairProfile(st *state.Store, a, b pairEnd) {
	freq := s.freqMHz
	assumedFreq := ""
	if freq <= 0 {
		// Two places can be asked about before any scenario is loaded; the
		// band is assumed and said, rather than refused.
		freq = 868
		assumedFreq = ", 868 MHz assumed"
	}
	terr := s.terrain()
	excess := s.excessLossDB
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
		p := stateProfile(cut, a.label, b.label, ab, ba,
			a.n.HeightAGLm, b.n.HeightAGLm, freq)
		p.Assumed = fmt.Sprintf(
			"bare earth + %.1f dB excess, default noise floor%s", excess, assumedFreq)
		_, _ = st.Do(ctx, "link.pair_set", &pairResult{
			profile: p,
			budgets: []state.Budget{
				{From: a.label, To: b.label, MarginDB: ab,
					Terms: termsOf(linkbudget.Terms(a.n, b.n, loss))},
				{From: b.label, To: a.label, MarginDB: ba,
					Terms: termsOf(linkbudget.Terms(b.n, a.n, loss))},
			},
		})
	}()
}

func registerLinkPair(st *state.Store, s *Sim) {
	st.Handle("link.pair", func(w *state.World, p any) (any, error) {
		m, ok := p.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("link.pair needs {a, b} endpoints")
		}
		a, err := s.pairEndOf(m["a"])
		if err != nil {
			return nil, err
		}
		b, err := s.pairEndOf(m["b"])
		if err != nil {
			return nil, err
		}
		if a.label == b.label {
			return nil, fmt.Errorf("both ends are %s - a link needs two places", a.label)
		}
		s.pairProfile(st, a, b)
		return map[string]any{"from": a.label, "to": b.label}, nil
	})

	st.Handle("link.pair_set", func(w *state.World, p any) (any, error) {
		r, _ := p.(*pairResult)
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
