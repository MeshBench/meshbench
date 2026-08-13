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

	"github.com/A13xB0/meshcoresim/internal/gui/state"
)

func registerExcessLoss(st *state.Store, s *Sim) {
	st.Handle("rf.excess_loss", func(w *state.World, p any) (any, error) {
		if v, ok := numField(p, "db"); ok {
			if v < 0 {
				return nil, fmt.Errorf("excess loss is a loss: %.1f dB would add signal", v)
			}
			s.excessLossDB, s.excessSet = v, true
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
