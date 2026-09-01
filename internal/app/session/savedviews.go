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

	st.HandleSpec("view.save", state.Spec{
		What: "keep the arrangement on screen under a name, so a layout built " +
			"for one kind of work survives the next launch and the next machine",
		Params: []state.Param{
			{Name: "name", Type: state.ParamString, Primary: true, Required: true,
				What: "what to call it; an empty or missing name is refused, and " +
					"a name already used is overwritten without asking"},
		},
		Returns: []string{"saved"},
		Answers: "It saves every view's arrangement, not only the one on screen, " +
			"along with which view is showing and which panels are in windows of " +
			"their own, to a file under the user's config directory. Refuses " +
			"when no interface is attached.",
		Example: &state.Example{
			Params: map[string]any{"name": "coverage-review"},
			What:   "keep the layout a coverage study is read in",
		},
	}, func(w *state.World, p any) (any, error) {
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
	st.HandleSpec("view.load", state.Spec{
		What: "put a saved arrangement back, windows and all",
		Params: []state.Param{
			{Name: "name", Type: state.ParamString, Primary: true, Required: true,
				What: "the saved view, as view.list names it; one that is not " +
					"there is refused"},
		},
		Returns: []string{"loaded"},
		Answers: "It answers once the arrangement is asked for, the layouts and " +
			"the view landing on the next frame and the windows opening after " +
			"that. A file written before docking existed carries no layouts and " +
			"loads as the declared presets rather than failing. Refuses when no " +
			"interface is attached.",
		Example: &state.Example{
			Params: "coverage-review", What: "go back to the layout a study is read in",
		},
	}, func(_ *state.World, p any) (any, error) {
		if err := need(); err != nil {
			return nil, err
		}
		name, _ := stringField(p, "name")
		if err := s.ui.LoadView(name); err != nil {
			return nil, err
		}
		return map[string]any{"loaded": name}, nil
	})
	st.HandleSpec("view.list", state.Spec{
		What: "name the saved arrangements there are to load, which is the only " +
			"way to find out what a machine has kept",
		Returns: []string{"views"},
		Answers: "The names alone, sorted, without the .json the files carry. " +
			"Null where nothing has been saved, and null too where the platform " +
			"cannot say where config lives, which reads the same from here. " +
			"Refuses when no interface is attached.",
		Example: &state.Example{
			Params: map[string]any{}, What: "see which layouts this machine has kept",
		},
	}, func(_ *state.World, _ any) (any, error) {
		if err := need(); err != nil {
			return nil, err
		}
		return map[string]any{"views": s.ui.ListViews()}, nil
	})
	st.HandleSpec("view.delete", state.Spec{
		What: "forget a saved arrangement, deleting the file behind it",
		Params: []state.Param{
			{Name: "name", Type: state.ParamString, Primary: true, Required: true,
				What: "the saved view to remove; one that is not there is refused"},
		},
		Returns: []string{"deleted"},
		Answers: "The layout on screen is not touched: this removes the copy on " +
			"disk, and nothing is asked first. Refuses when no interface is " +
			"attached.",
		Example: &state.Example{
			Params: map[string]any{"name": "coverage-review"},
			What:   "drop a layout that is no longer used",
		},
	}, func(_ *state.World, p any) (any, error) {
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
