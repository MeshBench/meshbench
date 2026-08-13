// A/B experiments, run here rather than in the old workbench.
//
// The eleven experiment verbs, on this build's own runner. The arms carry
// firmware versions and the cells run real MeshCore, because an arm that pins
// a version and then runs the engine's own relay logic measures nothing about
// that version - which is the whole question these verbs exist to answer.
//
// Three things decide whether the numbers mean anything, and each was got
// wrong at least once in the study this replaces, producing entirely plausible
// results:
//
//   - a node keeps its settings between runs exactly as hardware does, so
//     every arm gets storage of its own or it loads the previous arm's;
//   - the flood must be fired at the same simulated instant in each arm;
//   - the seed must vary across runs of one arm, or the spread being reported
//     is one draw repeated.
//
// They are enforced here rather than documented.
package session

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/A13xB0/meshcoresim/internal/engine"
	"github.com/A13xB0/meshcoresim/internal/gui/state"
	"github.com/A13xB0/meshcoresim/internal/scenario"
)

// ExpArm is one configuration under test.
type ExpArm struct {
	Label            string `json:"label"`
	RepeaterVersion  string `json:"repeater_version"`
	CompanionVersion string `json:"companion_version"`
}

// ExpResult is one arm at one seed.
type ExpResult struct {
	Arm       string  `json:"arm"`
	Seed      uint64  `json:"seed"`
	TX        int     `json:"tx"`
	RX        int     `json:"rx"`
	Delivered int     `json:"delivered"`
	Redundant int     `json:"redundant"`
	Collided  int     `json:"collisions"`
	AirtimeMs float64 `json:"airtime_ms"`
	// Firmware is how many processes this cell actually started, so a cell
	// that measured nothing is distinguishable from one that ran.
	Firmware int    `json:"firmware"`
	Err      string `json:"err,omitempty"`
}

// experiment is the matrix and what has come back from it.
type experiment struct {
	mu       sync.Mutex
	Arms     []ExpArm `json:"arms"`
	Seeds    []uint64 `json:"seeds"`
	Senders  []string `json:"senders"`
	RunForMs uint32   `json:"run_for_ms"`
	SendAtMs uint32   `json:"send_at_ms"`

	running bool
	cancel  context.CancelFunc
	results []ExpResult
	log     []string
	status  string
}

func (e *experiment) runsTotal() int { return len(e.Arms) * len(e.Seeds) }

func (e *experiment) logf(format string, a ...any) {
	e.log = append(e.log, fmt.Sprintf(format, a...))
	if len(e.log) > 200 {
		e.log = e.log[len(e.log)-200:]
	}
}

func (s *Sim) experiment() *experiment {
	if s.exp == nil {
		s.exp = &experiment{
			Arms:     []ExpArm{{Label: "baseline"}},
			Seeds:    []uint64{1, 2, 3, 4},
			RunForMs: 90_000, SendAtMs: 30_000,
		}
	}
	return s.exp
}

