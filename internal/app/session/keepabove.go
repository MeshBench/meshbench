// The one window-placement preference: whether panels in their own windows
// stay above the main one.
//
// On macOS and Windows that is a property of a normal window and always on,
// so no preference exists there. On Linux the ask exists only under Wayland,
// as a wlr-layer-shell window - and that changes what the window is: no
// decoration of the compositor's, so the window draws its own title bar; no
// taskbar entry; no minimise, the close button being what returns the panel
// to the main layout. A preference, because somebody may prefer the
// compositor's own windows to the pinning.
package session

import (
	"github.com/MeshBench/meshbench/internal/app/state"
)

// keepAbove is the preference, defaulting to on: the issue it answers is a
// panel lost behind the main window, so the machine that never said is the
// machine that wants it.
func (s *Sim) keepAbove() bool {
	return s.prefs.KeepAbove == nil || *s.prefs.KeepAbove
}

// registerKeepAbove is the verb the interface's check fires - the same one a
// script would use, because a preference only the mouse can change is a
// preference the control socket cannot report.
func registerKeepAbove(st *state.Store, s *Sim) {
	st.HandleSpec("ui.keep_above", state.Spec{
		What: "Say whether a window popped out of the workbench stays above it, " +
			"and report the setting either way.",
		Params: []state.Param{
			{Name: "on", Type: state.ParamBool,
				What: "set it; omit to read the current setting without changing it"},
		},
		Returns: []string{"on"},
	}, func(w *state.World, p any) (any, error) {
		if v, ok := boolField(p, "on"); ok {
			on := v
			s.prefs.KeepAbove = &on
			s.savePrefs()
			w.KeepAbove = on
			if on {
				w.Say("new windows stay above the main one - on Wayland that " +
					"means a window with our own title bar, which the close " +
					"button of returns the panel to the main window")
			} else {
				w.Say("new windows are the compositor's own, and can fall " +
					"behind the main one")
			}
		}
		return map[string]any{"on": s.keepAbove()}, nil
	})
}
