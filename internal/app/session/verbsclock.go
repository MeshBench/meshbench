// The clock: play, pause, step, and how fast a tick advances.
package session

import (
	"fmt"

	"github.com/MeshBench/meshbench/internal/app/state"
)

// playLine is what the status bar says when a run starts, which is the last
// thing anybody reads before deciding the simulator is broken.
//
// A warm held for terrain consent leaves the matrix empty, and an empty
// matrix means no node can hear any other: every transmission is real and
// reaches nobody. The workbench says so when the network opens - "no link
// has been measured" - and then playing overwrote it with one word, so by
// the time somebody started firmware and typed advert the only explanation
// on screen had gone. Fifty-six nodes up, a console answering OK, and
// nothing received: reported as an advert no node receives, which is
// exactly what it is.
func (s *Sim) playLine() string {
	if measured, held := s.linksMeasured(); !measured && held {
		return "playing, but no link has been measured: the warm stopped to " +
			"ask whether terrain may be downloaded, so every transmission " +
			"will reach nobody. Allow it in Configuration > System, or run " +
			"terrain.allow, then rewarm links"
	}
	return "playing"
}

func registerClockVerbs(st *state.Store, s *Sim) {
	st.Handle("sim.play", func(w *state.World, _ any) (any, error) {
		w.Playing = true
		w.Say(s.playLine())
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
			w.Say(s.playLine())
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