func registerExperiment(st *state.Store, s *Sim) {
	st.Handle("experiment.define", func(w *state.World, p any) (any, error) {
		e := s.experiment()
		if m, ok := p.(map[string]any); ok {
			if xs, ok := m["arms"].([]any); ok && len(xs) > 0 {
				e.Arms = e.Arms[:0]
				for _, x := range xs {
					am, _ := x.(map[string]any)
					arm := ExpArm{}
					arm.Label, _ = am["label"].(string)
					arm.RepeaterVersion, _ = am["repeater_version"].(string)
					arm.CompanionVersion, _ = am["companion_version"].(string)
					if arm.Label == "" {
						arm.Label = arm.RepeaterVersion
					}
					e.Arms = append(e.Arms, arm)
				}
			}
			if xs, ok := m["seeds"].([]any); ok && len(xs) > 0 {
				e.Seeds = e.Seeds[:0]
				for _, x := range xs {
					if v, ok := x.(float64); ok {
						e.Seeds = append(e.Seeds, uint64(v))
					}
				}
			}
			if xs, ok := m["senders"].([]any); ok {
				e.Senders = e.Senders[:0]
				for _, x := range xs {
					if v, ok := x.(string); ok {
						e.Senders = append(e.Senders, v)
					}
				}
			}
		}
		if v, ok := numField(p, "run_for_ms"); ok && v > 0 {
			e.RunForMs = uint32(v)
		}
		if v, ok := numField(p, "send_at_ms"); ok && v > 0 {
			e.SendAtMs = uint32(v)
		}
		return e.describe(), nil
	})

	// experiment.vary is the same gesture an operator makes: choose a
	// parameter, type the values, get one arm per value.
	st.Handle("experiment.vary", func(w *state.World, p any) (any, error) {
		e := s.experiment()
		param, _ := stringField(p, "parameter")
		var values []string
		if m, ok := p.(map[string]any); ok {
			if xs, ok := m["values"].([]any); ok {
				for _, x := range xs {
					if v, ok := x.(string); ok {
						values = append(values, v)
					}
				}
			}
		}
		if param != "repeater_version" && param != "companion_version" {
			return nil, fmt.Errorf(
				"this build varies repeater_version and companion_version; %q needs the study engine", param)
		}
		if len(values) == 0 {
			return nil, fmt.Errorf("experiment.vary needs values")
		}
		// A finished sweep's arms are the last question, not this one.
		e.results = nil
		e.Arms = e.Arms[:0]
		for _, v := range values {
			arm := ExpArm{Label: v}
			if param == "repeater_version" {
				arm.RepeaterVersion = v
			} else {
				arm.CompanionVersion = v
			}
			e.Arms = append(e.Arms, arm)
		}
		return e.describe(), nil
	})

	st.Handle("experiment.seeds", func(w *state.World, p any) (any, error) {
		e := s.experiment()
		var seeds []uint64
		if m, ok := p.(map[string]any); ok {
			if xs, ok := m["seeds"].([]any); ok {
				for _, x := range xs {
					if v, ok := x.(float64); ok {
						seeds = append(seeds, uint64(v))
					}
				}
			}
		}
		if len(seeds) == 0 {
			return nil, fmt.Errorf("experiment.seeds needs seeds")
		}
		e.Seeds = seeds
		return e.describe(), nil
	})

	st.Handle("experiment.senders", func(w *state.World, p any) (any, error) {
		e := s.experiment()
		if m, ok := p.(map[string]any); ok {
			if xs, ok := m["senders"].([]any); ok {
				e.Senders = e.Senders[:0]
				for _, x := range xs {
					if v, ok := x.(string); ok {
						e.Senders = append(e.Senders, v)
					}
				}
			}
		}
		return e.describe(), nil
	})

	st.Handle("experiment.base", func(w *state.World, p any) (any, error) {
		e := s.experiment()
		// Deliberately narrow. In the old workbench a base repeater_version
		// overrode a per-node pin and left the room server looking for a role
		// its build does not publish, so one node of fifty-six failed to start
		// and the arm looked like a firmware regression.
		if v, ok := stringField(p, "run_for_ms"); ok {
			_ = v
		}
		if v, ok := numField(p, "run_for_ms"); ok && v > 0 {
			e.RunForMs = uint32(v)
		}
		if v, ok := numField(p, "send_at_ms"); ok && v > 0 {
			e.SendAtMs = uint32(v)
		}
		return e.describe(), nil
	})

	st.Handle("experiment.state", func(w *state.World, _ any) (any, error) {
		e := s.experiment()
		e.mu.Lock()
		defer e.mu.Unlock()
		out := e.describe()
		out["running"] = e.running
		out["done"] = len(e.results)
		out["status"] = e.status
		if n := len(e.log); n > 0 {
			out["log"] = e.log[max(0, n-12):]
		}
		return out, nil
	})

	st.Handle("experiment.start", func(w *state.World, _ any) (any, error) {
		e := s.experiment()
		e.mu.Lock()
		if e.running {
			e.mu.Unlock()
			return nil, fmt.Errorf("an experiment is already running")
		}
		if len(s.nodes) == 0 {
			e.mu.Unlock()
			return nil, fmt.Errorf("no network loaded")
		}
		if len(e.Senders) == 0 {
			e.mu.Unlock()
			return nil, fmt.Errorf("experiment.start needs at least one sender")
		}
		e.running, e.results, e.status = true, nil, "starting"
		ctx, cancel := context.WithCancel(context.Background())
		e.cancel = cancel
		nodes := append([]scenario.Node(nil), s.nodes...)
		e.mu.Unlock()

		w.Jobs = append(w.Jobs, state.Job{
			ID: "experiment", What: "running arms", Total: e.runsTotal()})
		go s.runExperiment(ctx, st, e, nodes)
		return map[string]any{"running": true, "runs": e.runsTotal()}, nil
	})

	st.Handle("experiment.stop", func(w *state.World, _ any) (any, error) {
		e := s.experiment()
		e.mu.Lock()
		was := e.running
		if e.cancel != nil {
			e.cancel()
		}
		e.running = false
		done := len(e.results)
		e.mu.Unlock()
		w.Jobs = finishJob(w.Jobs, "experiment")
		w.Say("experiment stopped")
		return map[string]any{"stopped": was, "done": done, "total": e.runsTotal()}, nil
	})

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
		out := map[string]any{"runs": runs, "arms": e.summarise()}
		if w := e.notAResultYet(); w != "" {
			out["warning"] = w
		}
		return out, nil
	})

	st.Handle("experiment.compare", func(w *state.World, p any) (any, error) {
		e := s.experiment()
		a, _ := stringField(p, "arm_a")
		b, _ := stringField(p, "arm_b")
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

func (e *experiment) describe() map[string]any {
	return map[string]any{
		"arms": len(e.Arms), "seeds": len(e.Seeds), "senders": len(e.Senders),
		"runs": e.runsTotal(), "run_for_ms": e.RunForMs, "send_at_ms": e.SendAtMs,
	}
}

// notAResultYet is the honesty check: what would make these numbers not mean
// what they appear to.
func (e *experiment) notAResultYet() string {
	switch {
	case len(e.results) == 0:
		return "nothing has run yet"
	case len(e.Seeds) < 2:
		return "one seed: this is one draw, not a spread"
	case len(e.Arms) < 2:
		return "one arm: there is nothing to compare it against"
	}
	for _, r := range e.results {
		if r.Err != "" {
			return "at least one run failed: " + r.Err
		}
	}
	return ""
}

func (e *experiment) summarise() []map[string]any {
	by := map[string][]ExpResult{}
	for _, r := range e.results {
		by[r.Arm] = append(by[r.Arm], r)
	}
	names := make([]string, 0, len(by))
	for k := range by {
		names = append(names, k)
	}
	sort.Strings(names)
	out := make([]map[string]any, 0, len(names))
	for _, name := range names {
		rs := by[name]
		var tx, rx, del, red, coll, air float64
		for _, r := range rs {
			tx += float64(r.TX)
			rx += float64(r.RX)
			del += float64(r.Delivered)
			red += float64(r.Redundant)
			coll += float64(r.Collided)
			air += r.AirtimeMs
		}
		n := float64(len(rs))
		out = append(out, map[string]any{
			"arm": name, "runs": len(rs),
			"tx": tx / n, "rx": rx / n, "delivered": del / n,
			"redundant": red / n, "collisions": coll / n,
			"airtime_ms": air / n,
			"rx_spread":  spreadOf(rs),
		})
	}
	return out
}

// spreadOf is the range of receptions across seeds, which is what says whether
// a difference between arms is bigger than the noise within one.
func spreadOf(rs []ExpResult) float64 {
	if len(rs) < 2 {
		return 0
	}
	lo, hi := float64(rs[0].RX), float64(rs[0].RX)
	for _, r := range rs {
		v := float64(r.RX)
		if v < lo {
			lo = v
		}
		if v > hi {
			hi = v
		}
	}
	return hi - lo
}

var _ = time.Now
var _ = engine.New
