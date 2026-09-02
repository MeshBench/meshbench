// The map's own verbs: where the camera looks, what is drawn under it, and
// what a click on it does.
//
// Split out of ui.go, which held every interface verb and outgrew the file
// limit once each of them said what it was for.
package session

import (
	"fmt"

	"github.com/MeshBench/meshbench/internal/app/state"
)

func registerMapCamera(st *state.Store, s *Sim) {
	// map.centre: look at a place, or at a node.
	//
	// A name is accepted as well as a position because a caller aiming a
	// capture knows "Bishop Hill" and would otherwise have to look its
	// coordinates up first.
	st.Handle("map.centre", func(w *state.World, p any) (any, error) {
		if err := s.needUI(); err != nil {
			return nil, err
		}
		var lat, lon, zoom float64
		if name := primaryString(p, "node"); name != "" {
			n, found := findNode(w.Nodes, name)
			if !found {
				return nil, noSuchNode(name)
			}
			lat, lon = n.Lat, n.Lon
		}
		// Named, not bare: the node is this verb's one bare parameter, so a
		// bare number read as the latitude would be read as the longitude and
		// the zoom as well, and the camera would go to a place nobody named.
		if v, ok := namedNum(p, "lat"); ok {
			lat = v
		}
		if v, ok := namedNum(p, "lon"); ok {
			lon = v
		}
		if v, ok := namedNum(p, "zoom"); ok {
			zoom = v
		}
		if lat == 0 && lon == 0 {
			return nil, fmt.Errorf("map.centre needs a node, or a lat and lon")
		}
		s.ui.CentreMap(lat, lon, zoom)
		return map[string]any{"lat": lat, "lon": lon, "zoom": zoom}, nil
	})

	st.Handle("map.fit", func(w *state.World, _ any) (any, error) {
		if err := s.needUI(); err != nil {
			return nil, err
		}
		s.ui.FitMap()
		return map[string]any{"nodes": len(w.Nodes)}, nil
	})
}

// registerMapView is what is drawn on the map and what a click on it means.
func registerMapView(st *state.Store, s *Sim) {
	need := func() error { return s.needUI() }

	st.Handle("map.zoom", func(_ *state.World, p any) (any, error) {
		if err := need(); err != nil {
			return nil, err
		}
		f := 2.0
		if v, ok := numField(p, "factor"); ok && v > 0 {
			f = v
		}
		s.ui.ZoomMap(f)
		return map[string]any{"factor": f}, nil
	})

	st.Handle("map.filter", func(w *state.World, p any) (any, error) {
		if err := need(); err != nil {
			return nil, err
		}
		q, _ := stringField(p, "query")
		s.ui.FilterMap(q)
		w.Say("map filter: " + q)
		return map[string]any{"query": q}, nil
	})

	st.Handle("tool.set", func(w *state.World, p any) (any, error) {
		if err := need(); err != nil {
			return nil, err
		}
		name, _ := stringField(p, "name")
		if err := s.ui.SetTool(name); err != nil {
			return nil, err
		}
		w.Say("tool: " + name)
		return map[string]any{"tool": name}, nil
	})

	// map.layer: draw this, or stop drawing it.
	st.Handle("map.layer", func(_ *state.World, p any) (any, error) {
		if err := need(); err != nil {
			return nil, err
		}
		name, _ := stringField(p, "name")
		if name == "" {
			name = soleString(p)
		}
		on := true
		if v, ok := boolField(p, "on"); ok {
			on = v
		}
		if err := s.ui.SetLayer(name, on); err != nil {
			return nil, err
		}
		return map[string]any{"layers": s.ui.Layers()}, nil
	})

	// map.layers: what the map is drawing.
	st.Handle("map.layers", func(_ *state.World, _ any) (any, error) {
		if err := need(); err != nil {
			return nil, err
		}
		return map[string]any{"layers": s.ui.Layers()}, nil
	})
}
