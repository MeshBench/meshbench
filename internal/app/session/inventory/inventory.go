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
	st.HandleSpec("nodes.list", state.Spec{
		What: "read back the whole network as it stands, which is what anything " +
			"automated does first and the only way to see what a scenario built " +
			"by a script actually got",
		Returns: []string{"nodes", "count"},
		Answers: "A row per node under `nodes` and `count` beside them, so a " +
			"caller need not measure the list to know how big the network is. " +
			"Each row carries two boards, which are two facts: `board` is what " +
			"the node is and `firmware_board` what its image was built for, and " +
			"they come apart the moment a host build is pointed at a T-Deck. " +
			"There is no limit and no paging, so an imported deployment answers " +
			"with all of it.",
		Example: &state.Example{
			Params: map[string]any{}, What: "see what is in the scenario",
			Runnable: true,
		},
	}, func(w *state.World, _ any) (any, error) {
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
				"sent":           n.Sent, "heard": n.Heard,
				"selected": n.Selected,
			})
		}
		return map[string]any{"nodes": out, "count": len(out)}, nil
	})

	st.HandleSpec("events.recent", state.Spec{
		What: "read the end of the event log, which is what a caller polls a " +
			"run with rather than asking for the whole thing every second",
		Params: []state.Param{
			{Name: "limit", Type: state.ParamNumber, Primary: true,
				What: "how many of the most recent events to return; anything " +
					"not a positive number leaves it at 50, and asking for more " +
					"than the store keeps returns what there is"},
		},
		Returns: []string{"events", "total", "shown"},
		Answers: "`shown` is how many rows came back and `total` how many the " +
			"run has produced since the engine was built, which is much larger: " +
			"the store keeps a bounded tail, not the whole log, so `total` " +
			"cannot be used to page backwards. An event whose signal-to-noise " +
			"ratio has no finite value comes back with `snr_db` null.",
		Example: &state.Example{
			Params: map[string]any{"limit": 20}, What: "poll what has just happened",
			Runnable: true,
		},
	}, func(w *state.World, p any) (any, error) {
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

	st.HandleSpec("events.dump", state.Spec{
		What: "write the event log to disk as NDJSON, one event per line, " +
			"because a run's log is appended to and read back a line at a time " +
			"and a single JSON array can be neither streamed nor tailed",
		Params: []state.Param{
			{Name: "path", Type: state.ParamString, Primary: true,
				What: "where to write; absent it goes to meshbench-events.ndjson " +
					"in the temporary directory, and an existing file is " +
					"overwritten rather than appended to"},
		},
		Returns: []string{"path", "written", "total"},
		Answers: "`written` is how many lines the file got and `total` how many " +
			"the run has produced. They differ on any long run, because the " +
			"store keeps a bounded tail rather than the whole log, and the " +
			"difference is not the file being truncated by a bug. An event " +
			"whose signal-to-noise ratio has no finite value is written with " +
			"`snr_db` null, JSON having no way to say infinity.",
		Example: &state.Example{
			Params:   map[string]any{"path": "/tmp/run-events.ndjson"},
			What:     "keep a run's log for something else to read",
			Runnable: false,
		},
	}, func(w *state.World, p any) (any, error) {
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
