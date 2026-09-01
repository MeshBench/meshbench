// The Logs panel's own verbs: where the session log is, and putting a copy
// of it wherever the operator wants one.
package session

import (
	"fmt"

	"github.com/MeshBench/meshbench/internal/app/state"
)

func registerLogs(st *state.Store, s *Sim) {
	st.HandleSpec("log.path", state.Spec{
		What: "find this launch's full status log without knowing the naming " +
			"scheme, which is everything the session has said rather than the " +
			"last twenty lines the status strip keeps",
		Returns: []string{"path"},
		Answers: "One file per launch, timestamped, still being written to. " +
			"Refuses where no log was opened, which is the case for a session " +
			"started without one rather than a fault.",
		Example: &state.Example{
			Params: map[string]any{}, What: "tail this run's log from a shell",
		},
	}, func(_ *state.World, _ any) (any, error) {
		if s.logPath == "" {
			return nil, fmt.Errorf("no session log is open")
		}
		return map[string]any{"path": s.logPath}, nil
	})

	// Reads the file rather than FullLog, the panel's own in-memory tail,
	// because the file is what actually has everything - FullLog is bounded,
	// and a run long enough to fill it would export a log that quietly starts
	// partway through.
	st.HandleSpec("logs.export", state.Spec{
		What: "put a copy of the log as it stands somewhere the operator chose, " +
			"so a run can be attached to a report without the reader needing " +
			"the machine it ran on",
		Params: []state.Param{
			{Name: "to", Type: state.ParamString, Primary: true, Required: true,
				What: "where to write the copy; an empty or missing destination " +
					"is refused, and an existing file there is overwritten"},
		},
		Returns: []string{"path"},
		Answers: "The whole file on disk, not the tail the Logs panel holds, so " +
			"a long run exports everything it said. Refuses where no log was " +
			"opened, and where the copy cannot be written.",
		Example: &state.Example{
			Params: "/tmp/meshbench-run.log", What: "keep a run's log beside its results",
		},
	}, func(w *state.World, p any) (any, error) {
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
