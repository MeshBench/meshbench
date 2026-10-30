// Handing a node's radio a frame directly, for testing a firmware's receive
// path against receptions the channel would rarely produce on demand.
package session

import (
	"encoding/hex"
	"strings"

	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/firmware"
)

// deliverKinds names what radio.deliver can say about a reception, as
// virtual-sx1262 numbers the failures. "ok" is a frame that arrived intact.
var deliverKinds = map[string]firmware.FrameFailure{
	"ok":               0,
	"crc_error":        firmware.FailedCRC,
	"header_error":     firmware.FailedHeader,
	"no_crc":           firmware.FailedNoCRC,
	"next_before_read": firmware.FailedNextBeforeRead,
	"next_during_read": firmware.FailedNextDuringRead,
}

func registerRadioVerbs(st *state.Store, s *Sim) {
	st.Handle("radio.deliver", func(w *state.World, p any) (any, error) {
		m, _ := p.(map[string]any)
		name, _ := m["node"].(string)
		raw, _ := m["hex"].(string)
		as, _ := m["as"].(string)
		if as == "" {
			as = "ok"
		}
		why, known := deliverKinds[as]
		if !known {
			return nil, badParams("radio.deliver: as is one of ok, crc_error, header_error, no_crc, next_before_read, next_during_read, not %q", as)
		}
		frame, err := hex.DecodeString(strings.ReplaceAll(raw, " ", ""))
		if err != nil || len(frame) == 0 {
			return nil, badParams("radio.deliver: hex is the frame as hex digits")
		}
		at := -1
		for i := range w.Nodes {
			if w.Nodes[i].Name == name {
				at = i
			}
		}
		if at < 0 {
			return nil, UnknownNames("radio.deliver", w.Nodes, []string{name})
		}
		if s.Engine() == nil {
			return nil, ErrNoSimulation
		}
		// Not delivered is an answer, not an error: a node without firmware,
		// or whose chip predates the kind, was handed nothing.
		taken, err := s.Engine().DeliverFrame(at, frame, why)
		if err != nil {
			return nil, err
		}
		return map[string]any{"node": name, "as": as, "bytes": len(frame), "delivered": taken}, nil
	})
}
