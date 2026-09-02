// What the session is doing, what the interface is showing, and how big it is
// drawn.
//
// Split out of ui.go, which held every interface verb and outgrew the file
// limit once each of them said what it was for.
package session

import (
	"fmt"

	"github.com/MeshBench/meshbench/internal/app/state"
)

func registerSessionState(st *state.Store, s *Sim) {
	need := func() error { return s.needUI() }

	// session.status: the one-line answer to "what is this doing", which the
	// old workbench answered and this did not. Anything driving the workbench
	// polls it, so it must never fail and never need a loaded network.
	//
	// Namespaced, unlike the old workbench1 verb of the same purpose: every
	// verb here is noun.verb so a script reads as a sentence.
	st.Handle("session.status", func(w *state.World, _ any) (any, error) {
		out := map[string]any{
			"status": w.Status, "nodes": len(w.Nodes), "playing": w.Playing,
			"now_ms": w.NowMs, "firmware_running": w.FirmwareRunning,
		}
		if w.PendingPlay {
			out["status"] = "waiting for firmware before the run starts"
		}
		// The newest job still running, not merely the newest: a bar that
		// finished an hour ago reported as "the job" made every script that
		// polls for completion wait forever.
		out["jobs"] = w.JobsRunning()
		for i := len(w.Jobs) - 1; i >= 0; i-- {
			if j := w.Jobs[i]; !j.Finished {
				// With the id, because a script that has to wait for one
				// particular thing cannot match on prose, and matching on
				// prose is what stopped working the moment the wording of a
				// download improved.
				out["job"] = map[string]any{
					"id": j.ID, "what": j.What, "done": j.Done, "total": j.Total,
				}
				break
			}
		}
		return out, nil
	})

	// ui.said puts a line in the status bar. A control whose verb failed and
	// said nothing is indistinguishable from a control that does nothing.
	st.Handle("ui.said", func(w *state.World, p any) (any, error) {
		msg := primaryString(p, "text")
		w.Say(msg)
		return map[string]any{"said": msg}, nil
	})

	st.Handle("ui.scale", func(w *state.World, p any) (any, error) {
		if err := need(); err != nil {
			return nil, err
		}
		if v, ok := numField(p, "scale"); ok && v > 0 {
			s.ui.SetScale(v)
			w.Say(fmt.Sprintf("interface scale %.2f", v))
		}
		return map[string]any{"scale": s.ui.Scale()}, nil
	})

	st.Handle("ui.state", func(w *state.World, _ any) (any, error) {
		if err := need(); err != nil {
			return nil, err
		}
		out := s.ui.State()
		out["nodes"] = len(w.Nodes)
		out["playing"] = w.Playing
		out["now_ms"] = w.NowMs
		// Running, not merely present: this counted finished rows too, so it
		// reported two jobs long after both had ended and the files were on
		// disk. And the rows themselves, because a bare count cannot tell a
		// script what it is waiting for.
		out["jobs"] = w.JobsRunning()
		out["running"] = jobRows(w.Jobs, true)
		return out, nil
	})
}
