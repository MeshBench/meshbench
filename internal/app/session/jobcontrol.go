// Creating, cancelling and retiring the rows job.list reads.
//
// Apart from the list itself because these three are where the invariant
// lives: a job row is the only handle anybody has on stopping the work behind
// it, and the two callbacks that maintain the row are the two places that
// handle can be dropped without anybody noticing.
package session

import (
	"fmt"

	"github.com/MeshBench/meshbench/internal/app/state"
)

func registerJobControl(st *state.Store) {
	st.HandleInternal("job.progress", func(w *state.World, p any) (any, error) {
		j, ok := p.(state.Job)
		if !ok {
			return nil, wrongCallback("job.progress")
		}
		for i := range w.Jobs {
			if w.Jobs[i].ID == j.ID {
				// A progress update carries counts, not closures. The callback
				// that reports "412 of 500" has no way to rebuild the cancel
				// function whoever started the job registered, so replacing the
				// row wholesale threw away the only handle anybody had on
				// stopping it - which is why state.Job.Cancel existed for
				// months and could never once be called.
				if j.Cancel == nil {
					j.Cancel = w.Jobs[i].Cancel
				}
				w.Jobs[i] = j
				return nil, nil
			}
		}
		w.Jobs = append(w.Jobs, j)
		return nil, nil
	})

	// Refusing by name rather than silently doing nothing: a job with no
	// cancel is one that cannot be interrupted safely, and an operator who
	// asked deserves to be told that rather than left watching a bar that
	// carries on.
	st.Handle("job.cancel", func(w *state.World, p any) (any, error) {
		id := soleString(p)
		if m, ok := p.(map[string]any); ok {
			id, _ = m["id"].(string)
		}
		if id == "" {
			return nil, fmt.Errorf("job.cancel needs an id")
		}
		for i := range w.Jobs {
			if w.Jobs[i].ID != id {
				continue
			}
			if w.Jobs[i].Cancel == nil {
				return nil, fmt.Errorf("%q cannot be stopped once it has started", id)
			}
			w.Jobs[i].Cancel()
			// Said, and left on screen saying it: the work stops when its
			// context notices, which is not this instant, and a row that
			// vanished on the press would claim otherwise.
			w.Jobs[i].What = "stopping: " + w.Jobs[i].What
			w.Say("stopping " + id)
			return map[string]any{"stopping": id}, nil
		}
		return nil, fmt.Errorf("no job called %q is running", id)
	})

	st.HandleInternal("job.done", func(w *state.World, p any) (any, error) {
		id := soleString(p)
		for i := range w.Jobs {
			if w.Jobs[i].ID == id {
				w.Jobs = append(w.Jobs[:i], w.Jobs[i+1:]...)
				break
			}
		}
		return nil, nil
	})
}
