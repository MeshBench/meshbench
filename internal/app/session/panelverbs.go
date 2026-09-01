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

	st.HandleSpec("panel.open", state.Spec{
		What: "put a panel in the layout, switching to the view it belongs to " +
			"on the way, which is how the panels no view starts with are " +
			"reached at all",
		Params: []state.Param{
			{Name: "name", Type: state.ParamString, Primary: true, Required: true,
				What: "the panel, spelled as panels.list spells it; an unknown " +
					"or missing name is refused with the list of the ones there are"},
		},
		Returns: []string{"panel"},
		Answers: "It answers once the panel has been asked for, not once it is " +
			"drawn: the layout is the frame loop's, and the change lands on the " +
			"next frame. A panel already in the layout is left where it is. " +
			"Refuses when no interface is attached.",
		Example: &state.Example{
			Params: "Waterfall", What: "bring up the waterfall from a script",
		},
	}, func(w *state.World, p any) (any, error) {
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
	st.HandleSpec("panel.pop_out", state.Spec{
		What: "send a panel out into a window of its own, so a second monitor " +
			"can hold what the layout has no room for",
		Params: []state.Param{
			{Name: "name", Type: state.ParamString, Primary: true, Required: true,
				What: "the panel to pop out; an unknown or missing name is " +
					"refused with the list of the ones there are"},
		},
		Returns: []string{"panel", "where"},
		Answers: "`where` is always \"window\", the verb having only one " +
			"destination: it is there so an answer read on its own says what " +
			"happened. Refuses when no interface is attached.",
		Example: &state.Example{
			Params: map[string]any{"name": "Map"},
			What:   "give the map a monitor to itself",
		},
	}, func(_ *state.World, p any) (any, error) {
		if err := need(); err != nil {
			return nil, err
		}
		name, _ := stringField(p, "name")
		if err := s.ui.OpenPanel(name, "window"); err != nil {
			return nil, err
		}
		return map[string]any{"panel": name, "where": "window"}, nil
	})
	st.HandleSpec("panel.dock", state.Spec{
		What: "bring a popped-out panel back into the main window, which is the " +
			"other half of panel.pop_out and the only way back for a window a " +
			"compositor has hidden",
		Params: []state.Param{
			{Name: "name", Type: state.ParamString, Primary: true, Required: true,
				What: "the panel to dock; an unknown or missing name is refused " +
					"with the list of the ones there are"},
		},
		Returns: []string{"panel", "where"},
		Answers: "`where` is always \"layout\". A panel that was never popped " +
			"out is docked where it belongs rather than refused. Refuses when " +
			"no interface is attached.",
		Example: &state.Example{
			Params: map[string]any{"name": "Map"},
			What:   "put the map back in the main window",
		},
	}, func(_ *state.World, p any) (any, error) {
		if err := need(); err != nil {
			return nil, err
		}
		name, _ := stringField(p, "name")
		if err := s.ui.OpenPanel(name, "dock"); err != nil {
			return nil, err
		}
		return map[string]any{"panel": name, "where": "layout"}, nil
	})
	st.HandleSpec("panel.close", state.Spec{
		What: "take a panel out of the layout, giving the room to what is left",
		Params: []state.Param{
			{Name: "name", Type: state.ParamString, Primary: true, Required: true,
				What: "the panel to close; an unknown or missing name is refused " +
					"with the list of the ones there are"},
		},
		Returns: []string{"panel"},
		Answers: "Whether the panel was open is not asked, only whether it " +
			"exists: closing one that is not in the layout answers the same way " +
			"and does nothing, which is what the caller wanted anyway. Refuses " +
			"when no interface is attached.",
		Example: &state.Example{
			Params: map[string]any{"name": "Inspector"},
			What:   "clear the rail before a screenshot",
		},
	}, func(w *state.World, p any) (any, error) {
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
	st.HandleSpec("layout.reset", state.Spec{
		What: "put the view on screen back to the arrangement it is declared " +
			"with, which is the way back from a layout somebody has docked into " +
			"an unusable shape",
		Returns: []string{"reset"},
		Answers: "The view on screen only: the other views keep whatever was " +
			"done to them. Popped-out windows are not pulled back in. Refuses " +
			"when no interface is attached.",
		Example: &state.Example{
			Params: map[string]any{}, What: "start this view's layout again",
		},
	}, func(w *state.World, _ any) (any, error) {
		if err := need(); err != nil {
			return nil, err
		}
		s.ui.ResetLayout()
		w.Say("layout reset")
		return map[string]any{"reset": true}, nil
	})
	st.HandleSpec("window.open", state.Spec{
		What: "open a panel in a window of its own, the same act as " +
			"panel.pop_out under the name a caller thinking in windows reaches " +
			"for",
		Params: []state.Param{
			{Name: "name", Type: state.ParamString, Primary: true, Required: true,
				What: "the panel to give a window; an unknown or missing name is " +
					"refused with the list of the ones there are"},
		},
		Returns: []string{"window"},
		Answers: "The window is asked for here and opened by the frame loop, so " +
			"the answer says it was accepted rather than that a window is up. " +
			"Refuses when no interface is attached.",
		Example: &state.Example{
			Params: map[string]any{"name": "Waterfall"},
			What:   "watch the waterfall beside the map",
		},
	}, func(_ *state.World, p any) (any, error) {
		if err := need(); err != nil {
			return nil, err
		}
		name, _ := stringField(p, "name")
		if err := s.ui.OpenPanel(name, "window"); err != nil {
			return nil, err
		}
		return map[string]any{"window": name}, nil
	})
	st.HandleSpec("window.close", state.Spec{
		What: "close a panel's own window, which returns the panel to the main " +
			"layout rather than losing it",
		Params: []state.Param{
			{Name: "name", Type: state.ParamString, Primary: true, Required: true,
				What: "the panel whose window is closed; a panel that is not in " +
					"a window of its own is refused"},
		},
		Returns: []string{"closed"},
		Answers: "Refuses when no interface is attached, and refuses where the " +
			"interface has no window manager behind it.",
		Example: &state.Example{
			Params: map[string]any{"name": "Waterfall"},
			What:   "close the window a capture no longer needs",
		},
	}, func(_ *state.World, p any) (any, error) {
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
