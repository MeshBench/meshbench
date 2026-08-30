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

	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/rf/environ"
	"github.com/MeshBench/meshbench/internal/sim/engine"
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
		return setRFMode(w, s, mode)
	})

	// rf.toggle: the chrome's one-click flip between the two physics.
	// Shared logic called directly, never st.Do from inside a handler -
	// this goroutine IS the store, and asking it to do something is a wait
	// for yourself.
	st.Handle("rf.toggle", func(w *state.World, _ any) (any, error) {
		next := "waveform"
		if s.rfMode == "waveform" {
			next = "calculated"
		}
		return setRFMode(w, s, next)
	})
}

// setRFMode is the one place the physics flips: validation, the engine, the
// prefs, and the announcement.
func setRFMode(w *state.World, s *Sim, mode string) (any, error) {
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
}

// engineRealism translates the panel's switch set to the engine's.
func engineRealism(r state.RFRealism) engine.Realism {
	return engine.Realism{
		OscillatorPPM: r.OscPPM, MultipathEchoDB: r.MultipathDB,
		FadingHz: r.FadingHz, ImplementationLossDB: r.ImplLossDB,
		SaturationDBm: r.SaturationDBm,
	}
}

func registerRFRealism(st *state.Store, s *Sim) {
	// rf.realism: the imperfection switches, applied live and persisted.
	// Absent fields are left alone, so one knob can move without restating
	// the rest.
	st.Handle("rf.realism", func(w *state.World, p any) (any, error) {
		r := s.realism
		if v, ok := numField(p, "osc_ppm"); ok {
			r.OscPPM = v
		}
		if v, ok := numField(p, "multipath_db"); ok {
			r.MultipathDB = v
		}
		if v, ok := numField(p, "fading_hz"); ok {
			r.FadingHz = v
		}
		if v, ok := numField(p, "impl_loss_db"); ok {
			r.ImplLossDB = v
		}
		if v, ok := numField(p, "saturation_dbm"); ok {
			r.SaturationDBm = v
		}
		s.realism = r
		if s.eng != nil {
			s.eng.SetRealism(engineRealism(r))
		}
		w.RFRealism = r
		s.prefs.OscPPM, s.prefs.MultipathDB, s.prefs.FadingHz = r.OscPPM, r.MultipathDB, r.FadingHz
		s.prefs.ImplLossDB, s.prefs.SaturationDBm = r.ImplLossDB, r.SaturationDBm
		s.savePrefs()
		w.Say("RF realism updated - the switches are stamped into every result")
		return map[string]any{"realism": r}, nil
	})

	// node.truerf: the hybrid flag - waveform verdicts at one receiver
	// inside a calculated run.
	st.Handle("node.truerf", func(w *state.World, p any) (any, error) {
		name, _ := stringField(p, "node")
		on, _ := boolField(p, "on")
		found := false
		for i := range s.nodes {
			if s.nodes[i].Name == name {
				s.nodes[i].TrueRF = on
				found = true
			}
		}
		if !found {
			return nil, noSuchNode(name)
		}
		if s.eng != nil {
			s.eng.SetTrueRF(name, on)
		}
		for i := range w.Nodes {
			if w.Nodes[i].Name == name {
				w.Nodes[i].TrueRF = on
			}
		}
		if on {
			w.Say(name + " now takes waveform verdicts, whatever the run mode")
		} else {
			w.Say(name + " is back on the run's own RF mode")
		}
		return map[string]any{"node": name, "true_rf": on}, nil
	})
}

// registerRFEnvironment wires the buildings switch: point the session at an
// environment tile directory (built by tools/envgen) and both RF modes start
// pricing buildings into the path budget. Off is bare earth, exactly as
// before.
func registerRFEnvironment(st *state.Store, s *Sim) {
	st.Handle("rf.environment", func(w *state.World, p any) (any, error) {
		dir, _ := stringField(p, "dir")
		if on, ok := boolField(p, "on"); ok && !on {
			s.envDir = ""
			s.envView = nil
			if s.eng != nil {
				s.eng.Env = nil
				s.eng.DropLinkCache()
			}
			w.RFEnvironment = ""
			s.prefs.EnvironmentDir = ""
			s.savePrefs()
			w.Say("environment off - the model is bare earth again, and says so")
			return map[string]any{"environment": ""}, nil
		}
		if dir == "" {
			return nil, fmt.Errorf("rf.environment needs a dir (tiles from tools/envgen) or on:false")
		}
		s.envDir = dir
		s.envView = nil
		store := environ.OpenTiles(dir)
		if s.eng != nil {
			s.eng.Env = store
			// Every cached loss was priced without buildings; keeping any
			// would mix two physics in one matrix.
			s.eng.DropLinkCache()
		}
		w.RFEnvironment = dir
		s.prefs.EnvironmentDir = dir
		s.savePrefs()
		w.Say("environment loaded from " + dir + " - buildings now price the path in both modes")
		return map[string]any{"environment": dir}, nil
	})
}
