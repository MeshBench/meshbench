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
package experiment

import (
	"context"
	"fmt"
	"sync"

	"github.com/MeshBench/meshbench/internal/app/session"
	"github.com/MeshBench/meshbench/internal/app/state"
)

// Result is one arm at one seed.
type Result struct {
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
	Arms     []session.ExpArm `json:"arms"`
	Seeds    []uint64         `json:"seeds"`
	Senders  []string         `json:"senders"`
	RunForMs uint32           `json:"run_for_ms"`
	SendAtMs uint32           `json:"send_at_ms"`

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
	results []Result
	log     []string
	status  string

	// notReproducible is why this matrix's cells cannot be compared with each
	// other, or "" when they can.
	//
	// Taken from the network the run was started on and kept, rather than read
	// again when the results are: an arm is a comparison between cells that
	// each ran once, and what decides whether that comparison means anything is
	// what the mesh was made of at the time. A node swapped for an emulated one
	// after the run would otherwise condemn results it had no part in, and one
	// swapped the other way would quietly clear a warning that was earned.
	notReproducible string

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

// newExperiment is the matrix a session starts with: one unnamed arm and four
// seeds, which is the smallest thing that runs and reports.
//
// It used to be a field on Sim, filled in on first use. It is owned by this
// package now and handed to the verbs at registration, which is what lets the
// matrix live outside the session at all - and it removes the lazy nil check
// that every one of those verbs had to remember to go through.
func newExperiment() *experiment {
	return &experiment{
		Arms:     []session.ExpArm{{Label: "baseline"}},
		Seeds:    []uint64{1, 2, 3, 4},
		RunForMs: 90_000, SendAtMs: 30_000,
	}
}

// publish puts what is defined into the world, so every panel and every client
// sees one answer. The panel used to keep its own copy and disagreed with the
// session the moment anything else defined an arm.
func (e *experiment) publish(w *state.World) {
	w.ExperimentArms = e.armLabels()
	w.ExperimentSenders = append([]string(nil), e.Senders...)
	w.ExperimentRuns = e.runRows()
}

func registerExperiment(st *state.Store, s *session.Sim, e *experiment) {
	st.Handle("experiment.define", func(w *state.World, p any) (any, error) {
		defer func() { e.publish(w) }()
		if m, ok := p.(map[string]any); ok {
			if xs, ok := m["arms"].([]any); ok && len(xs) > 0 {
				e.Arms = e.Arms[:0]
				for _, x := range xs {
					am, _ := x.(map[string]any)
					arm := session.ExpArm{}
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
		if v, ok := session.NamedNum(p, "run_for_ms"); ok && v > 0 {
			e.RunForMs = uint32(v)
		}
		if v, ok := session.NamedNum(p, "send_at_ms"); ok && v > 0 {
			e.SendAtMs = uint32(v)
		}
		if v, ok := session.NamedNum(p, "spread_ms"); ok && v >= 0 {
			e.SpreadMs = uint32(v)
		}
		if v, ok := session.NamedNum(p, "bytes"); ok && v >= 0 {
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
		defer func() { e.publish(w) }()
		param, _ := session.StringField(p, "parameter")
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
		if len(base) == 0 || (len(base) == 1 && !base[0].Names()) {
			base = []session.ExpArm{{}}
		}
		var out []session.ExpArm
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
		defer func() { e.publish(w) }()
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
		defer func() { e.publish(w) }()
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
		// Deliberately narrow. In the old workbench a base repeater_version
		// overrode a per-node pin and left the room server looking for a role
		// its build does not publish, so one node of fifty-six failed to start
		// and the arm looked like a firmware regression.
		if v, ok := session.StringField(p, "run_for_ms"); ok {
			_ = v
		}
		if v, ok := session.NamedNum(p, "run_for_ms"); ok && v > 0 {
			e.RunForMs = uint32(v)
		}
		if v, ok := session.NamedNum(p, "send_at_ms"); ok && v > 0 {
			e.SendAtMs = uint32(v)
		}
		return e.describe(), nil
	})
}
