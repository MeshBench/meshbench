// Reading the network and the log from outside the window.
//
// Three verbs the old control socket had. They are what anything automated
// does first - list what is here, then watch what happened - and without them
// a caller can drive a run on this build but cannot find out what it did.
package inventory

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"

	"github.com/MeshBench/meshbench/internal/app/session"
	"github.com/MeshBench/meshbench/internal/app/state"
)

// registerInventory adds the read-only verbs.
func registerInventory(st *state.Store, s *session.Sim) {
	st.Handle("nodes.list", func(w *state.World, _ any) (any, error) {
		out := make([]map[string]any, 0, len(w.Nodes))
		for _, n := range w.Nodes {
			out = append(out, map[string]any{
				"name": n.Name, "kind": n.Kind,
				"lat": n.Lat, "lon": n.Lon, "height_m": n.HeightM,
				"tx_dbm": n.TxDBm, "regions": n.Regions,
				// Two boards, because they are two facts. "board" is what the
				// node is; "firmware_board" is what its image was built for.
				// They agree most of the time and come apart the moment
				// somebody points a host build at a T-Deck - and neither was
				// published at all, so a scenario built by a script could not
				// be read back with the hardware it had been given.
				"firmware": n.Firmware, "board": n.Hardware,
				"firmware_board": n.Board,
				// No sent or heard here. They were published as nought for
				// every node in every reply, because nothing wrote them: a
				// caller reading a number believes it, and there was no way to
				// tell that number from a node which genuinely had not
				// transmitted. nodes.stats answers this, from the engine's own
				// scoreboard.
				"selected": n.Selected,
			})
		}
		return map[string]any{"nodes": out, "count": len(out)}, nil
	})

	st.Handle("events.recent", func(w *state.World, p any) (any, error) {
		limit := 50
		if v, ok := session.NumField(p, "limit"); ok && v > 0 {
			limit = int(v)
		}
		from := len(w.Events) - limit
		if from < 0 {
			from = 0
		}
		return map[string]any{
			"events": eventsAsMaps(w.Events[from:]),
			"total":  w.EventTotal,
			"shown":  len(w.Events) - from,
		}, nil
	})

	st.Handle("events.dump", func(w *state.World, p any) (any, error) {
		path := session.SoleString(p)
		if m, ok := p.(map[string]any); ok {
			path, _ = m["path"].(string)
		}
		if path == "" {
			path = filepath.Join(os.TempDir(), "meshbench-events.ndjson")
		}
		f, err := os.Create(path)
		if err != nil {
			return nil, err
		}
		defer func() { _ = f.Close() }()
		// The engine's own ledger, not the snapshot's.
		//
		// The snapshot keeps a bounded tail because the tables draw from it,
		// and this wrote that tail: a run of 2732 events produced a file of
		// 2000 and said so honestly, which is not the same as being useful.
		// Writing to a file has no drawing budget to protect, and this is the
		// documented way to get a run's whole ledger out for analysis, so it
		// takes everything the engine still holds.
		//
		// It falls back to the snapshot when there is no live engine - a
		// loaded capture has a readout and nothing behind it - so a dump after
		// a run has finished still writes what is there.
		evs, total := w.Events, w.EventTotal
		if all, n := s.EventLedger(); all != nil {
			evs, total = all, n
		}
		enc := json.NewEncoder(f)
		for _, e := range evs {
			if err := enc.Encode(eventAsMap(e)); err != nil {
				return nil, fmt.Errorf("writing %s: %w", path, err)
			}
		}
		// The count written beside the count that exists, still: they can
		// differ when the engine has retired events of its own, and a caller
		// told "38,000" who receives fewer lines would reasonably think the
		// file was truncated by a bug.
		return map[string]any{
			"path": path, "written": len(evs), "total": total,
		}, nil
	})
}

func eventsAsMaps(evs []state.Event) []map[string]any {
	out := make([]map[string]any, 0, len(evs))
	for _, e := range evs {
		out = append(out, eventAsMap(e))
	}
	return out
}

func eventAsMap(e state.Event) map[string]any {
	m := map[string]any{
		"at_ms": e.AtMs, "kind": e.Kind, "from": e.From, "to": e.To,
		"message_id": e.MessageID, "packet_id": e.PacketID,
		"snr_db": e.SNRdB, "detail": e.Detail,
		// The class, which the cards and the filter chips are built on and
		// which never reached the wire. Kind is "tx"/"rx"/"miss"; the class
		// says which *kind* of miss - half duplex, interference, or simply
		// too quiet - and those are three different problems. Without it
		// every client-side filter on class matched nothing at all.
		"class": e.Class,
	}
	// A ratio against no noise at all is infinite, and JSON has no way to
	// say so: one such event failed the whole dump with "unsupported value:
	// -Inf", losing a hundred thousand good rows to one unrepresentable
	// number. Null is what JSON has for "no value", and a consumer reading
	// null as missing is right.
	if math.IsInf(e.SNRdB, 0) || math.IsNaN(e.SNRdB) {
		m["snr_db"] = nil
	}
	return m
}
