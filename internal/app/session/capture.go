// Capture, radio presets, and placing a node.
//
// The last of the old socket's vocabulary that does not need the study engine.
package session

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/MeshBench/meshbench/internal/app/control"
	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/rf/geo"
	"github.com/MeshBench/meshbench/internal/world/scenario"
)

func registerCapture(st *state.Store, s *Sim) {
	// capture.file: the bytes, without a GUI.
	//
	// Diagnosing why a packet was not relayed needs the frame, not a window,
	// and on a driven session there is often nobody at the screen to look at
	// one.
	st.Handle("capture.file", func(w *state.World, p any) (any, error) {
		if s.eng == nil {
			return nil, fmt.Errorf("no network loaded")
		}
		path, _ := stringField(p, "path")
		if path == "" {
			path = filepath.Join(os.TempDir(), "meshcoresim-capture.pcapng")
		}
		if err := s.eng.StartCapture(path); err != nil {
			return nil, err
		}
		s.capturePath = path
		w.Say("capturing to " + path)
		return map[string]any{"path": path}, nil
	})

	// capture.wireshark: the same frames, streamed, with Wireshark opened on
	// them.
	//
	// Loopback, not the udpdump extcap - see wireshark.go's own doc for why
	// those are not interchangeable, however alike "port 5555" makes them
	// look. All three parts, because any one missing is the feature not
	// working: the stream Wireshark is actually capturing, both dissectors in
	// the order that makes them agree with each other, and Wireshark started.
	// It streams even when the last two fail, and says which of them did - a
	// capture running with no window is recoverable by hand, and knowing that
	// is the difference between a hint and a dead end.
	st.Handle("capture.wireshark", func(w *state.World, p any) (any, error) {
		if s.eng == nil {
			return nil, fmt.Errorf("no network loaded")
		}
		if err := s.eng.StartCaptureUDP(captureUDPAddr); err != nil {
			return nil, err
		}
		meshcoreLua, meshbenchLua := dissectorFiles()
		out := map[string]any{
			"addr": captureUDPAddr, "how": wiresharkHint(meshcoreLua, meshbenchLua),
		}
		switch {
		case meshbenchLua == "":
			// meshcoresim.lua is the one that registers on this port at all;
			// without it Wireshark shows raw UDP payload, not a missing
			// field here and there.
			out["dissector_error"] = "tools/dissector/meshcoresim.lua was not found beside the binary"
		case meshcoreLua == "":
			out["dissector_warning"] = "tools/dissector/meshcore_dissector.lua was not found - " +
				"MeshBench's own metadata will show, the MeshCore frame inside it will not"
		}

		bin := wiresharkBinary()
		if bin == "" {
			w.Say("streaming frames to " + captureUDPAddr + " - Wireshark is not installed, so run: " +
				wiresharkHint(meshcoreLua, meshbenchLua))
			out["launched"] = false
			return out, nil
		}
		if why := launchWireshark(bin, meshcoreLua, meshbenchLua); why != "" {
			w.Say("streaming to " + captureUDPAddr + ", but Wireshark would not start: " + why)
			out["launched"] = false
			out["launch_error"] = why
			return out, nil
		}
		out["launched"] = true
		s.captureLive = captureUDPAddr
		w.Say("Wireshark is opening on " + captureUDPAddr)
		return out, nil
	})

	st.Handle("capture.stop", func(w *state.World, _ any) (any, error) {
		if s.eng == nil {
			return nil, fmt.Errorf("no network loaded")
		}
		path, frames, err := s.eng.StopCapture()
		if err != nil {
			return nil, err
		}
		s.capturePath, s.captureLive = "", ""
		w.Say(fmt.Sprintf("captured %d frames", frames))
		return map[string]any{"path": path, "frames": frames}, nil
	})

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
		if b, ok := namedField(p, "board"); ok && b != "" {
			board, err := scenario.BoardByName(b)
			if err != nil {
				return nil, control.WithCode(control.BadParams, err)
			}
			node.Board = board.Name
		}
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
