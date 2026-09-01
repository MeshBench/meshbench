// Named arrangements: the layout somebody built, kept so it can be had again.
//
// Split out of ui.go, which held every interface verb and outgrew the file
// limit once each of them said what it was for.
package session

import (
	"github.com/MeshBench/meshbench/internal/app/state"
)

func registerSavedViews(st *state.Store, s *Sim) {
	need := func() error { return s.needUI() }

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
	st.Handle("view.load", func(_ *state.World, p any) (any, error) {
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
	st.Handle("view.delete", func(_ *state.World, p any) (any, error) {
		if err := need(); err != nil {
			return nil, err
		}
		name, _ := stringField(p, "name")
		if err := s.ui.DeleteView(name); err != nil {
			return nil, err
		}
		return map[string]any{"deleted": name}, nil
	})
}
