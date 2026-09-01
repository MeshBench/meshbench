// Panels, and the windows they can be pulled out into.
//
// Split out of ui.go, which held every interface verb and outgrew the file
// limit once each of them said what it was for.
package session

import (
	"github.com/MeshBench/meshbench/internal/app/state"
)

func registerPanelVerbs(st *state.Store, s *Sim) {
	need := func() error { return s.needUI() }

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
	st.Handle("panel.pop_out", func(_ *state.World, p any) (any, error) {
		if err := need(); err != nil {
			return nil, err
		}
		name, _ := stringField(p, "name")
		if err := s.ui.OpenPanel(name, "window"); err != nil {
			return nil, err
		}
		return map[string]any{"panel": name, "where": "window"}, nil
	})
	st.Handle("panel.dock", func(_ *state.World, p any) (any, error) {
		if err := need(); err != nil {
			return nil, err
		}
		name, _ := stringField(p, "name")
		if err := s.ui.OpenPanel(name, "dock"); err != nil {
			return nil, err
		}
		return map[string]any{"panel": name, "where": "layout"}, nil
	})
	st.Handle("panel.close", func(w *state.World, p any) (any, error) {
		if err := need(); err != nil {
			return nil, err
		}
		name, _ := stringField(p, "name")
		if err := s.ui.ClosePanel(name); err != nil {
			return nil, err
		}
		w.Say(name + " closed")
		return map[string]any{"panel": name}, nil
	})
	st.Handle("layout.reset", func(w *state.World, _ any) (any, error) {
		if err := need(); err != nil {
			return nil, err
		}
		s.ui.ResetLayout()
		w.Say("layout reset")
		return map[string]any{"reset": true}, nil
	})
	st.Handle("window.open", func(_ *state.World, p any) (any, error) {
		if err := need(); err != nil {
			return nil, err
		}
		name, _ := stringField(p, "name")
		if err := s.ui.OpenPanel(name, "window"); err != nil {
			return nil, err
		}
		return map[string]any{"window": name}, nil
	})
	st.Handle("window.close", func(_ *state.World, p any) (any, error) {
		if err := need(); err != nil {
			return nil, err
		}
		name, _ := stringField(p, "name")
		if err := s.ui.CloseWindow(name); err != nil {
			return nil, err
		}
		return map[string]any{"closed": name}, nil
	})
}
