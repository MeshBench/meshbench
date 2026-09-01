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
	// Applies live to a built engine - the switch lands on a whole-transmission
	// boundary - and to every engine built after.
	st.HandleSpec("rf.mode", state.Spec{
		What: "choose which physics decides reception, and stamp the choice " +
			"into the world so every snapshot, saved run and export says " +
			"which of the two models produced it",
		Params: []state.Param{
			{Name: "mode", Type: state.ParamString, Required: true, Primary: true,
				What: "calculated for link budgets against demodulator floors, " +
					"which is the fast model, or waveform for the full receive " +
					"chain of demodulation, FEC and CRC; any other value is " +
					"refused, and so is the empty string a caller who named " +
					"nothing sends"},
		},
		Returns: []string{"mode"},
		Example: &state.Example{
			Params:   "waveform",
			What:     "let the receive chain decide, rather than a link budget",
			Runnable: true,
		},
	}, func(w *state.World, p any) (any, error) {
		mode, _ := stringField(p, "mode")
		return setRFMode(w, s, mode)
	})

	// Shared logic called directly, never st.Do from inside a handler -
	// this goroutine IS the store, and asking it to do something is a wait
	// for yourself.
	st.HandleSpec("rf.toggle", state.Spec{
		What: "flip to whichever RF physics is not running, for a control " +
			"that is one button rather than a choice of two",
		Returns: []string{"mode"},
		Answers: "`mode` is the physics now in force, not the one it left.",
		Example: &state.Example{
			Params: map[string]any{}, What: "swap the physics for the other one",
			Runnable: true,
		},
	}, func(w *state.World, _ any) (any, error) {
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
	_ = s.savePrefs(w)
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
	st.HandleSpec("rf.realism", state.Spec{
		What: "price in the imperfections the channel otherwise leaves out - " +
			"crystal error, a delayed echo, fading, receiver implementation " +
			"loss and front-end clipping - because with all five at zero the " +
			"model is kinder than the air and every margin it reports is a " +
			"best case",
		Params: []state.Param{
			{Name: "osc_ppm", Type: state.ParamNumber,
				What: "worst-case crystal error in parts per million, each node " +
					"offset deterministically within it; absent leaves the " +
					"current value alone, and zero is a pair of perfect oscillators"},
			{Name: "multipath_db", Type: state.ParamNumber,
				What: "how far below the direct ray one delayed reflection " +
					"arrives, in decibels; absent leaves it alone, and zero is a " +
					"single clean path with nothing to cancel against"},
			{Name: "fading_hz", Type: state.ParamNumber,
				What: "how fast that echo's phase rotates over simulated time, " +
					"so a marginal link breathes; absent leaves it alone, and " +
					"zero holds the interference pattern still"},
			{Name: "impl_loss_db", Type: state.ParamNumber,
				What: "the receiver's shortfall from theory, applied as extra " +
					"receiver noise; absent leaves it alone, and zero credits " +
					"the receiver with its datasheet floor and nothing worse"},
			{Name: "saturation_dbm", Type: state.ParamNumber,
				What: "the level above which the front end clips, harmonics and " +
					"all; absent leaves it alone, and zero models a receiver " +
					"that never overloads however close the transmitter is"},
		},
		Returns: []string{"realism"},
		Answers: "`realism` is the whole switch set after the call, including " +
			"the switches this call did not name, so one knob can move without " +
			"restating the rest. The effects act on the waveform paths, so a " +
			"calculated run stores them and shows no change until the physics " +
			"is switched.",
		Example: &state.Example{
			Params:   map[string]any{"impl_loss_db": 2, "osc_ppm": 10},
			What:     "charge the receiver a realistic 2 dB and let the crystals disagree",
			Runnable: true,
		},
	}, func(w *state.World, p any) (any, error) {
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
		_ = s.savePrefs(w)
		w.Say("RF realism updated - the switches are stamped into every result")
		return map[string]any{"realism": r}, nil
	})

	st.HandleSpec("node.truerf", state.Spec{
		What: "give one receiver waveform verdicts inside a calculated run, so " +
			"the pair being studied is decided by the full receive chain while " +
			"the rest of the network stays on the fast model",
		Params: []state.Param{
			{Name: "node", Type: state.ParamString, Required: true, Primary: true,
				What: "the receiver that takes waveform verdicts; a name no node " +
					"has is refused rather than ignored"},
			{Name: "on", Type: state.ParamBool,
				What: "true to hold this node on waveform whatever the run mode; " +
					"absent means false, which puts it back on the run's own mode"},
		},
		Returns: []string{"node", "true_rf"},
		Example: &state.Example{
			Params:   map[string]any{"node": "West Lomond", "on": true},
			What:     "decide this one receiver honestly, at one node's cost",
			Runnable: true,
		},
	}, func(w *state.World, p any) (any, error) {
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
	st.HandleSpec("rf.environment", state.Spec{
		What: "point the session at a directory of environment tiles so both RF " +
			"modes price buildings into the path budget, or take it away again " +
			"and go back to bare earth",
		Params: []state.Param{
			{Name: "dir", Type: state.ParamString, Primary: true,
				What: "the tile directory, as tools/envgen or environ.fetch " +
					"wrote it; absent is refused unless on is false, because a " +
					"switch with nothing to switch on would silently leave the " +
					"model bare"},
			{Name: "on", Type: state.ParamBool,
				What: "false drops the environment and returns the model to bare " +
					"earth; absent or true expects a dir"},
		},
		Returns: []string{"environment"},
		Answers: "`environment` is the directory now in force, and empty means " +
			"bare earth. Every path loss already cached was priced without " +
			"buildings, so a live engine drops its link cache and the links are " +
			"measured again.",
		Example: &state.Example{
			Params:   map[string]any{"dir": "/var/lib/meshbench/environment/fife"},
			What:     "charge the paths for the buildings they cross",
			Runnable: false,
		},
	}, func(w *state.World, p any) (any, error) {
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
			_ = s.savePrefs(w)
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
		_ = s.savePrefs(w)
		w.Say("environment loaded from " + dir + " - buildings now price the path in both modes")
		return map[string]any{"environment": dir}, nil
	})
}
