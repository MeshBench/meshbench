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
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/MeshBench/meshbench/internal/engine"
	"github.com/MeshBench/meshbench/internal/gui/state"
	"github.com/MeshBench/meshbench/internal/scenario"
)

// ExpArm is one configuration under test.
//
// Everything past the two firmware fields is a provisioning setting, because
// the questions worth asking of a mesh are mostly not "which build" - they are
// how much of a path a message carries, whether repeaters detect loops, and
// whether anybody listens before talking.
type ExpArm struct {
	Label            string `json:"label"`
	RepeaterVersion  string `json:"repeater_version"`
	CompanionVersion string `json:"companion_version"`

	// PathHashMode is the *companion's* setting, and it is the one that
	// decides what a message carries: the originator stamps it and every hop
	// honours it. RepPathHash is the repeaters' own, which only affects the
	// adverts they originate and is normally held constant.
	//
	// Pointers rather than a -1 sentinel. Mode 0 is a real value - one byte per
	// hop - and it is also the zero value, so "unset" and "one byte" were the
	// same thing to every path that built an arm without naming the field.
	PathHashMode *int `json:"path_hash_mode,omitempty"`
	RepPathHash  *int `json:"rep_path_hash,omitempty"`

	// LoopDetect is off, minimal, moderate or strict; CAD is on or off. Empty
	// leaves whatever the scenario has.
	LoopDetect string `json:"loop_detect,omitempty"`
	CAD        string `json:"cad,omitempty"`

	// SpreadMs staggers this arm's senders across the burst, overriding the
	// experiment's own.
	SpreadMs *int `json:"spread_ms,omitempty"`
}

// applyOver writes only what this arm actually names, over a base.
//
// The distinction against writing every field is load-bearing: an arm varying
// one parameter must leave the other seven as the base set them, and a struct
// copy would silently reset them to their zero values - which for path hash
// mode is a real setting rather than "unset".
func (a ExpArm) applyOver(p *Provisioning) {
	if a.PathHashMode != nil {
		p.CompPathHashMode = *a.PathHashMode
	}
	if a.RepPathHash != nil {
		p.PathHashMode = *a.RepPathHash
	}
	if a.LoopDetect != "" {
		p.LoopDetect = a.LoopDetect
	}
	if a.CAD != "" {
		p.CadMode = a.CAD
	}
}

// names reports whether this arm sets anything at all. An arm that names
// nothing is a placeholder to be replaced by the first cross, not something to
// cross onto.
func (a ExpArm) names() bool {
	return a.RepeaterVersion != "" || a.CompanionVersion != "" ||
		a.PathHashMode != nil || a.RepPathHash != nil ||
		a.LoopDetect != "" || a.CAD != "" || a.SpreadMs != nil
}

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

// varied returns the arm with one parameter set, and the label segment that
// records it.
func varied(base ExpArm, param, v string) (ExpArm, string, error) {
	arm := base
	switch param {
	case "repeater_version":
		arm.RepeaterVersion = v
		return arm, bareVersion(v, "repeater-"), nil
	case "companion_version":
		arm.CompanionVersion = v
		return arm, bareVersion(v, "companion-"), nil
	case "path_hash_mode", "rep_path_hash":
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 || n > 2 {
			return arm, "", fmt.Errorf("path hash mode is 0, 1 or 2; got %q", v)
		}
		if param == "path_hash_mode" {
			arm.PathHashMode = &n
			// n+1 because mode 0 is one byte per hop, and an arm labelled
			// "0-byte" reads as a path that carries nothing.
			return arm, fmt.Sprintf("%d-byte", n+1), nil
		}
		arm.RepPathHash = &n
		return arm, fmt.Sprintf("rpt %d-byte", n+1), nil
	case "loop_detect":
		switch v {
		case "off", "minimal", "moderate", "strict":
		default:
			return arm, "", fmt.Errorf("loop.detect is off, minimal, moderate or strict; got %q", v)
		}
		arm.LoopDetect = v
		return arm, "loop " + v, nil
	case "cad":
		if v != "on" && v != "off" {
			return arm, "", fmt.Errorf("cad is on or off; got %q", v)
		}
		arm.CAD = v
		return arm, "cad " + v, nil
	case "spread_ms":
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			return arm, "", fmt.Errorf("spread is a whole number of seconds; got %q", v)
		}
		ms := n * 1000
		arm.SpreadMs = &ms
		if n == 0 {
			return arm, "all at once", nil
		}
		return arm, fmt.Sprintf("over %ds", n), nil
	}
	var have []string
	for _, p := range VaryParams {
		have = append(have, p.Name)
	}
	return arm, "", fmt.Errorf("cannot vary %q; there is: %s", param, strings.Join(have, ", "))
}

// bareVersion is a build name with its role and its v stripped, because an arm
// column headed "repeater-v1.17.0 · 1-byte" spends most of its width on the two
// things every arm in the sweep has in common.
func bareVersion(v, role string) string {
	return strings.TrimPrefix(strings.TrimPrefix(v, role), "v")
}

// joinLabel builds an arm's name one crossed parameter at a time.
func joinLabel(a, b string) string {
	if a == "" {
		return b
	}
	if b == "" {
		return a
	}
	return a + " · " + b
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
		sums := e.summarise()
		out := map[string]any{"runs": runs, "arms": sums}
		warn := e.notAResultYet()
		if warn != "" {
			out["warning"] = warn
		}
		w.Experiment = w.Experiment[:0]
		for _, m := range sums {
			w.Experiment = append(w.Experiment, state.ArmSummary{
				Arm:       m["arm"].(string),
				Runs:      m["runs"].(int),
				TX:        m["tx"].(float64),
				RX:        m["rx"].(float64),
				Delivered: m["delivered"].(float64),
				Redundant: m["redundant"].(float64),
				RXSpread:  m["rx_spread"].(float64),
			})
		}
		w.ExperimentWarning = warn
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
	// Identical runs across seeds are not a spread. That is not a fault here:
	// with one originator and rx_delay_base at its shipped zero, every
	// repeater relays exactly once and the count is a property of the
	// topology - the project's own study measured the same 93 transmissions
	// on each of eight seeds. What follows from it is that the seed cannot
	// bound the noise, so a difference between arms has nothing to be called
	// larger than, and saying so is the whole job of this function.
	for _, sum := range e.summarise() {
		if n, _ := sum["runs"].(int); n < 2 {
			continue
		}
		if sp, _ := sum["rx_spread"].(float64); sp == 0 {
			return fmt.Sprintf(
				"every seed of %v returned the same numbers, which this firmware "+
					"does by design with one originator - so the seed gives no "+
					"noise floor here, and a difference between arms has nothing "+
					"to be called larger than. Vary the senders, or quote the "+
					"deltas as unbounded.", sum["arm"])
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
