// What is running, for something that is not a person watching a window.
//
// The window has always had this: a status line naming a job and its
// percentage. A script had a count, and only through ui.state, which refuses
// where there is no interface at all. So a driver could tell that two things
// were happening and never which two, could not say how far either had got,
// and could not wait for one in particular without matching on the wording of
// its label.
package session

import (
	"github.com/MeshBench/meshbench/internal/app/state"
)

// jobRows renders jobs for a caller outside the process. runningOnly drops the
// finished rows, which are kept in the list so a caller polling at the wrong
// moment still learns how what it was waiting for turned out.
func jobRows(jobs []state.Job, runningOnly bool) []map[string]any {
	out := make([]map[string]any, 0, len(jobs))
	for i := range jobs {
		j := &jobs[i]
		if runningOnly && j.Finished {
			continue
		}
		out = append(out, map[string]any{
			"id": j.ID, "what": j.What, "done": j.Done, "total": j.Total,
			"finished": j.Finished, "failed": j.Failed,
			// Whether job.cancel would be refused, which is worth knowing
			// before pressing it: a terrain download can be stopped and a link
			// measurement cannot.
			"cancellable": j.Cancel != nil,
		})
	}
	return out
}

func registerJobList(st *state.Store) {
	st.Handle("job.list", func(w *state.World, p any) (any, error) {
		all, _ := boolField(p, "all")
		return map[string]any{
			"jobs": jobRows(w.Jobs, !all), "running": w.JobsRunning(),
		}, nil
	})
}
