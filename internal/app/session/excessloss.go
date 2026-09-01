// The calibration term the bare-earth model does not contain.
//
// The old workbench carried an excess path loss, set from the Validate panel
// once real observations had been compared against the model, and passed it
// into every engine it built. This build hardcoded the engine config without
// it, so it was permanently zero and there was no way to set it - which is why
// a path across the Lomond ridge closes here with 4.5 dB to spare when the
// real network cannot make it at all.
//
// Zero is a defensible default, but only when it is stated and settable. The
// menu bar already says results are a best case; this is the number that makes
// that true.
package session

import (
	"fmt"

	"github.com/MeshBench/meshbench/internal/app/state"
)

func registerExcessLoss(st *state.Store, s *Sim) {
	st.HandleSpec("rf.excess_loss", state.Spec{
		What: "add a flat calibration loss to every path, which is where the " +
			"clutter, body loss and multipath the bare-earth model has no term " +
			"for get paid for, and read back what the model is running with",
		Params: []state.Param{
			{Name: "db", Type: state.ParamNumber, Primary: true,
				What: "decibels of loss on top of the modelled path, fitted from " +
					"observations the study has already validated against; a " +
					"negative figure would add signal and is refused, and absent " +
					"changes nothing and only reports, leaving the default zero " +
					"that makes every margin a best case"},
		},
		Returns: []string{"db", "links"},
		Answers: "`links` is how many links the world is holding as this returns. " +
			"Path loss is cached per pair for the life of an engine, so setting " +
			"`db` over a loaded network rebuilds the engine and measures every " +
			"pair again on a worker: the links are empty until that finishes, " +
			"and this answers before it does.",
		Example: &state.Example{
			Params:   map[string]any{"db": 8},
			What:     "charge the 8 dB a validation run found the model was short",
			Runnable: false,
		},
	}, func(w *state.World, p any) (any, error) {
		if v, ok := numField(p, "db"); ok {
			if v < 0 {
				return nil, fmt.Errorf("excess loss is a loss: %.1f dB would add signal", v)
			}
			s.excessLossDB, s.excessSet = v, true
			w.ExcessLossDB, w.Calibrated = v, true
			if len(s.nodes) > 0 {
				// Rebuilt, because path loss is cached per pair for the life
				// of an engine - terrain does not move, so the cache never
				// expires on its own. Measured in the background, because on
				// a 166-node import that is 13,695 terrain profiles and doing
				// them here stops the store answering anything.
				if err := s.rebuild(w); err != nil {
					return nil, err
				}
				w.Links = nil
				s.warm(st, len(s.nodes))
			}
			w.Say(fmt.Sprintf("excess path loss %.1f dB", v))
		}
		return map[string]any{"db": s.excessLossDB, "links": len(w.Links)}, nil
	})
}
