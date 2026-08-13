// Capture, radio presets, and placing a node.
//
// The last of the old socket's vocabulary that does not need the study engine.
package session

import (
	"fmt"
	"math"
	"os"
	"path/filepath"

	"github.com/A13xB0/meshcoresim/internal/gui/state"
	"github.com/A13xB0/meshcoresim/internal/scenario"
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

	// capture.wireshark: the same frames, streamed, for somebody watching.
	st.Handle("capture.wireshark", func(w *state.World, p any) (any, error) {
		if s.eng == nil {
			return nil, fmt.Errorf("no network loaded")
		}
		addr, _ := stringField(p, "addr")
		if addr == "" {
			addr = "127.0.0.1:19000"
		}
		if err := s.eng.StartCaptureUDP(addr); err != nil {
			return nil, err
		}
		w.Say("streaming frames to " + addr)
		return map[string]any{
			"addr": addr,
			"how":  "wireshark -k -i udpdump, or udpdump on " + addr,
		}, nil
	})

	st.Handle("capture.stop", func(w *state.World, _ any) (any, error) {
		if s.eng == nil {
			return nil, fmt.Errorf("no network loaded")
		}
		path, frames, err := s.eng.StopCapture()
		if err != nil {
			return nil, err
		}
		s.capturePath = ""
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
		only, _ := stringField(p, "node")
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
		kind, _ := stringField(p, "kind")
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
		// A placed node holds what its neighbours hold. Not the busiest
		// region in the fixture: on a mesh spanning two islands that would
		// give a repeater in Edinburgh the region held in Wicklow, and a node
		// holding a region its neighbours do not is as silent as one holding
		// none.
		node.Regions = regionsOfNeighbours(s.nodes, lat, lon)
		nodes := append(append([]scenario.Node(nil), s.nodes...), node)
		s.buildSeeded(nodes, s.freqMHz, s.seed)
		w.Nodes = stateNodes(nodes)
		w.Links = s.links()
		w.Say("placed " + name)
		return map[string]any{
			"placed": name, "kind": kind, "regions": node.Regions,
			"nodes": len(nodes),
		}, nil
	})
}

// regionsOfNeighbours is what a majority of the ten nearest nodes hold.
func regionsOfNeighbours(nodes []scenario.Node, lat, lon float64) []string {
	type near struct {
		d float64
		n scenario.Node
	}
	var all []near
	for _, n := range nodes {
		all = append(all, near{kmBetween(lat, lon, n.Position.Lat, n.Position.Lon), n})
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

// kmBetween is the great-circle distance, for choosing neighbours.
func kmBetween(lat1, lon1, lat2, lon2 float64) float64 {
	const r = 6371.0
	p := math.Pi / 180
	a := 0.5 - math.Cos((lat2-lat1)*p)/2 +
		math.Cos(lat1*p)*math.Cos(lat2*p)*(1-math.Cos((lon2-lon1)*p))/2
	return 2 * r * math.Asin(math.Sqrt(a))
}
