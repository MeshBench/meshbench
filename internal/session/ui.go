// What a session can ask of whatever is drawing it.
//
// Three of the old socket's verbs move the interface rather than the
// simulation: show me that view, what panels are there, close. They are still
// session verbs, because a script asks for them over the same socket as
// everything else and should not have to know which binary is listening.
//
// So the verb lives here and the drawing is delegated. A workbench registers
// itself; a headless driver registers nothing and the verbs say plainly that
// there is no interface, rather than being absent and looking like a version
// mismatch.
package session

import (
	"fmt"

	"github.com/A13xB0/meshcoresim/internal/gui/state"
)

// UI is implemented by whatever is on screen.
type UI interface {
	// ShowView switches to a named view, returning an error naming the ones
	// that exist if it does not.
	ShowView(name string) error
	// PanelNames is every panel, in any order.
	PanelNames() []string
	// Quit closes the application, stopping firmware on the way out.
	Quit()
	// CentreMap points the camera at a position. Zoom of zero leaves the
	// current scale alone, so "look here" and "look here this close" are the
	// same verb rather than two.
	CentreMap(lat, lon, zoom float64)
	// FitMap frames every node, which is the only camera request that needs
	// no numbers and is the one somebody driving a capture usually wants.
	FitMap()
}

// SetUI attaches an interface. Safe to leave unset.
func (s *Sim) SetUI(u UI) { s.ui = u }

func (s *Sim) needUI() error {
	if s.ui == nil {
		return fmt.Errorf("this session has no interface attached, so there is nothing to show")
	}
	return nil
}

func registerUI(st *state.Store, s *Sim) {
	st.Handle("workspace.set", func(w *state.World, p any) (any, error) {
		if err := s.needUI(); err != nil {
			return nil, err
		}
		name, _ := p.(string)
		if m, ok := p.(map[string]any); ok {
			name, _ = m["view"].(string)
		}
		if err := s.ui.ShowView(name); err != nil {
			return nil, err
		}
		w.Say("showing " + name)
		return map[string]any{"view": name}, nil
	})
	st.Handle("panels.list", func(_ *state.World, _ any) (any, error) {
		if err := s.needUI(); err != nil {
			return nil, err
		}
		names := s.ui.PanelNames()
		return map[string]any{"panels": names, "count": len(names)}, nil
	})
	st.Handle("app.quit", func(w *state.World, _ any) (any, error) {
		w.Say("closing")
		if s.ui != nil {
			go s.ui.Quit()
			return map[string]any{"closing": true}, nil
		}
		// No interface: still stop firmware, because a headless driver that
		// asks to quit means it.
		go s.Close()
		return map[string]any{"closing": true, "headless": true}, nil
	})
}

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
		if name, ok := p.(string); ok {
			n, found := findNode(w.Nodes, name)
			if !found {
				return nil, fmt.Errorf("no node named %q", name)
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
						return nil, fmt.Errorf("no node named %q", name)
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

	st.Handle("map.fit", func(w *state.World, _ any) (any, error) {
		if err := s.needUI(); err != nil {
			return nil, err
		}
		s.ui.FitMap()
		return map[string]any{"nodes": len(w.Nodes)}, nil
	})
}

func findNode(nodes []state.Node, name string) (state.Node, bool) {
	for _, n := range nodes {
		if n.Name == name {
			return n, true
		}
	}
	return state.Node{}, false
}
