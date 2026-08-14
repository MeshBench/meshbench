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

	"github.com/MeshBench/meshbench/internal/gui/state"
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
	// OpenNodeWindow gives one node a window of its own.
	OpenNodeWindow(node string)

	// OpenPanel shows a panel. where is "" for in the layout, "window" for
	// its own window, or "dock" to bring it back.
	OpenPanel(name, where string) error
	// CloseWindow closes a popped-out panel's window.
	CloseWindow(name string) error

	// Scale and SetScale are the interface's own size, which somebody on a
	// high-density screen changes once and then never thinks about again.
	Scale() float64
	SetScale(v float64)

	// SaveView, LoadView, ListViews and DeleteView are named arrangements.
	SaveView(name string) error
	LoadView(name string) error
	ListViews() []string
	DeleteView(name string) error

	// ZoomMap multiplies the current scale; FilterMap dims what does not
	// match; SetTool chooses what a click on the map does.
	ZoomMap(factor float64)
	FilterMap(query string)
	SetTool(name string) error
	// SetLayer turns a map layer on or off by the name the map shows, and
	// reports the ones there are when it does not know it. A layer that can
	// only be reached by clicking cannot be reached by a script, a capture or
	// a test - which is how coverage and terrain went unchecked.
	SetLayer(name string, on bool) error
	// Layers is what is drawn right now.
	Layers() map[string]bool

	// State is what the interface is showing, for a caller that has no eyes.
	State() map[string]any
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
		name := soleString(p)
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

func registerNodeWindow(st *state.Store, s *Sim) {
	// node.window: the thing people put on a second monitor.
	st.Handle("node.window", func(w *state.World, p any) (any, error) {
		if err := s.needUI(); err != nil {
			return nil, err
		}
		name := soleString(p)
		if m, ok := p.(map[string]any); ok {
			name, _ = m["node"].(string)
		}
		if _, found := findNode(w.Nodes, name); !found {
			return nil, fmt.Errorf("no node named %q", name)
		}
		s.ui.OpenNodeWindow(name)
		return map[string]any{"node": name}, nil
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
		if name := soleString(p); name != "" {
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

// registerUIVerbs are the ones that move the interface rather than the model.
//
// They live here, not in the workbench, so a headless driver gets the same
// vocabulary and a clear "no interface attached" rather than an absence that
// looks like a version mismatch.
func registerUIVerbs(st *state.Store, s *Sim) {
	need := func() error { return s.needUI() }

	// session.status: the one-line answer to "what is this doing", which the
	// old workbench answered and this did not. Anything driving the workbench
	// polls it, so it must never fail and never need a loaded network.
	//
	// Namespaced, unlike the old workbench1 verb of the same purpose: every
	// verb here is noun.verb so a script reads as a sentence.
	st.Handle("session.status", func(w *state.World, _ any) (any, error) {
		out := map[string]any{
			"status": w.Status, "nodes": len(w.Nodes), "playing": w.Playing,
			"now_ms": w.NowMs, "firmware_running": w.FirmwareRunning,
		}
		if w.PendingPlay {
			out["status"] = "waiting for firmware before the run starts"
		}
		if len(w.Jobs) > 0 {
			j := w.Jobs[len(w.Jobs)-1]
			out["job"] = map[string]any{
				"what": j.What, "done": j.Done, "total": j.Total,
			}
		}
		return out, nil
	})

	// ui.said puts a line in the status bar. A control whose verb failed and
	// said nothing is indistinguishable from a control that does nothing.
	st.Handle("ui.said", func(w *state.World, p any) (any, error) {
		msg := soleString(p)
		w.Say(msg)
		return map[string]any{"said": msg}, nil
	})

	st.Handle("panel.open", func(w *state.World, p any) (any, error) {
		if err := need(); err != nil {
			return nil, err
		}
		name, _ := stringField(p, "name")
		if err := s.ui.OpenPanel(name, ""); err != nil {
			return nil, err
		}
		w.Say("showing " + name)
		return map[string]any{"panel": name}, nil
	})
	st.Handle("panel.pop_out", func(w *state.World, p any) (any, error) {
		if err := need(); err != nil {
			return nil, err
		}
		name, _ := stringField(p, "name")
		if err := s.ui.OpenPanel(name, "window"); err != nil {
			return nil, err
		}
		return map[string]any{"panel": name, "where": "window"}, nil
	})
	st.Handle("panel.dock", func(w *state.World, p any) (any, error) {
		if err := need(); err != nil {
			return nil, err
		}
		name, _ := stringField(p, "name")
		if err := s.ui.OpenPanel(name, "dock"); err != nil {
			return nil, err
		}
		return map[string]any{"panel": name, "where": "layout"}, nil
	})
	st.Handle("window.open", func(w *state.World, p any) (any, error) {
		if err := need(); err != nil {
			return nil, err
		}
		name, _ := stringField(p, "name")
		if err := s.ui.OpenPanel(name, "window"); err != nil {
			return nil, err
		}
		return map[string]any{"window": name}, nil
	})
	st.Handle("window.close", func(w *state.World, p any) (any, error) {
		if err := need(); err != nil {
			return nil, err
		}
		name, _ := stringField(p, "name")
		if err := s.ui.CloseWindow(name); err != nil {
			return nil, err
		}
		return map[string]any{"closed": name}, nil
	})

	st.Handle("ui.scale", func(w *state.World, p any) (any, error) {
		if err := need(); err != nil {
			return nil, err
		}
		if v, ok := numField(p, "scale"); ok && v > 0 {
			s.ui.SetScale(v)
			w.Say(fmt.Sprintf("interface scale %.2f", v))
		}
		return map[string]any{"scale": s.ui.Scale()}, nil
	})
	st.Handle("ui.state", func(w *state.World, _ any) (any, error) {
		if err := need(); err != nil {
			return nil, err
		}
		out := s.ui.State()
		out["nodes"] = len(w.Nodes)
		out["playing"] = w.Playing
		out["now_ms"] = w.NowMs
		out["jobs"] = len(w.Jobs)
		return out, nil
	})

	st.Handle("view.save", func(w *state.World, p any) (any, error) {
		if err := need(); err != nil {
			return nil, err
		}
		name, _ := stringField(p, "name")
		if err := s.ui.SaveView(name); err != nil {
			return nil, err
		}
		w.Say("saved view " + name)
		return map[string]any{"saved": name}, nil
	})
	st.Handle("view.load", func(w *state.World, p any) (any, error) {
		if err := need(); err != nil {
			return nil, err
		}
		name, _ := stringField(p, "name")
		if err := s.ui.LoadView(name); err != nil {
			return nil, err
		}
		return map[string]any{"loaded": name}, nil
	})
	st.Handle("view.list", func(_ *state.World, _ any) (any, error) {
		if err := need(); err != nil {
			return nil, err
		}
		return map[string]any{"views": s.ui.ListViews()}, nil
	})
	st.Handle("view.delete", func(w *state.World, p any) (any, error) {
		if err := need(); err != nil {
			return nil, err
		}
		name, _ := stringField(p, "name")
		if err := s.ui.DeleteView(name); err != nil {
			return nil, err
		}
		return map[string]any{"deleted": name}, nil
	})

	st.Handle("map.zoom", func(w *state.World, p any) (any, error) {
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
	st.Handle("map.layer", func(w *state.World, p any) (any, error) {
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
	st.Handle("map.layers", func(w *state.World, _ any) (any, error) {
		if err := need(); err != nil {
			return nil, err
		}
		return map[string]any{"layers": s.ui.Layers()}, nil
	})
}
