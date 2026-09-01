package nodeantenna

import (
	"fmt"
	"strings"

	"github.com/MeshBench/meshbench/internal/app/session"
	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/rf/geo"
	"github.com/MeshBench/meshbench/internal/world/scenario"
)

func registerNodeAntenna(st *state.Store, s *session.Sim) {
	registerRead(st, s)
	registerSet(st, s)
	registerAim(st, s)
}

func registerRead(st *state.Store, s *session.Sim) {
	st.Handle("node.antenna", func(_ *state.World, p any) (any, error) {
		name, _ := session.StringField(p, "node")
		if name == "" {
			return nil, session.BadParams("node.antenna needs a node")
		}
		i, ok := s.NodeIndex(name)
		if !ok {
			return nil, session.NoSuchNode(name)
		}
		return describe(s.Nodes()[i]), nil
	})
}

// registerSet is the one verb that changes an antenna, for one node or for as
// many as the filters leave.
//
// One verb rather than a per-node and a fleet-wide pair, because "every node"
// and "this node" differ only in a filter and two verbs would have to be kept
// agreeing about what a partial change means. It follows nodes.regions, which
// is the same shape for the same reason.
func registerSet(st *state.Store, s *session.Sim) {
	st.Handle("nodes.antenna", func(w *state.World, p any) (any, error) {
		only, _ := session.NamedField(p, "node")
		kind, _ := session.NamedField(p, "kind")
		if only != "" {
			if _, ok := s.NodeIndex(only); !ok {
				return nil, session.NoSuchNode(only)
			}
		}
		nodes := append([]scenario.Node(nil), s.Nodes()...)
		var last scenario.Node
		changed := 0
		for i := range nodes {
			if only != "" && nodes[i].Name != only {
				continue
			}
			if kind != "" && string(nodes[i].Kind) != kind {
				continue
			}
			m, err := overlay(nodes[i].Antenna, p)
			if err != nil {
				return nil, session.BadParams("nodes.antenna: %v", err)
			}
			nodes[i].Antenna = m
			last, changed = nodes[i], changed+1
		}
		if changed == 0 {
			return nil, fmt.Errorf(
				"no node is of kind %q, so there is nothing to put an antenna on", kind)
		}
		commit(st, s, w, nodes)
		w.Say(said(changed, last))
		out := describe(last)
		delete(out, "node")
		delete(out, "peak_dbi")
		out["nodes"] = changed
		return out, nil
	})
}

// registerAim turns a node's antenna towards another one.
//
// Bearing is a spatial quantity, and the question is almost never "what is the
// number": it is "point this at the mast on that hill". The arithmetic is two
// positions the scenario already holds, so making somebody read a bearing off a
// map and type it back is asking them to do a job the tool can do exactly.
func registerAim(st *state.Store, s *session.Sim) {
	st.Handle("node.aim", func(w *state.World, p any) (any, error) {
		name, _ := session.StringField(p, "node")
		at, _ := session.NamedField(p, "at")
		if name == "" || at == "" {
			return nil, session.BadParams("node.aim needs a node and an at")
		}
		from, ok := s.NodeIndex(name)
		if !ok {
			return nil, session.NoSuchNode(name)
		}
		to, ok := s.NodeIndex(at)
		if !ok {
			return nil, session.NoSuchNode(at)
		}
		if from == to {
			return nil, session.BadParams("%s cannot be aimed at itself", name)
		}
		a, b := s.Nodes()[from].Position, s.Nodes()[to].Position
		if a == b {
			// Two nodes at one point have no direction between them, and turning
			// an antenna to an arbitrary bearing would look like an answer.
			return nil, fmt.Errorf(
				"%s and %s are at the same position, so there is no bearing between them",
				name, at)
		}
		bearing := compass(geo.BearingDeg(a.Lat, a.Lon, b.Lat, b.Lon))
		nodes := append([]scenario.Node(nil), s.Nodes()...)
		nodes[from].Antenna.BearingDeg = bearing
		commit(st, s, w, nodes)
		// What it won, not just where it points: on an omni the answer is "the
		// same as before", and a control that reports success while changing
		// nothing is one somebody trusts once.
		gain := nodes[from].Antenna.GainAlongDBi(bearing)
		w.Say(fmt.Sprintf("%s points at %s, %.0f degrees, %.1f dBi that way",
			name, at, bearing, gain))
		return map[string]any{
			"node": name, "at": at, "bearing_deg": bearing,
			"distance_km": geo.DistanceKm(a.Lat, a.Lon, b.Lat, b.Lon),
			"gain_dbi":    gain,
		}, nil
	})
}

// commit rebuilds the engine over the changed nodes and re-warms the links.
//
// An antenna is physics, not a label: the engine caches each pair's look angles
// and gains, and it holds its own copy of every node's spec. Mutating the
// scenario in place would leave both answering about the antenna that used to
// be there, which is precisely the failure that makes an aiming control look
// like it does nothing.
func commit(st *state.Store, s *session.Sim, w *state.World, nodes []scenario.Node) {
	s.BuildSeeded(nodes, s.FreqMHz(), s.Seed())
	w.Nodes = session.StateNodes(nodes)
	w.Links = nil
	s.Warm(st, len(nodes))
}

// said is the sentence the interface prints, naming the node when there is one
// to name and counting them when there is not.
func said(changed int, last scenario.Node) string {
	shape, _ := describe(last)["pattern"].(string)
	what := strings.TrimSpace(fmt.Sprintf("%s at %.0f degrees",
		shape, last.Antenna.BearingDeg))
	if changed == 1 {
		return last.Name + ": " + what
	}
	return fmt.Sprintf("%d nodes now carry a %s", changed, what)
}
