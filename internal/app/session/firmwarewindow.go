// Opening one build's own window.
//
// Beside the verbs that read and change a build rather than in ui.go, which
// holds the interface verbs and was already at the length limit: this one is
// about firmware and only incidentally about a window.
package session

import (
	"github.com/MeshBench/meshbench/internal/app/control"
	"github.com/MeshBench/meshbench/internal/app/state"
)

func registerFirmwareWindow(st *state.Store, s *Sim) {
	// firmware.window: one build, on its own, where it can be changed.
	st.HandleSpec("firmware.window", state.Spec{
		What: "Open one build's own window: what it is, where it lives, and how it is run.",
		Params: []state.Param{
			{Name: "version", Type: state.ParamString, Primary: true, Required: true,
				What: "the build's version or imported label"},
			{Name: "role", Type: state.ParamString,
				What: "which role, when one label carries more than one"},
			{Name: "board", Type: state.ParamString,
				What: "which board; absent means a build for this machine"},
		},
		Returns: []string{"role", "version", "board"},
	}, func(w *state.World, p any) (any, error) {
		if err := s.needUI(); err != nil {
			return nil, err
		}
		// Resolved before the window opens, so an unknown build is refused
		// here rather than by an empty window somebody has to close.
		row, err := findBuildRow(w, s, p)
		if err != nil {
			return nil, err
		}
		if err := s.ui.OpenFirmwareWindow(row.Role, row.Version, row.Board); err != nil {
			return nil, control.WithCode(control.BadParams, err)
		}
		return map[string]any{
			"role": row.Role, "version": row.Version, "board": row.Board,
		}, nil
	})
}
