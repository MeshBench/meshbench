// Reading the network and the log from outside the window.
//
// Three verbs the old control socket had. They are what anything automated
// does first - list what is here, then watch what happened - and without them
// a caller can drive a run on this build but cannot find out what it did.
package session

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/MeshBench/meshbench/internal/gui/state"
)

// registerInventory adds the read-only verbs.
func registerInventory(st *state.Store, s *Sim) {
	// nodes.list: the network as it stands.
	st.Handle("nodes.list", func(w *state.World, _ any) (any, error) {
		out := make([]map[string]any, 0, len(w.Nodes))
		for _, n := range w.Nodes {
			out = append(out, map[string]any{
				"name": n.Name, "kind": n.Kind,
				"lat": n.Lat, "lon": n.Lon, "height_m": n.HeightM,
				"tx_dbm": n.TxDBm, "regions": n.Regions,
				"firmware": n.Firmware,
				"sent":     n.Sent, "heard": n.Heard,
				"selected": n.Selected,
			})
		}
		return map[string]any{"nodes": out, "count": len(out)}, nil
	})

	// events.recent: the tail, which is what a poller wants.
	//
	// The store keeps a bounded tail rather than the whole log, so this can
	// never return a run's worth of events by accident - a caller polling
	// every second on a long run would otherwise be handed millions.
	st.Handle("events.recent", func(w *state.World, p any) (any, error) {
		limit := 50
		if v, ok := numField(p, "limit"); ok && v > 0 {
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

	// events.dump: the tail as NDJSON on disk, one event per line.
	//
	// NDJSON because a run's log is appended to and read back a line at a
	// time; a single JSON array cannot be streamed and cannot be tailed.
	st.Handle("events.dump", func(w *state.World, p any) (any, error) {
		path := soleString(p)
		if m, ok := p.(map[string]any); ok {
			path, _ = m["path"].(string)
		}
		if path == "" {
			path = filepath.Join(os.TempDir(), "meshcoresim-events.ndjson")
		}
		f, err := os.Create(path)
		if err != nil {
			return nil, err
		}
		defer func() { _ = f.Close() }()
		enc := json.NewEncoder(f)
		for _, e := range w.Events {
			if err := enc.Encode(eventAsMap(e)); err != nil {
				return nil, fmt.Errorf("writing %s: %w", path, err)
			}
		}
		// The count written, not the count that exists: the store keeps a
		// bounded tail, and a caller told "38,000 events" who receives 2,000
		// lines would reasonably think the file was truncated by a bug.
		return map[string]any{
			"path": path, "written": len(w.Events), "total": w.EventTotal,
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
	return map[string]any{
		"at_ms": e.AtMs, "kind": e.Kind, "from": e.From, "to": e.To,
		"message_id": e.MessageID, "packet_id": e.PacketID,
		"snr_db": e.SNRdB, "detail": e.Detail,
	}
}
