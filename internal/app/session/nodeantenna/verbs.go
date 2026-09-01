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
	st.HandleSpec("node.antenna", state.Spec{
		What: "report what one node's antenna is and which way it points",
		Params: []state.Param{
			{Name: "node", Type: state.ParamString, Required: true, Primary: true,
				What: "the node to ask about"},
		},
		Returns: []string{"node", "pattern", "gain_dbi_peak", "beamwidth_deg",
			"front_to_back_db", "bearing_deg", "downtilt_deg", "polarisation",
			"feedline_db", "peak_dbi"},
		Answers: "The same words the verb that sets an antenna takes, so what " +
			"comes back can be handed straight back in. A node carrying no " +
			"antenna answers with an empty `pattern` and `peak_dbi` zero rather " +
			"than as an omni at 0 dBi, which in a table of numbers those two " +
			"would otherwise share. Both gain figures are the peak along " +
			"boresight; what a given link is actually worth is the pattern read " +
			"along the bearing to the far end, and differs in each direction.",
		Example: &state.Example{
			Params:   map[string]any{"node": "West Lomond"},
			What:     "ask what a node stands under and where it faces",
			Runnable: true,
		},
	}, func(_ *state.World, p any) (any, error) {
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
	st.HandleSpec("nodes.antenna", state.Spec{
		What: "choose and aim the antenna on one node, on a kind, or on every node",
		Params: []state.Param{
			{Name: "node", Type: state.ParamString,
				What: "one node; absent means every node the other filters leave"},
			{Name: "kind", Type: state.ParamString,
				What: "only nodes of this scenario kind"},
			{Name: "pattern", Type: state.ParamString,
				What: "isotropic, dipole, collinear or yagi"},
			{Name: "gain_dbi_peak", Type: state.ParamNumber,
				What: "the headline gain, for a collinear or a yagi"},
			{Name: "beamwidth_deg", Type: state.ParamNumber,
				What: "a yagi's horizontal half-power beamwidth"},
			{Name: "front_to_back_db", Type: state.ParamNumber,
				What: "how far down a yagi's back is on its front"},
			{Name: "bearing_deg", Type: state.ParamNumber,
				What: "compass bearing of boresight, 0 at north"},
			{Name: "downtilt_deg", Type: state.ParamNumber,
				What: "degrees the beam is tilted below the horizon"},
			{Name: "polarisation", Type: state.ParamString,
				What: "vertical, horizontal or circular"},
			{Name: "feedline_db", Type: state.ParamNumber,
				What: "cable and connector loss, as a positive number"},
		},
		Returns: []string{"nodes", "pattern", "gain_dbi_peak", "beamwidth_deg",
			"front_to_back_db", "bearing_deg", "downtilt_deg", "polarisation",
			"feedline_db"},
		Answers: "What the last matched node now carries, with `nodes` for how " +
			"many were changed and no node name, because the answer is about a " +
			"selection rather than one node. Each field named replaces one part " +
			"of the antenna already there, so a collinear switched to a yagi " +
			"keeps the gain figure somebody chose. Setting one rebuilds the " +
			"engine over the changed nodes and re-measures every link: the " +
			"cached look angles belong to the antenna that used to be there. " +
			"Refused where a named node is unknown, where a pattern or a " +
			"polarisation is not one the model prices, and where the filters " +
			"leave no node at all.",
		Example: &state.Example{
			Params: map[string]any{"node": "West Lomond", "pattern": "yagi",
				"gain_dbi_peak": 12, "beamwidth_deg": 45,
				"front_to_back_db": 20, "bearing_deg": 208},
			What:     "stand a yagi on the hill, facing down the Forth",
			Runnable: true,
		},
	}, func(w *state.World, p any) (any, error) {
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
	st.HandleSpec("node.aim", state.Spec{
		What: "turn a node's antenna towards another node, and say what that won it",
		Params: []state.Param{
			{Name: "node", Type: state.ParamString, Required: true, Primary: true,
				What: "the node whose antenna is turned"},
			{Name: "at", Type: state.ParamString, Required: true,
				What: "the node to point it at"},
		},
		Returns: []string{"node", "at", "bearing_deg", "distance_km", "gain_dbi"},
		Answers: "`bearing_deg` is the true bearing between the two positions " +
			"the scenario already holds, and `gain_dbi` is this node's pattern " +
			"read along it - which is the part worth reading, because on an omni " +
			"it is the figure it was before and a control that reports success " +
			"while changing nothing is one somebody trusts once. Only the named " +
			"node turns: what the far end hears back still depends on where its " +
			"own antenna points. Refused where either node is unknown, where a " +
			"node is aimed at itself, and where both stand at the same position, " +
			"which has no bearing between them.",
		Example: &state.Example{
			Params: map[string]any{"node": "West Lomond", "at": "Dunfermline"},
			What:   "point the hilltop repeater at the node it talks to",
		},
	}, func(w *state.World, p any) (any, error) {
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
