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
	"github.com/A13xB0/meshcoresim/internal/gui/state"
)

func registerRunKind(st *state.Store, s *Sim) {
	// sim.kind: on, play starts MeshCore on every node first.
	st.Handle("sim.kind", func(w *state.World, p any) (any, error) {
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
	st.Handle("sim.start", func(w *state.World, _ any) (any, error) {
		if w.Playing {
			w.Playing = false
			w.Say("paused")
			return map[string]any{"playing": false}, nil
		}
		started := false
		if w.RealFirmware && s.eng != nil && s.firmwareCount() == 0 && len(w.Nodes) > 0 {
			// Called directly, not through st.Do: this handler already runs
			// on the store's goroutine, and asking the store to do something
			// from inside it is a wait for yourself. It deadlocked the whole
			// session, which from outside looks exactly like a hung socket.
			s.startFirmware(st, w.Seed)
			started = true
			w.Say("starting MeshCore on every node, then running")
		}
		w.Playing = true
		if !started {
			w.Say("playing")
		}
		return map[string]any{"playing": true, "started_firmware": started}, nil
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
