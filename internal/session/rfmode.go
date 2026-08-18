// Which physics decides reception.
//
// One verb, one field, one honest rule: the mode is stamped into the world so
// every snapshot, every saved run and every export says which model produced
// it. The two modes share the whole scenario - terrain, budgets, antennas,
// timing, seed - so the same run can be replayed under either and the diff is
// a measurement of where the fast model lies, not an argument.
package session

import (
	"fmt"

	"github.com/MeshBench/meshbench/internal/engine"
	"github.com/MeshBench/meshbench/internal/gui/state"
)

// rfModeOf normalises the stored string; empty is calculated, the default
// every existing scenario keeps.
func rfModeOf(s string) engine.RFMode {
	if s == "waveform" {
		return engine.RFWaveform
	}
	return engine.RFCalculated
}

func registerRFMode(st *state.Store, s *Sim) {
	// rf.mode: choose the physics. Applies live to a built engine - the
	// switch lands on a whole-transmission boundary - and to every engine
	// built after.
	st.Handle("rf.mode", func(w *state.World, p any) (any, error) {
		mode, _ := stringField(p, "mode")
		switch mode {
		case "calculated", "waveform":
		default:
			return nil, fmt.Errorf("rf.mode is calculated or waveform, not %q", mode)
		}
		s.rfMode = mode
		if s.eng != nil {
			s.eng.SetRFMode(rfModeOf(mode))
		}
		w.RFMode = mode
		s.prefs.RFMode = mode
		s.savePrefs()
		if mode == "waveform" {
			w.Say("waveform RF: reception is decided by the full receive " +
				"chain - demodulation, FEC and CRC")
		} else {
			w.Say("calculated RF: reception is decided by link budgets and " +
				"demodulator floors - the fast model")
		}
		return map[string]any{"mode": mode}, nil
	})
}
