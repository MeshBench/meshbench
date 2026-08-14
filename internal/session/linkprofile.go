// The cut-through: the ground between a pair, and what it costs.
//
// wb1's Link tab, behind verbs: pathview walks the path with the earth's
// curvature in it and terrain.Edges names the obstructions, so the picture
// says "that ridge at 4.2 km costs you 31 dB" rather than only a total.
package session

import (
	"context"
	"fmt"

	"github.com/MeshBench/meshbench/internal/gui/state"
	"github.com/MeshBench/meshbench/internal/pathview"
	"github.com/MeshBench/meshbench/internal/scenario"
	"github.com/MeshBench/meshbench/internal/terrain"
)

// profileFor computes the cut-through for a named pair on a worker and hands
// it to the store. Terrain reads are the cost, and the tiles are hot.
func (s *Sim) profileFor(st *state.Store, from, to string, atob, btoa float64) {
	var a, b *scenario.Node
	for i := range s.nodes {
		if s.nodes[i].Name == from {
			a = &s.nodes[i]
		}
		if s.nodes[i].Name == to {
			b = &s.nodes[i]
		}
	}
	if a == nil || b == nil || a == b {
		return
	}
	freq := s.freqMHz
	terr := s.terrain()
	go func() {
		ctx := context.Background()
		cut, err := pathview.Analyse(terr, a.Position.Lat, a.Position.Lon, a.HeightAGLm,
			b.Position.Lat, b.Position.Lon, b.HeightAGLm, freq, 256)
		if err != nil {
			_, _ = st.Do(ctx, "link.profile_set", (*state.Profile)(nil))
			return
		}
		p := &state.Profile{
			From: from, To: to, DistanceKm: cut.DistanceKm,
			AtoB: atob, BtoA: btoa, Verdict: cut.Verdict(),
		}
		p.LowM, p.HighM = cut.Extent()
		pts := make([]terrain.Point, 0, len(cut.Samples))
		for _, sm := range cut.Samples {
			p.Samples = append(p.Samples, state.ProfileSample{
				DistM: sm.DistM, BulgedM: sm.BulgedM,
				LOSm: sm.LOSm, FresnelM: sm.FresnelM,
			})
			pts = append(pts, terrain.Point{DistM: sm.DistM, HeightM: sm.GroundM})
		}
		// The obstructions, each with its own Bullington contribution. Only
		// the ones that cost anything worth a label.
		for _, e := range terrain.Edges(pts, a.HeightAGLm, b.HeightAGLm, freq) {
			if e.LossDB < 1 {
				continue
			}
			p.Edges = append(p.Edges, state.ProfileEdge{DistM: e.DistM, LossDB: e.LossDB})
			if len(p.Edges) == 6 {
				break
			}
		}
		_, _ = st.Do(ctx, "link.profile_set", p)
	}()
}

func registerLinkProfile(st *state.Store, s *Sim) {
	st.Handle("link.profile_set", func(w *state.World, p any) (any, error) {
		prof, _ := p.(*state.Profile)
		w.LinkProfile = prof
		if prof != nil {
			return map[string]any{"from": prof.From, "to": prof.To,
				"km": prof.DistanceKm, "edges": len(prof.Edges)}, nil
		}
		return nil, nil
	})

	// link.profile computes for the selected pair, for scripts and captures;
	// the interface reaches the same worker through budget.for_selection.
	st.Handle("link.profile", func(w *state.World, _ any) (any, error) {
		var sel []string
		for i := range w.Nodes {
			if w.Nodes[i].Selected {
				sel = append(sel, w.Nodes[i].Name)
			}
		}
		if len(sel) < 2 {
			return nil, fmt.Errorf("select two nodes to cut through between")
		}
		s.profileFor(st, sel[0], sel[len(sel)-1], 0, 0)
		return map[string]any{"from": sel[0], "to": sel[len(sel)-1]}, nil
	})
}
