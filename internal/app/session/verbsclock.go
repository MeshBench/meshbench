// The clock: play, pause, step, and how fast a tick advances.
package session

import (
	"fmt"

	"github.com/MeshBench/meshbench/internal/app/state"
)

func registerClockVerbs(st *state.Store, s *Sim) {
	st.HandleSpec("sim.play", state.Spec{
		What: "let the clock run, without touching firmware or waiting for the " +
			"links to be measured",
		Returns: []string{"playing"},
		Answers: "`playing` is always true: this verb sets the clock going " +
			"rather than reporting whether it could.",
		Example: &state.Example{
			Params: map[string]any{}, What: "start the clock", Runnable: true,
		},
	}, func(w *state.World, _ any) (any, error) {
		w.Playing = true
		w.Say("playing")
		return map[string]any{"playing": true}, nil
	})

	st.HandleSpec("sim.pause", state.Spec{
		What: "stop the clock where it is, leaving firmware running and the " +
			"engine built",
		Returns: []string{"playing"},
		Example: &state.Example{
			Params: map[string]any{}, What: "hold the run still", Runnable: true,
		},
	}, func(w *state.World, _ any) (any, error) {
		w.Playing = false
		w.Say("paused")
		return map[string]any{"playing": false}, nil
	})

	st.HandleSpec("sim.toggle", state.Spec{
		What:    "play if paused and pause if playing, for a control that is one key",
		Returns: []string{"playing"},
		Answers: "`playing` is the state it has just moved to, not the one it was in.",
		Example: &state.Example{
			Params: map[string]any{}, What: "flip the clock", Runnable: true,
		},
	}, func(w *state.World, _ any) (any, error) {
		// One control for both, because play and pause are one thought.
		w.Playing = !w.Playing
		if w.Playing {
			w.Say("playing")
		} else {
			w.Say("paused")
		}
		return map[string]any{"playing": w.Playing}, nil
	})

	st.HandleSpec("sim.step", state.Spec{
		What: "advance the engine by one tick while the clock is stopped, which " +
			"is how a paused run is inspected between packets",
		Returns: []string{"now_ms"},
		Answers: "`now_ms` is the simulated clock after the tick. One tick is " +
			"whatever `sim.speed` last set, not one millisecond.",
		Example: &state.Example{
			Params: map[string]any{}, What: "move on one tick", Runnable: true,
		},
	}, func(w *state.World, _ any) (any, error) {
		// One step whether or not it is playing: stepping a paused simulation
		// is the whole point of a step control.
		if w.Tick != nil {
			w.Tick(0)
		}
		w.Say(fmt.Sprintf("stepped to %.2f s", float64(w.NowMs)/1000))
		return map[string]any{"now_ms": w.NowMs}, nil
	})

	st.HandleSpec("sim.faster", state.Spec{
		What: "double how much simulated time one tick covers, trading detail " +
			"for how long a run takes to watch",
		Returns: []string{"step_ms"},
		Answers: "`step_ms` is the new tick length. Nothing bounds it: enough " +
			"calls will step past whole transmissions.",
		Example: &state.Example{
			Params: map[string]any{}, What: "run twice as coarsely", Runnable: true,
		},
	}, func(w *state.World, _ any) (any, error) {
		st.SetStepMs(st.StepMs() * 2)
		w.Say(fmt.Sprintf("%d ms per tick", st.StepMs()))
		return map[string]any{"step_ms": st.StepMs()}, nil
	})

	st.HandleSpec("sim.slower", state.Spec{
		What:    "halve how much simulated time one tick covers",
		Returns: []string{"step_ms"},
		Answers: "`step_ms` is the new tick length, which reaches zero after " +
			"enough calls and stops the clock advancing at all.",
		Example: &state.Example{
			Params: map[string]any{}, What: "run twice as finely", Runnable: true,
		},
	}, func(w *state.World, _ any) (any, error) {
		st.SetStepMs(st.StepMs() / 2)
		w.Say(fmt.Sprintf("%d ms per tick", st.StepMs()))
		return map[string]any{"step_ms": st.StepMs()}, nil
	})
}
