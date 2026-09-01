// The nodes around one node.
//
// Trimming an imported deployment to a neighbourhood is the first thing anybody
// does with one - 676 nodes is more firmware than a desktop will run - and
// every caller was writing its own haversine to do it. Written here once, on
// the same geo the path losses use, so a script's idea of "nearest" and the
// simulator's are the same idea.
package nodelookup

import (
	"sort"

	"github.com/MeshBench/meshbench/internal/app/session"
	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/rf/geo"
)

func registerNodesNear(st *state.Store) {
	st.Handle("nodes.near", func(w *state.World, p any) (any, error) {
		name, _ := session.StringField(p, "node")
		if name == "" {
			name = session.SoleString(p)
		}
		here, found := session.FindNode(w.Nodes, name)
		if !found {
			return nil, session.NoSuchNode(name)
		}
		count := 0
		if v, ok := session.NumField(p, "count"); ok && v > 0 {
			count = int(v)
		}

		type near struct {
			node state.Node
			km   float64
		}
		out := make([]near, 0, len(w.Nodes))
		for _, n := range w.Nodes {
			if n.Name == here.Name {
				continue
			}
			out = append(out, near{n, geo.DistanceKm(here.Lat, here.Lon, n.Lat, n.Lon)})
		}
		sort.SliceStable(out, func(i, j int) bool {
			if out[i].km != out[j].km {
				return out[i].km < out[j].km
			}
			// Name breaks a tie, so two nodes at one site come back in the
			// same order twice.
			return out[i].node.Name < out[j].node.Name
		})
		if count > 0 && len(out) > count {
			out = out[:count]
		}

		rows := make([]map[string]any, 0, len(out))
		for _, n := range out {
			rows = append(rows, map[string]any{
				"name": n.node.Name, "km": n.km, "kind": n.node.Kind,
				"lat": n.node.Lat, "lon": n.node.Lon,
			})
		}
		return map[string]any{"node": here.Name, "near": rows}, nil
	})
}
