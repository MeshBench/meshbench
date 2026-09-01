// Radio presets and placing a node - the socket vocabulary that stays in core.
package session

import (
	"fmt"

	"github.com/MeshBench/meshbench/internal/app/control"
	"github.com/MeshBench/meshbench/internal/app/state"
	hw "github.com/MeshBench/meshbench/internal/firmware/board"
	"github.com/MeshBench/meshbench/internal/rf/geo"
	"github.com/MeshBench/meshbench/internal/world/scenario"
)

func registerPresetsAndPlace(st *state.Store, s *Sim) {
	// radio.preset: the modem configuration, which decides sensitivity and
	// airtime and therefore every number downstream of them.
	st.Handle("radio.preset", func(w *state.World, p any) (any, error) {
		label, _ := stringField(p, "preset")
		if label == "" {
			var have []string
			for _, pr := range scenario.RadioPresets {
				have = append(have, pr.Label)
			}
			return map[string]any{"presets": have}, nil
		}
		preset, ok := scenario.PresetByLabel(label)
		if !ok {
			return nil, fmt.Errorf("no radio preset %q", label)
		}
		only, _ := namedField(p, "node")
		n := 0
		for i := range s.nodes {
			if only != "" && s.nodes[i].Name != only {
				continue
			}
			s.nodes[i].Radio = scenario.RadioConfig{
				CentreHz:     preset.FreqMHz * 1e6,
				BandwidthHz:  preset.BwKHz * 1e3,
				SpreadFactor: preset.SF,
				CodingRate:   preset.CR,
			}
			n++
		}
		if n > 0 && len(s.nodes) > 0 {
			// The engine caches path loss per pair and a preset changes the
			// frequency, so the cache is answering about a different radio.
			s.buildSeeded(s.nodes, preset.FreqMHz, s.seed)
			w.Links = nil
			s.warm(st, len(s.nodes))
		}
		w.Say(fmt.Sprintf("%d nodes on %s", n, label))
		return map[string]any{"preset": label, "nodes": n}, nil
	})

	// nodes.place: put one down, which is how the four kinds an import never
	// contains get into a scenario.
	st.Handle("nodes.place", func(w *state.World, p any) (any, error) {
		name, _ := stringField(p, "name")
		kind, _ := namedField(p, "kind")
		lat, okLat := numField(p, "lat")
		lon, okLon := numField(p, "lon")
		if name == "" || !okLat || !okLon {
			return nil, fmt.Errorf("nodes.place needs a name, a lat and a lon")
		}
		if kind == "" {
			kind = string(scenario.SimpleRepeater)
		}
		for _, n := range s.nodes {
			if n.Name == name {
				return nil, fmt.Errorf("there is already a node called %q", name)
			}
		}
		node := scenario.Node{
			Name:     name,
			Kind:     scenario.Kind(kind),
			Position: scenario.LatLon{Lat: lat, Lon: lon},
		}
		node.HeightAGLm = 10
		node.TxPowerDBm = 22
		if v, ok := numField(p, "height_m"); ok {
			node.HeightAGLm = v
		}
		if v, ok := numField(p, "tx_dbm"); ok {
			node.TxPowerDBm = v
		}
		// The hardware it is, if it was said.
		//
		// Missing until now, which meant a node could be placed and could not
		// be made a T-Deck without going through the interface - so a script
		// could build a mesh and not build the one it wanted. The board is
		// what decides the transmit ceiling, the receive chain's noise figure
		// and the battery the energy model needs, so a wrong name has to
		// refuse rather than fall back to a plausible default.
		// And the antenna it stands under. A placed node had none at all, which
		// is not "an omni": the engine credited it zero gain, the map drew no
		// pattern beside it, and the coverage raster dereferenced it. The board's
		// own answer, the same one an imported node on that board gets, so a
		// mesh built by hand and a mesh built by import are priced alike.
		profile := hw.Board{}
		if b, ok := namedField(p, "board"); ok && b != "" {
			board, err := hw.BoardByName(b)
			if err != nil {
				return nil, control.WithCode(control.BadParams, err)
			}
			node.Board, profile = board.Name, board
		}
		node.Antenna = scenario.BoardAntenna(profile)
		// A placed node holds what its neighbours hold. Not the busiest
		// region in the fixture: on a mesh spanning two islands that would
		// give a repeater in Edinburgh the region held in Wicklow, and a node
		// holding a region its neighbours do not is as silent as one holding
		// none.
		node.Regions = regionsOfNeighbours(s.nodes, lat, lon)
		// And it runs what its mesh runs. A placed repeater with no build
		// pinned refused the whole real-firmware run - correctly, but the
		// operator who just dropped a node on the map was not choosing a
		// firmware strategy, they were adding a repeater to this network.
		node.Firmware = firmwareOfNeighbours(s.nodes, node.Kind)
		nodes := append(append([]scenario.Node(nil), s.nodes...), node)
		s.buildSeeded(nodes, s.freqMHz, s.seed)
		w.Nodes = stateNodes(nodes)
		// Not s.links() here: that measures every pair, inline, on the
		// store's goroutine - 48,000 terrain profiles on a national network,
		// with the whole application waiting on it. The warm publishes them
		// when it has them.
		s.warm(st, len(nodes))
		w.Say("placed " + name)
		return map[string]any{
			"placed": name, "kind": kind, "regions": node.Regions,
			"board": node.Board, "nodes": len(nodes),
		}, nil
	})
}

// firmwareOfNeighbours is the build most nodes of the same application
// already run - a placed node joins this mesh, not an abstract one. A mesh
// with no such nodes leaves the ref empty, and sim.start's own message
// says what to pin.
func firmwareOfNeighbours(nodes []scenario.Node, kind scenario.Kind) scenario.FirmwareRef {
	app := kind.Application()
	if app == "" {
		return scenario.FirmwareRef{}
	}
	counts := map[scenario.FirmwareRef]int{}
	for _, n := range nodes {
		if n.Kind.Application() != app || n.Firmware.Version == "" {
			continue
		}
		ref := n.Firmware
		ref.Board = "" // the host build; the operator can emulate later
		counts[ref]++
	}
	var best scenario.FirmwareRef
	bestN := 0
	for ref, n := range counts {
		// Deterministic tie-break by version then role, because map order
		// must never pick a node's firmware.
		if n > bestN || (n == bestN && (ref.Version > best.Version ||
			(ref.Version == best.Version && ref.Role > best.Role))) {
			best, bestN = ref, n
		}
	}
	return best
}

// regionsOfNeighbours is what a majority of the ten nearest nodes hold.
func regionsOfNeighbours(nodes []scenario.Node, lat, lon float64) []string {
	type near struct {
		d float64
		n scenario.Node
	}
	var all []near
	for _, n := range nodes {
		all = append(all, near{geo.DistanceKm(lat, lon, n.Position.Lat, n.Position.Lon), n})
	}
	for i := 1; i < len(all); i++ {
		for j := i; j > 0 && all[j].d < all[j-1].d; j-- {
			all[j], all[j-1] = all[j-1], all[j]
		}
	}
	if len(all) > 10 {
		all = all[:10]
	}
	count := map[string]int{}
	for _, a := range all {
		for _, r := range a.n.Regions {
			count[r]++
		}
	}
	var out []string
	for r, c := range count {
		if c*2 > len(all) {
			out = append(out, r)
		}
	}
	return out
}
