// The Logs panel's own verbs: where the session log is, and putting a copy
// of it wherever the operator wants one.
package session

import (
	"fmt"

	"github.com/MeshBench/meshbench/internal/app/state"
)

func registerLogs(st *state.Store, s *Sim) {
	st.Handle("log.path", func(_ *state.World, _ any) (any, error) {
		if s.logPath == "" {
			return nil, fmt.Errorf("no session log is open")
		}
		return map[string]any{"path": s.logPath}, nil
	})

	// Reads the file rather than FullLog, the panel's own in-memory tail,
	// because the file is what actually has everything - FullLog is bounded,
	// and a run long enough to fill it would export a log that quietly starts
	// partway through.
	st.Handle("logs.export", func(w *state.World, p any) (any, error) {
		to := soleString(p)
		if to == "" {
			return nil, fmt.Errorf("logs.export needs a destination path")
		}
		if s.logPath == "" {
			return nil, fmt.Errorf("no session log is open")
		}
		if err := copyFile(s.logPath, to); err != nil {
			return nil, fmt.Errorf("logs.export: %w", err)
		}
		w.Say("log exported to " + to)
		return map[string]any{"path": to}, nil
	})
}
