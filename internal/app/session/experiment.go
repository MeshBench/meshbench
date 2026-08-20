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
	"sync"
	"time"

	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/sim/engine"
	"github.com/MeshBench/meshbench/internal/world/scenario"
)

// VaryParams is every parameter an arm can be crossed on, in the order the
// Bench offers them, with the values it offers by default.
//
// One table so the panel, the verb and anything scripting this cannot drift:
// a dropdown listing a parameter the verb rejects is worse than not offering
// it, because the failure arrives after the arms have been built.
var VaryParams = []struct {
	Name, Label, Defaults string
}{
	{"path_hash_mode", "companion path hash", "0, 1, 2"},
	{"rep_path_hash", "repeater path hash", "0, 1, 2"},
	{"loop_detect", "loop.detect", "off, minimal, moderate, strict"},
	{"cad", "cad", "off, on"},
	{"repeater_version", "repeater firmware", ""},
	{"companion_version", "companion firmware", ""},
	{"spread_ms", "spread", "0, 5, 20"},
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

	// PerSecond is receptions in each second after the burst. The shape of a
	// flood, rather than its total: one clean wave and a long tail of retries
	// deliver the same count and are not the same network.
	PerSecond []int `json:"per_second,omitempty"`

	// AtRisk is the share of this cell's decodes that would have been lost had
	// every receiver been 1, 2, 3, 6 and 10 dB less sensitive - the bands in
	// engine.MarginEdgesDB.
	//
	// It is here because delivery totals cannot answer a question about the
	// receiver. A flood that loses an edge arrives by another, so an effect
	// real at link level is summed away before it reaches a row, and repeating
	// the run does not recover it. This is measured on the deliveries the cell
	// actually made, so it needs no second arm to compare against and no
	// assumption about what boosted gain is worth.
	AtRisk []float64 `json:"at_risk,omitempty"`
}

// experiment is the matrix and what has come back from it.
type experiment struct {
	mu       sync.Mutex
	Arms     []ExpArm `json:"arms"`
	Seeds    []uint64 `json:"seeds"`
	Senders  []string `json:"senders"`
	RunForMs uint32   `json:"run_for_ms"`
	SendAtMs uint32   `json:"send_at_ms"`

	// SpreadMs staggers the senders across the burst instead of firing them on
	// one instant. Zero is all at once, which is the sharpest test of
	// contention and the least like anything real.
	SpreadMs uint32 `json:"spread_ms"`
	// Bytes pads the message. Airtime scales with payload and airtime is what
	// collides, so a one-byte label and a full message are different
	// experiments. Zero sends the label alone.
	Bytes int `json:"bytes"`

	// Scope is the region every sender originates under. Empty sends unscoped.
	//
	// It matters far more than its size here suggests: repeaters carry flood
	// traffic only for regions they hold, so an unscoped run is carried by a
	// different set of them and measures a different network. workbench1 set it
	// per send; this inherited the loop without it.
	Scope string `json:"scope"`

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

// publish puts what is defined into the world, so every panel and every client
// sees one answer. The panel used to keep its own copy and disagreed with the
// session the moment anything else defined an arm.
func (e *experiment) publish(w *state.World) {
	w.ExperimentArms = e.armLabels()
	w.ExperimentSenders = append([]string(nil), e.Senders...)
	w.ExperimentRuns = e.runRows()
}

func registerExperiment(st *state.Store, s *Sim) {
	st.Handle("experiment.define", func(w *state.World, p any) (any, error) {
		defer func() { s.experiment().publish(w) }()
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
		if v, ok := numField(p, "spread_ms"); ok && v >= 0 {
			e.SpreadMs = uint32(v)
		}
		if v, ok := numField(p, "bytes"); ok && v >= 0 {
			e.Bytes = int(v)
		}
		if m, ok := p.(map[string]any); ok {
			if v, ok := m["scope"].(string); ok {
				e.Scope = v
			}
		}
		return e.describe(), nil
	})

	// experiment.vary is the same gesture an operator makes: choose a
	// parameter, type the values, get one arm per value.
	st.Handle("experiment.vary", func(w *state.World, p any) (any, error) {
		defer func() { s.experiment().publish(w) }()
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
		if len(values) == 0 {
			return nil, fmt.Errorf("experiment.vary needs values")
		}
		// A finished sweep's arms are the last question, not this one.
		e.results = nil

		// Crossed onto what is already there, not put in its place. Three path
		// hash sizes by two firmware versions is six arms, and varying twice
		// used to leave two - the second call throwing the first away without
		// saying so.
		//
		// An arm that names nothing is a placeholder rather than something to
		// cross onto, so the first vary from a fresh experiment gives clean
		// labels instead of "baseline · 1.17.0".
		base := e.Arms
		if len(base) == 0 || (len(base) == 1 && !base[0].names()) {
			base = []ExpArm{{}}
		}
		var out []ExpArm
		for _, b := range base {
			for _, v := range values {
				arm, seg, err := varied(b, param, v)
				if err != nil {
					return nil, err
				}
				arm.Label = joinLabel(b.Label, seg)
				out = append(out, arm)
			}
		}
		if len(out) == 0 {
			return nil, fmt.Errorf("experiment.vary produced no arms from %v", values)
		}
		e.Arms = out
		return e.describe(), nil
	})

	st.Handle("experiment.seeds", func(w *state.World, p any) (any, error) {
		defer func() { s.experiment().publish(w) }()
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
		defer func() { s.experiment().publish(w) }()
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

var _ = time.Now
var _ = engine.New

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
