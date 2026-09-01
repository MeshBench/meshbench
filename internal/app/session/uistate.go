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
	st.HandleSpec("session.status", state.Spec{
		What: "report what the session is saying, where the run has got to and " +
			"which long job is still going, cheaply enough for a script to poll " +
			"and answered whether or not anything is loaded or drawing",
		Returns: []string{
			"status", "nodes", "playing", "now_ms", "firmware_running", "jobs", "job",
		},
		Answers: "`jobs` counts only the jobs still running, and `job` is the " +
			"newest of those: it is absent when nothing is running, which is " +
			"what a script waiting for a download or a sweep watches for. It " +
			"carries the job's id, because matching on the wording of a " +
			"progress line stopped working the moment the wording improved. " +
			"`status` is the last thing said, replaced while a play is waiting " +
			"on firmware to come up.",
		Example: &state.Example{
			Params: map[string]any{}, What: "poll until the work in hand has finished",
			Runnable: true,
		},
	}, func(w *state.World, _ any) (any, error) {
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
	st.HandleSpec("ui.said", state.Spec{
		What: "put a line where the operator is already looking, so a script's " +
			"own step is visible in the status strip and in the session log " +
			"beside the verbs it drove",
		Params: []state.Param{
			{Name: "text", Type: state.ParamString, Primary: true,
				What: "the line to show; anything that is not a bare string or a " +
					"single-keyed object says an empty line rather than refusing"},
		},
		Returns: []string{"said"},
		Answers: "It never refuses, and it does not need an interface: with " +
			"nothing drawing, the line still goes to the log the session keeps.",
		Example: &state.Example{
			Params: "coverage sweep finished", What: "mark a step of a script on screen",
			Runnable: true,
		},
	}, func(w *state.World, p any) (any, error) {
		msg := soleString(p)
		w.Say(msg)
		return map[string]any{"said": msg}, nil
	})

	st.HandleSpec("ui.scale", state.Spec{
		What: "read or set how large the interface draws itself, the one setting " +
			"a high-density screen needs and then never again",
		Params: []state.Param{
			{Name: "scale", Type: state.ParamNumber, Primary: true,
				What: "the new scale, one being the interface's own size; absent, " +
					"zero or negative reads the current scale and changes nothing"},
		},
		Returns: []string{"scale"},
		Answers: "The scale in force after the call, read back from the " +
			"interface rather than repeated from the request, so an interface " +
			"that clamps it says so. Refuses when no interface is attached.",
		Example: &state.Example{
			Params: map[string]any{"scale": 1.25},
			What:   "make everything a quarter larger on a dense screen",
		},
	}, func(w *state.World, p any) (any, error) {
		if err := need(); err != nil {
			return nil, err
		}
		if v, ok := numField(p, "scale"); ok && v > 0 {
			s.ui.SetScale(v)
			w.Say(fmt.Sprintf("interface scale %.2f", v))
		}
		return map[string]any{"scale": s.ui.Scale()}, nil
	})

	st.HandleSpec("ui.state", state.Spec{
		What: "ask what is on screen for a caller with no eyes: the view, the " +
			"panels in their own windows, the map tool, and what the run is " +
			"doing beside them",
		Returns: []string{
			"view", "popped", "scale", "tool", "nodes", "playing", "now_ms",
			"jobs", "running",
		},
		Answers: "The first four come from whatever is drawing, so another " +
			"interface may answer with other keys; the rest are the session's " +
			"own. `jobs` counts the jobs still running and `running` is those " +
			"same jobs as rows with their ids, because a bare count cannot tell " +
			"a script what it is waiting for. Refuses when no interface is " +
			"attached, which is what makes session.status the one to poll.",
		Example: &state.Example{
			Params: map[string]any{}, What: "check which view a screenshot will catch",
		},
	}, func(w *state.World, _ any) (any, error) {
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
