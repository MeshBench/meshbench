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
	"encoding/json"
	"fmt"

	"github.com/MeshBench/meshbench/internal/app/state"
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
	st.HandleSpec("sim.run", state.Spec{
		What: "play until a stated simulated time and stop there, which is how a " +
			"script gets a run of a known length instead of watching for one",
		Params: []state.Param{
			{Name: "for_ms", Type: state.ParamNumber, Primary: true,
				What: "simulated milliseconds to run for; anything not a " +
					"positive number leaves it at ten seconds"},
		},
		Returns: []string{"running", "until_ms", "now_ms"},
		Answers: "It returns as soon as the limit is set, not when the run " +
			"reaches it. Poll `sim.state` until `playing` goes false.",
		Example: &state.Example{
			Params: map[string]any{"for_ms": 60000}, What: "run for a simulated minute",
			Runnable: true,
		},
	}, func(w *state.World, p any) (any, error) {
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
	st.HandleSpec("sim.state", state.Spec{
		What: "report where the run has got to, cheaply enough to poll and " +
			"safely enough to call before anything is loaded",
		Returns: []string{"playing", "now_ms", "until_ms", "events", "step_ms", "seed"},
		Answers: "`until_ms` is zero unless `sim.run` set a limit. `events` is " +
			"the count since the engine was built, not since the last call.",
		Example: &state.Example{
			Params: map[string]any{}, What: "ask whether the run has finished",
			Runnable: true,
		},
	}, func(w *state.World, _ any) (any, error) {
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
	st.HandleSpec("sim.seed", state.Spec{
		What: "read the seed the run draws its noise and its timing jitter from, " +
			"or set a new one and rebuild on it",
		Params: []state.Param{
			{Name: "seed", Type: state.ParamNumber, Primary: true,
				What: "the new seed; anything not a positive number reads the " +
					"current one without changing it"},
		},
		Returns: []string{"seed"},
		Answers: "Setting it rebuilds the engine and re-measures every link, so " +
			"the run starts again rather than continuing on a new draw.",
		Example: &state.Example{
			Params: map[string]any{}, What: "read the seed this run is on",
			Runnable: true,
		},
	}, func(w *state.World, p any) (any, error) {
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
	st.HandleSpec("sim.settle", state.Spec{
		What: "step the engine on a stopped clock so queued serial input reaches " +
			"the firmware, which is what makes provisioning take effect",
		Params: []state.Param{
			{Name: "steps", Type: state.ParamNumber, Primary: true,
				What: "how many ticks to step; anything not a positive number " +
					"leaves it at sixty"},
		},
		Returns: []string{"now_ms", "steps"},
		Answers: "Refuses with no engine built. It steps synchronously, so it " +
			"answers only once the steps have been taken.",
		Example: &state.Example{
			Params: map[string]any{"steps": 60},
			What:   "let a just-provisioned mesh read what it was sent",
		},
	}, func(w *state.World, p any) (any, error) {
		if s.eng == nil {
			return nil, ErrNoSimulation
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

	st.HandleSpec("sim.reset", state.Spec{
		What: "put the clock back to zero and rebuild the engine on the same " +
			"seed and the same nodes, which is how the arm of a comparison starts",
		Returns: []string{"seed", "now_ms"},
		Answers: "The scenario survives and the run does not: the send schedule " +
			"is cleared with the clock, and every link is measured again.",
		Example: &state.Example{
			Params: map[string]any{}, What: "start this scenario over",
			Runnable: true,
		},
	}, func(w *state.World, _ any) (any, error) {
		w.Playing, w.RunUntilMs = false, 0
		if err := s.rebuild(w); err != nil {
			return nil, err
		}
		// The clock went back to zero, so what the schedule has already said
		// has to go with it - or a repeating send would sit waiting out an
		// interval measured against a run that no longer exists.
		s.resetSendClock()
		w.Links = nil
		s.warm(st, len(s.nodes))
		w.Say("reset")
		return map[string]any{"seed": w.Seed, "now_ms": w.NowMs}, nil
	})

	// study.margin: how far outside the boundary a node still matters. It was
	// readable everywhere and settable nowhere but the fixture file.
	st.HandleSpec("study.margin", state.Spec{
		What: "read or set how far outside the study boundary a node still counts, " +
			"which decides what an imported network keeps",
		Params: []state.Param{
			{Name: "km", Type: state.ParamNumber, Primary: true,
				What: "kilometres beyond the boundary; a negative number is " +
					"ignored and only reads the current value"},
		},
		Returns: []string{"km"},
		Example: &state.Example{
			Params:   map[string]any{"km": 20},
			What:     "keep nodes up to twenty kilometres outside the boundary",
			Runnable: true,
		},
	}, func(w *state.World, p any) (any, error) {
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
	st.HandleSpec("sim.speed", state.Spec{
		What: "set how much simulated time one tick covers, which is the honest " +
			"form of a speed control",
		Params: []state.Param{
			{Name: "step_ms", Type: state.ParamNumber,
				What: "simulated milliseconds per tick, which is what the engine " +
					"actually paces on"},
			{Name: "factor", Type: state.ParamNumber,
				What: "a multiple of ten milliseconds per tick, read only when " +
					"`step_ms` is absent, for scripts written against the old socket"},
		},
		Returns: []string{"step_ms"},
		Answers: "`step_ms` is what it settled on. Neither parameter positive " +
			"leaves the tick alone and reports it.",
		Example: &state.Example{
			Params: map[string]any{"step_ms": 10}, What: "ten simulated milliseconds a tick",
			Runnable: true,
		},
	}, func(w *state.World, p any) (any, error) {
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
// numField reads a number a verb was given, whoever gave it.
//
// Every kind of integer, not just the float a decoder produces. The control
// socket arrives as JSON and every number in it is a float64, so a map that
// only understood floats worked perfectly for anything scripted - and refused
// the interface, which calls the same verbs in process and passes an int like
// any Go caller would. The symptom was a drawn screen that could not be
// tapped and drawn buttons that could not be pressed, both of them answering
// "needs a node and a point" to a call that had one.
func numField(p any, name string) (float64, bool) {
	if m, ok := p.(map[string]any); ok {
		return toNumber(m[name])
	}
	return toNumber(p)
}

// toNumber is the one place that says what counts as a number here.
func toNumber(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int8:
		return float64(n), true
	case int16:
		return float64(n), true
	case int32:
		return float64(n), true
	case int64:
		return float64(n), true
	case uint:
		return float64(n), true
	case uint8:
		return float64(n), true
	case uint16:
		return float64(n), true
	case uint32:
		return float64(n), true
	case uint64:
		return float64(n), true
	case json.Number:
		f, err := n.Float64()
		return f, err == nil
	}
	return 0, false
}
