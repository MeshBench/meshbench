// Reading a sweep back out: the table, the difference between two arms, and
// the file that outlives the session.
//
// Apart from the definition verbs because these three are the ones that have to
// hold the matrix's lock: the runner appends to the results from its own
// goroutine, so anything that reads them races the cell in flight unless it
// takes the lock, and anything that takes the lock must not then wait on the
// runner.
package session

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/MeshBench/meshbench/internal/app/state"
)

func registerExperimentResults(st *state.Store, s *Sim) {
	st.Handle("experiment.results", func(w *state.World, _ any) (any, error) {
		e := s.experiment()
		e.mu.Lock()
		defer e.mu.Unlock()
		runs := make([]map[string]any, 0, len(e.results))
		for _, r := range e.results {
			runs = append(runs, map[string]any{
				"arm": r.Arm, "seed": r.Seed, "tx": r.TX, "rx": r.RX,
				"delivered": r.Delivered, "redundant": r.Redundant,
				"collisions": r.Collided, "airtime_ms": r.AirtimeMs,
				"err": r.Err,
			})
		}
		sums := e.summarise()
		out := map[string]any{"runs": runs, "arms": sums}
		warn := e.notAResultYet()
		if warn != "" {
			out["warning"] = warn
		}
		w.Experiment = w.Experiment[:0]
		for _, m := range sums {
			// Read rather than asserted. These summaries travel as
			// map[string]any because the control socket serves them too, and
			// a key that is absent or holds another type would panic the
			// store's goroutine - taking the whole application with it - for
			// what is at worst one arm with a missing number.
			arm, _ := m["arm"].(string)
			w.Experiment = append(w.Experiment, state.ArmSummary{
				Arm:       arm,
				Runs:      mapInt(m, "runs"),
				Failed:    mapInt(m, "failed"),
				TX:        mapFloat(m, "tx"),
				RX:        mapFloat(m, "rx"),
				Delivered: mapFloat(m, "delivered"),
				Redundant: mapFloat(m, "redundant"),
				Collided:  mapFloat(m, "collisions"),
				AirtimeMs: mapFloat(m, "airtime_ms"),
				RXSpread:  mapFloat(m, "rx_spread"),
				PerSecond: e.perSecondFor(arm),
			})
		}
		w.ExperimentRuns = e.runRows()
		w.ExperimentWarning = warn
		if len(e.results) >= e.runsTotal() && e.runsTotal() > 0 && !e.running {
			w.ExperimentVerdict = e.verdict()
		} else {
			w.ExperimentVerdict = ""
		}
		return out, nil
	})

	st.Handle("experiment.compare", func(w *state.World, p any) (any, error) {
		e := s.experiment()
		a, _ := stringField(p, "arm_a")
		b, _ := namedField(p, "arm_b")
		e.mu.Lock()
		defer e.mu.Unlock()
		sums := e.summarise()
		var sa, sb map[string]any
		for _, s := range sums {
			if s["arm"] == a {
				sa = s
			}
			if s["arm"] == b {
				sb = s
			}
		}
		if sa == nil || sb == nil {
			return nil, fmt.Errorf("no results for %q and %q", a, b)
		}
		delta := map[string]any{}
		for _, k := range []string{"tx", "rx", "delivered", "redundant", "collisions"} {
			x, _ := sa[k].(float64)
			y, _ := sb[k].(float64)
			delta[k] = y - x
			if x != 0 {
				delta[k+"_pct"] = (y - x) / x * 100
			}
		}
		return map[string]any{"a": sa, "b": sb, "delta": delta,
			"note": "both arms carry the same excess path loss, so a difference " +
				"in direction is the firmware rather than the calibration"}, nil
	})

	st.Handle("experiment.export", func(w *state.World, p any) (any, error) {
		e := s.experiment()
		path, _ := stringField(p, "path")
		if path == "" {
			path = filepath.Join(os.TempDir(), "meshbench-experiment.json")
		}
		e.mu.Lock()
		b, err := json.MarshalIndent(map[string]any{
			"arms": e.Arms, "seeds": e.Seeds, "senders": e.Senders,
			"run_for_ms": e.RunForMs, "send_at_ms": e.SendAtMs,
			"results": e.results, "summary": e.summarise(),
		}, "", "  ")
		e.mu.Unlock()
		if err != nil {
			return nil, err
		}
		if err := os.WriteFile(path, b, 0o644); err != nil {
			return nil, err
		}
		w.Say("exported to " + path)
		return map[string]any{"path": path, "bytes": len(b)}, nil
	})
}

// mapFloat and mapInt read a summary field without asserting it.
//
// A summary is built in this package and consumed in it, so a missing key is a
// bug here rather than bad input - but the consequence of asserting is a panic
// on the store's goroutine, which ends the application, and the consequence of
// reading is a zero in one cell of a table. The second is the better failure
// for a number nobody has looked at yet.
func mapFloat(m map[string]any, key string) float64 {
	v, _ := m[key].(float64)
	return v
}

func mapInt(m map[string]any, key string) int {
	v, _ := m[key].(int)
	return v
}
