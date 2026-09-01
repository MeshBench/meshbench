// What kind of run this is.
//
// Real firmware used to be a second button that had to be pressed before play,
// in the right order, or the run was a different simulation than the one
// intended - two decisions where the operator has one. The old workbench fixed
// that by making it a property of the run, set once beside the transport, with
// play honouring it. This build lost the coupling: play advanced a clock and
// nothing ever started.
package session

import (
	"github.com/MeshBench/meshbench/internal/app/state"
)

func registerRunKind(st *state.Store, s *Sim) {
	// sim.kind: on, play starts MeshCore on every node first.
	st.HandleSpec("sim.kind", state.Spec{
		What: "choose whether a run carries real MeshCore firmware or only the " +
			"channel, and report which it is now",
		Params: []state.Param{
			{Name: "real", Type: state.ParamBool, Primary: true,
				What: "true to start MeshCore on every node at play; " +
					"absent leaves the setting alone and only reads it"},
		},
		Returns: []string{"real", "running"},
		Answers: "`running` is how many nodes have firmware up right now, which " +
			"is not changed by setting `real`: it takes effect at the next play.",
		Example: &state.Example{
			Params: map[string]any{}, What: "ask what kind of run this is",
			Runnable: true,
		},
	}, func(w *state.World, p any) (any, error) {
		if v, ok := boolField(p, "real"); ok {
			w.RealFirmware = v
			if v {
				w.Say("play will start MeshCore on every node")
			} else {
				w.Say("play will run the channel without firmware")
			}
		}
		return map[string]any{
			"real": w.RealFirmware, "running": s.firmwareCount(),
		}, nil
	})

	// sim.start is what a play button presses.
	//
	// One verb rather than two, because the order mattered and getting it
	// wrong produced a plausible run of the wrong thing: firmware started
	// after play meant the first seconds had no relays in them.
	st.HandleSpec("sim.start", state.Spec{
		What: "what a play button presses, which is four different things: it " +
			"pauses a run already playing, declines while the links are still " +
			"being measured, brings MeshCore up on every node without playing " +
			"if this is a firmware run and nothing is up yet, and otherwise " +
			"starts the clock",
		Returns: []string{"playing", "warming", "starting_firmware", "started_firmware"},
		Answers: "`playing` is the only key always present. `warming` appears " +
			"when it declined because the link matrix is not measured yet, and " +
			"`starting_firmware` when it brought the mesh up instead of playing, " +
			"which is the case where a second call is needed.",
		Example: &state.Example{
			Params: map[string]any{}, What: "press play", Runnable: true,
		},
	}, func(w *state.World, _ any) (any, error) {
		if w.Playing {
			w.Playing = false
			w.Say("paused")
			return map[string]any{"playing": false}, nil
		}
		// A cold engine warms before it runs.
		//
		// Nothing is blocked while it does: the measurement is already a job
		// on its own goroutine, and this only declines to start the clock
		// until it has finished. The alternative is a run whose first packet
		// pays for the whole path-loss matrix on whichever thread delivered
		// it, which is what "it stalls the moment I send" was.
		if s.warming() {
			w.Say("warming up: the run starts when every link has been measured")
			return map[string]any{"playing": false, "warming": true}, nil
		}
		if w.RealFirmware && s.eng != nil && s.firmwareCount() == 0 && len(w.Nodes) > 0 {
			// Every node needs a build before any node starts.
			//
			// Starting anyway leaves half a mesh up and the run measuring a
			// network that does not exist, and the operator sees "starting
			// MeshCore" for ever with nothing saying which node had no
			// firmware. Say it before the first process is launched.
			if err := s.firmwareStartBlocker(); err != nil {
				return nil, err
			}
			// Called directly, not through st.Do: this handler already runs
			// on the store's goroutine, and asking the store to do something
			// from inside it is a wait for yourself.
			s.startFirmware(st, w.Seed)
			// Not playing yet, and not later either.
			//
			// Playing immediately started the clock against nodes that were
			// still attaching, so the store's ticker drove an engine whose
			// nodes had no process behind them and wedged - the status said
			// "starting MeshCore" and nothing ever moved again. Starting the
			// run for you once they were up fixed the wedge and introduced a
			// different confusion: one press, two things, a minute apart.
			// So this press brings the mesh up and says so, and the next one
			// starts the run.
			w.PendingPlay = true
			w.Say("starting MeshCore on every node - play again once they are up")
			return map[string]any{"playing": false, "starting_firmware": true}, nil
		}
		w.Playing = true
		w.Say("playing")
		return map[string]any{"playing": true, "started_firmware": false}, nil
	})
}

// boolField reads a boolean from a verb's parameters, which arrive either as a
// JSON object or as a bare value when the verb takes exactly one.
func boolField(p any, name string) (bool, bool) {
	switch v := p.(type) {
	case map[string]any:
		b, ok := v[name].(bool)
		return b, ok
	case bool:
		return v, true
	}
	return false, false
}
