// Driving a run from outside the window.
//
// These five verbs are what a script needs and a person does not: a person
// presses play and watches, a script has to start a run of a stated length,
// ask whether it has finished, and get the same run again tomorrow. They
// existed on the old control socket and had no equivalent here, so anything
// automated against MeshBench - CI, a sweep driver, the study harness - could
// not be pointed at this build at all.
package session

import (
	"context"
	"fmt"

	"github.com/MeshBench/meshbench/internal/gui/state"
)

// registerSimControl adds the run-from-a-script verbs.
func registerSimControl(st *state.Store, s *Sim) {
	// sim.run: play until a stated simulated time, then stop on its own.
	//
	// Asynchronous by construction - it sets a limit and returns, rather than
	// stepping the engine here. Stepping inside a handler would block the
	// store's goroutine, so nothing would draw and no other verb would be
	// answered for the length of the run, which is indistinguishable from a
	// hang.
	st.Handle("sim.run", func(w *state.World, p any) (any, error) {
		forMs := uint32(10_000)
		if v, ok := numField(p, "for_ms"); ok && v > 0 {
			forMs = uint32(v)
		}
		w.RunUntilMs = w.NowMs + forMs
		w.Playing = true
		w.Say(fmt.Sprintf("running to %.1f s", float64(w.RunUntilMs)/1000))
		return map[string]any{
			"running": true, "until_ms": w.RunUntilMs, "now_ms": w.NowMs,
		}, nil
	})

	// sim.state: the one a caller polls, so it must be cheap and must never
	// fail before a network is loaded.
	st.Handle("sim.state", func(w *state.World, _ any) (any, error) {
		return map[string]any{
			"playing":  w.Playing,
			"now_ms":   w.NowMs,
			"until_ms": w.RunUntilMs,
			"events":   w.EventTotal,
			"step_ms":  st.StepMs(),
			"seed":     w.Seed,
		}, nil
	})

	// sim.seed: repeats of one seed are identical by design, so a study that
	// wants to know whether a difference is real rather than one draw has to
	// vary this. Setting it rebuilds, because a seed applied halfway through
	// a run is neither of the two runs it claims to be.
	st.Handle("sim.seed", func(w *state.World, p any) (any, error) {
		if v, ok := numField(p, "seed"); ok && v > 0 {
			w.Seed = uint64(v)
			if err := s.rebuild(w); err != nil {
				return nil, err
			}
			w.Links = nil
			s.warm(st, len(s.nodes))
		}
		return map[string]any{"seed": w.Seed}, nil
	})

	// sim.settle advances the engine without the run being on. Provisioning
	// queues commands at each node's serial input, and time has to move for
	// the firmware to read and act on them - the old workbench steps sixty
	// after configuring, and without the same here a paused mesh has been
	// told everything and heard nothing.
	st.Handle("sim.settle", func(w *state.World, p any) (any, error) {
		if s.eng == nil {
			return nil, fmt.Errorf("no simulation")
		}
		n := 60
		if v, ok := numField(p, "steps"); ok && v > 0 {
			n = int(v)
		}
		for i := 0; i < n; i++ {
			_ = s.eng.Step(context.Background())
		}
		w.NowMs = s.eng.NowMs()
		return map[string]any{"now_ms": w.NowMs, "steps": n}, nil
	})

	st.Handle("sim.reset", func(w *state.World, _ any) (any, error) {
		w.Playing, w.RunUntilMs = false, 0
		if err := s.rebuild(w); err != nil {
			return nil, err
		}
		w.Links = nil
		s.warm(st, len(s.nodes))
		w.Say("reset")
		return map[string]any{"seed": w.Seed, "now_ms": w.NowMs}, nil
	})

	// study.margin: how far outside the boundary a node still matters. It was
	// readable everywhere and settable nowhere but the fixture file.
	st.Handle("study.margin", func(w *state.World, p any) (any, error) {
		if v, ok := numField(p, "km"); ok && v >= 0 {
			w.MarginKm = v
			w.Say(fmt.Sprintf("study margin %g km", v))
		}
		return map[string]any{"km": w.MarginKm}, nil
	})

	// sim.speed: the old socket said "factor" and meant a multiplier of real
	// time; this build paces in simulated milliseconds per tick, which is the
	// same control said honestly. A factor is accepted and converted so an
	// existing script keeps working.
	st.Handle("sim.speed", func(w *state.World, p any) (any, error) {
		if v, ok := numField(p, "step_ms"); ok && v > 0 {
			st.SetStepMs(uint32(v))
		} else if v, ok := numField(p, "factor"); ok && v > 0 {
			st.SetStepMs(uint32(float64(baseStepMs) * v))
		}
		w.Say(fmt.Sprintf("%d ms per tick", st.StepMs()))
		return map[string]any{"step_ms": st.StepMs()}, nil
	})
}

// baseStepMs is what a factor of one means.
const baseStepMs = 10

// numField reads a number from a verb's parameters, which arrive either as a
// JSON object or as a bare number when the verb takes exactly one.
func numField(p any, name string) (float64, bool) {
	switch v := p.(type) {
	case map[string]any:
		n, ok := v[name].(float64)
		return n, ok
	case float64:
		return v, true
	case int:
		return float64(v), true
	}
	return 0, false
}
