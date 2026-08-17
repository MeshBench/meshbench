// The clock: play, pause, step, and how fast a tick advances.
package session

import (
	"fmt"

	"github.com/MeshBench/meshbench/internal/gui/state"
)

func registerClockVerbs(st *state.Store, s *Sim) {
	st.Handle("sim.play", func(w *state.World, _ any) (any, error) {
		w.Playing = true
		w.Say("playing")
		return map[string]any{"playing": true}, nil
	})

	st.Handle("sim.pause", func(w *state.World, _ any) (any, error) {
		w.Playing = false
		w.Say("paused")
		return map[string]any{"playing": false}, nil
	})

	st.Handle("sim.toggle", func(w *state.World, _ any) (any, error) {
		// One control for both, because play and pause are one thought.
		w.Playing = !w.Playing
		if w.Playing {
			w.Say("playing")
		} else {
			w.Say("paused")
		}
		return map[string]any{"playing": w.Playing}, nil
	})

	st.Handle("sim.step", func(w *state.World, _ any) (any, error) {
		// One step whether or not it is playing: stepping a paused simulation
		// is the whole point of a step control.
		if w.Tick != nil {
			w.Tick(0)
		}
		w.Say(fmt.Sprintf("stepped to %.2f s", float64(w.NowMs)/1000))
		return map[string]any{"now_ms": w.NowMs}, nil
	})

	st.Handle("sim.faster", func(w *state.World, _ any) (any, error) {
		st.SetStepMs(st.StepMs() * 2)
		w.Say(fmt.Sprintf("%d ms per tick", st.StepMs()))
		return map[string]any{"step_ms": st.StepMs()}, nil
	})

	st.Handle("sim.slower", func(w *state.World, _ any) (any, error) {
		st.SetStepMs(st.StepMs() / 2)
		w.Say(fmt.Sprintf("%d ms per tick", st.StepMs()))
		return map[string]any{"step_ms": st.StepMs()}, nil
	})
}
