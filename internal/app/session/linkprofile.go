// The cut-through: the ground between a pair, and what it costs.
//
// wb1's Link tab, behind verbs: pathview walks the path with the earth's
// curvature in it and terrain.Edges names the obstructions, so the picture
// says "that ridge at 4.2 km costs you 31 dB" rather than only a total.
package session

import (
	"context"

	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/rf/terrain"
	"github.com/MeshBench/meshbench/internal/study/pathview"
	"github.com/MeshBench/meshbench/internal/world/scenario"
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
		p := stateProfile(cut, from, to, atob, btoa, a.HeightAGLm, b.HeightAGLm, freq)
		_, _ = st.Do(ctx, "link.profile_set", p)
	}()
}

// stateProfile turns a cut-through into what the snapshot carries: the
// samples with their bare ground, the labelled edges, and the sample that
// decides the verdict.
func stateProfile(cut pathview.CutThrough, from, to string,
	atob, btoa, aAGL, bAGL, freq float64) *state.Profile {
	p := &state.Profile{
		From: from, To: to, DistanceKm: cut.DistanceKm,
		AtoB: atob, BtoA: btoa, Verdict: cut.Verdict(),
	}
	p.LowM, p.HighM = cut.Extent()
	pts := make([]terrain.Point, 0, len(cut.Samples))
	for _, sm := range cut.Samples {
		p.Samples = append(p.Samples, state.ProfileSample{
			DistM: sm.DistM, GroundM: sm.GroundM, BulgedM: sm.BulgedM,
			LOSm: sm.LOSm, FresnelM: sm.FresnelM,
		})
		pts = append(pts, terrain.Point{DistM: sm.DistM, HeightM: sm.GroundM})
	}
	if cut.Worst >= 0 && cut.Worst < len(cut.Samples) {
		w := cut.Samples[cut.Worst]
		p.Worst = state.ProfileWorst{
			DistM: w.DistM, ClearanceM: w.ClearanceM,
			FresnelPct: cut.WorstF1Pct, Blocked: cut.Blocked,
		}
	}
	// The obstructions, each with its own Bullington contribution. Only
	// the ones that cost anything worth a label.
	for _, e := range terrain.Edges(pts, aAGL, bAGL, freq) {
		if e.LossDB < 1 {
			continue
		}
		p.Edges = append(p.Edges, state.ProfileEdge{DistM: e.DistM, LossDB: e.LossDB})
		if len(p.Edges) == 6 {
			break
		}
	}
	return p
}
