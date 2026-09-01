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
	"fmt"
	"sync"

	"github.com/MeshBench/meshbench/internal/app/state"
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

	// done is closed by the run goroutine as it leaves, and is the only honest
	// answer to "has the last experiment finished".
	//
	// running cannot answer it: stop clears running the instant it is asked, and
	// the goroutine goes on for as long as the cell it is in takes to notice its
	// context. A start in that window used to clear the results out from under a
	// worker still appending to them, so the new run's table carried the tail of
	// the old one's cells and looked like a real measurement.
	done chan struct{}
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

// matrixKeys is what every definition verb answers with: counts of what is now
// defined, and the arm labels, which are the only part a caller cannot infer.
var matrixKeys = []string{"arms", "seeds", "senders", "runs", "run_for_ms",
	"send_at_ms", "spread_ms", "bytes", "scope", "arm_labels"}

func registerExperiment(st *state.Store, s *Sim) {
	st.HandleSpec("experiment.define", state.Spec{
		What: "state a whole matrix in one call - the arms, the seeds, the " +
			"senders and the burst's timing - which is how a script sets up a " +
			"sweep it did not build in the panel",
		Params: []state.Param{
			{Name: "arms", Type: state.ParamArray,
				What: "one object per arm, carrying `label`, `repeater_version` " +
					"and `companion_version`; an arm with no label takes its " +
					"repeater version as one, and an absent or empty list " +
					"leaves the arms alone"},
			{Name: "seeds", Type: state.ParamArray,
				What: "the seeds each arm is repeated over, as numbers; anything " +
					"that is not a number is dropped, and an absent or empty " +
					"list leaves the seeds alone"},
			{Name: "senders", Type: state.ParamArray,
				What: "the nodes that originate the burst, by name; unlike the " +
					"others an empty list is obeyed and clears them, which " +
					"leaves an experiment experiment.start will refuse"},
			{Name: "run_for_ms", Type: state.ParamNumber,
				What: "how long each cell runs, in simulated milliseconds; zero " +
					"or less is ignored and the current length kept"},
			{Name: "send_at_ms", Type: state.ParamNumber,
				What: "the simulated instant the burst is fired, which is the " +
					"same in every arm; zero or less is ignored"},
			{Name: "spread_ms", Type: state.ParamNumber,
				What: "milliseconds to stagger the senders over; zero fires them " +
					"all at once, which is the sharpest test of contention and " +
					"the least like anything real, and a negative value is ignored"},
			{Name: "bytes", Type: state.ParamNumber,
				What: "pad the message to this size, since airtime scales with " +
					"payload and airtime is what collides; zero sends the label " +
					"alone, and a negative value is ignored"},
			{Name: "scope", Type: state.ParamString,
				What: "the region every sender originates under; empty sends " +
					"unscoped, which is carried by a different set of repeaters " +
					"and so measures a different network"},
		},
		Returns: matrixKeys,
		Answers: "Counts of what is now defined rather than the definition " +
			"itself, except `arm_labels`, which names every arm: a count cannot " +
			"tell a cross that produced the six arms wanted from one that " +
			"produced six others.",
		Example: &state.Example{
			Params: map[string]any{
				"senders": []any{"West Lomond"}, "seeds": []any{1.0, 2.0},
				"run_for_ms": 90000.0, "send_at_ms": 30000.0,
			},
			What:     "a flood from one node, ninety seconds a cell, over two seeds",
			Runnable: true,
		},
	}, func(w *state.World, p any) (any, error) {
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
	st.HandleSpec("experiment.vary", state.Spec{
		What: "cross the arms already defined with one parameter's values, so " +
			"three path hash modes against two firmware versions is six arms " +
			"rather than the two the second call would leave",
		Params: []state.Param{
			{Name: "parameter", Type: state.ParamString, Required: true, Primary: true,
				What: "what to vary: path_hash_mode, rep_path_hash, loop_detect, " +
					"cad, repeater_version, companion_version, spread_ms, or " +
					"`set:` followed by any firmware setting the CLI takes; " +
					"anything else is refused, with the list"},
			{Name: "values", Type: state.ParamArray, Required: true,
				What: "the values, as strings, one arm per value; a list holding " +
					"no strings is refused, and a value the parameter cannot " +
					"take is refused with what it can"},
		},
		Returns: matrixKeys,
		Answers: "It crosses onto the arms that are there rather than replacing " +
			"them, so calling it three times gives the full product. It also " +
			"discards the last sweep's results, because a finished sweep's arms " +
			"answered a different question from the one now being asked.",
		Example: &state.Example{
			Params:   map[string]any{"parameter": "cad", "values": []any{"off", "on"}},
			What:     "one arm that listens before talking and one that does not",
			Runnable: true,
		},
	}, func(w *state.World, p any) (any, error) {
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

	st.HandleSpec("experiment.seeds", state.Spec{
		What: "replace the seeds every arm is repeated over, which is the only " +
			"thing that gives a difference between arms something to be called " +
			"larger than",
		Params: []state.Param{
			{Name: "seeds", Type: state.ParamArray, Required: true,
				What: "the seeds, as numbers; anything that is not a number is " +
					"dropped, and a list left holding none is refused rather " +
					"than emptying the seeds"},
		},
		Returns: matrixKeys,
		Example: &state.Example{
			Params:   map[string]any{"seeds": []any{1.0, 2.0, 3.0, 4.0}},
			What:     "four draws of each arm",
			Runnable: true,
		},
	}, func(w *state.World, p any) (any, error) {
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

	st.HandleSpec("experiment.senders", state.Spec{
		What: "choose which nodes originate the burst, which decides more than " +
			"it looks like: with one originator every seed can return the same " +
			"numbers, and then the seed bounds nothing",
		Params: []state.Param{
			{Name: "senders", Type: state.ParamArray,
				What: "the node names; an absent list leaves the senders alone, " +
					"an empty one clears them, and entries that are not strings " +
					"are dropped"},
		},
		Returns: matrixKeys,
		Example: &state.Example{
			Params:   map[string]any{"senders": []any{"West Lomond", "Dunfermline"}},
			What:     "two originators, so the seeds have something to disagree about",
			Runnable: true,
		},
	}, func(w *state.World, p any) (any, error) {
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

	st.HandleSpec("experiment.base", state.Spec{
		What: "set the two timings every arm shares, how long a cell runs and " +
			"when its burst is fired, and deliberately nothing else: the " +
			"firmware versions belong to the arms",
		Params: []state.Param{
			{Name: "run_for_ms", Type: state.ParamNumber,
				What: "how long each cell runs, in simulated milliseconds; zero " +
					"or less is ignored and the current length kept"},
			{Name: "send_at_ms", Type: state.ParamNumber,
				What: "the simulated instant the burst is fired, the same in " +
					"every arm; zero or less is ignored"},
		},
		Returns: matrixKeys,
		Answers: "The same summary experiment.define answers with, so the " +
			"arms and senders it reports are whatever they already were.",
		Example: &state.Example{
			Params:   map[string]any{"run_for_ms": 120000.0, "send_at_ms": 30000.0},
			What:     "a two minute cell with the burst thirty seconds in",
			Runnable: true,
		},
	}, func(w *state.World, p any) (any, error) {
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
}
