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
	st.HandleSpec("map.centre", state.Spec{
		What: "Point the map at a node, or at a latitude and longitude.",
		Params: []state.Param{
			{Name: "node", Type: state.ParamString, Primary: true,
				What: "centre on this node instead of giving coordinates"},
			{Name: "lat", Type: state.ParamNumber, What: "degrees north"},
			{Name: "lon", Type: state.ParamNumber, What: "degrees east"},
			{Name: "zoom", Type: state.ParamNumber, What: "zoom level; unchanged when absent"},
		},
		Returns: []string{"lat", "lon", "zoom"},
		Answers: "The position the camera was sent to, which for a node is that " +
			"node's own, and `zoom` is zero where none was asked for rather " +
			"than the zoom the map is on. The camera moves on the next frame, " +
			"so the answer is what was asked for, not what is drawn yet. " +
			"Refuses a node it cannot find, refuses a call that leaves both " +
			"coordinates at zero, and refuses when no interface is attached.",
		Example: &state.Example{
			Params: map[string]any{"node": "West Lomond", "zoom": 12},
			What:   "frame one repeater before a screenshot",
		},
	}, func(w *state.World, p any) (any, error) {
		if err := s.needUI(); err != nil {
			return nil, err
		}
		var lat, lon, zoom float64
		if name := soleString(p); name != "" {
			n, found := findNode(w.Nodes, name)
			if !found {
				return nil, noSuchNode(name)
			}
			lat, lon = n.Lat, n.Lon
		} else {
			if v, ok := numField(p, "node"); !ok {
				_ = v
			}
			if m, ok := p.(map[string]any); ok {
				if name, ok := m["node"].(string); ok && name != "" {
					n, found := findNode(w.Nodes, name)
					if !found {
						return nil, noSuchNode(name)
					}
					lat, lon = n.Lat, n.Lon
				}
			}
			if v, ok := numField(p, "lat"); ok {
				lat = v
			}
			if v, ok := numField(p, "lon"); ok {
				lon = v
			}
			if v, ok := numField(p, "zoom"); ok {
				zoom = v
			}
		}
		if lat == 0 && lon == 0 {
			return nil, fmt.Errorf("map.centre needs a node, or a lat and lon")
		}
		s.ui.CentreMap(lat, lon, zoom)
		return map[string]any{"lat": lat, "lon": lon, "zoom": zoom}, nil
	})

	st.HandleSpec("map.fit", state.Spec{
		What:    "Zoom the map so every node is on it.",
		Returns: []string{"nodes"},
		Answers: "`nodes` is how many the camera was framed around, so zero " +
			"means an empty network and a camera that has not moved. Refuses " +
			"when no interface is attached.",
		Example: &state.Example{
			Params: map[string]any{}, What: "get the whole network back on screen",
		},
	}, func(w *state.World, _ any) (any, error) {
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

	st.HandleSpec("map.zoom", state.Spec{
		What: "zoom the map in or out from wherever it is now, for a caller " +
			"that wants a step rather than a stated scale",
		Params: []state.Param{
			{Name: "factor", Type: state.ParamNumber, Primary: true,
				What: "what to multiply the current scale by, so above one is " +
					"closer and below one is further out; anything not a " +
					"positive number leaves it at two"},
		},
		Returns: []string{"factor"},
		Answers: "The factor applied, not the zoom level reached: this " +
			"multiplies whatever the map is on, and nothing here knows what " +
			"that was. A caller that needs a known scale gives `map.centre` a " +
			"zoom instead. Refuses when no interface is attached.",
		Example: &state.Example{
			Params: map[string]any{"factor": 0.5}, What: "pull back to twice the ground",
		},
	}, func(_ *state.World, p any) (any, error) {
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

	st.HandleSpec("map.filter", state.Spec{
		What: "dim everything on the map that does not match some text, which " +
			"is how one network's worth of nodes is read as a handful",
		Params: []state.Param{
			{Name: "query", Type: state.ParamString, Primary: true,
				What: "the text to match; empty or absent clears the filter and " +
					"everything is drawn again"},
		},
		Returns: []string{"query"},
		Answers: "The filter dims what does not match rather than removing it, " +
			"so nothing is hidden and the count of nodes does not change. " +
			"Refuses when no interface is attached.",
		Example: &state.Example{
			Params: map[string]any{"query": "repeater"},
			What:   "pick the repeaters out of a busy map",
		},
	}, func(w *state.World, p any) (any, error) {
		if err := need(); err != nil {
			return nil, err
		}
		q, _ := stringField(p, "query")
		s.ui.FilterMap(q)
		w.Say("map filter: " + q)
		return map[string]any{"query": q}, nil
	})

	st.HandleSpec("tool.set", state.Spec{
		What: "choose what a click on the map does, so a script can place or " +
			"measure without a hand on the mouse",
		Params: []state.Param{
			{Name: "name", Type: state.ParamString, Primary: true, Required: true,
				What: "select, move, place, link or measure; anything else is " +
					"refused with that same list"},
		},
		Returns: []string{"tool"},
		Example: &state.Example{
			Params: "measure", What: "make the next two clicks a distance",
		},
	}, func(w *state.World, p any) (any, error) {
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
	st.HandleSpec("map.layer", state.Spec{
		What: "turn one of the map's layers on or off by the name the map shows, " +
			"so coverage, terrain and the antenna pattern can be reached by a " +
			"script, a capture or a test rather than only by clicking",
		Params: []state.Param{
			{Name: "name", Type: state.ParamString, Primary: true, Required: true,
				What: "the layer: basemap, boundaries, links, nodes, labels, " +
					"traffic, coverage, terrain, regions, antenna or measure. " +
					"An unknown or missing name is refused with the list",
			},
			{Name: "on", Type: state.ParamBool,
				What: "false to stop drawing it; absent means on"},
		},
		Returns: []string{"layers"},
		Answers: "`layers` is every layer against whether it is drawn, the one " +
			"just changed among them, so nothing has to ask again. Turning one " +
			"on is a request for work as well as a redraw: coverage and terrain " +
			"are computed when they are first drawn. Refuses when no interface " +
			"is attached.",
		Example: &state.Example{
			Params: map[string]any{"name": "coverage", "on": true},
			What:   "put the coverage raster under the nodes",
		},
	}, func(_ *state.World, p any) (any, error) {
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
	st.HandleSpec("map.layers", state.Spec{
		What: "read every layer the map knows and whether it is being drawn, " +
			"which is the list `map.layer` takes its names from",
		Returns: []string{"layers"},
		Answers: "One object of layer name against true or false, not a list of " +
			"the ones that are on. Refuses when no interface is attached.",
		Example: &state.Example{
			Params: map[string]any{}, What: "check whether terrain is being drawn",
		},
	}, func(_ *state.World, _ any) (any, error) {
		if err := need(); err != nil {
			return nil, err
		}
		return map[string]any{"layers": s.ui.Layers()}, nil
	})
}
