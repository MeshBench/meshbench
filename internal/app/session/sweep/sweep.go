// Sweeping a parameter over seeds, into the matrix that draws it.
//
// Every cell is its own engine on its own seed. Reusing one engine across
// cells would carry a warmed link cache and an event log from the previous
// cell into the next, and the first thing anybody would ask of a sweep is
// whether the cells were independent.
package sweep

import (
	"context"
	"math"

	"github.com/MeshBench/meshbench/internal/app/session"
	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/sim/engine"
	"github.com/MeshBench/meshbench/internal/world/scenario"
)

// SweepPlan is what to run.
type SweepPlan struct {
	Arms    []session.SweepArm
	Seeds   []uint64
	UntilMs uint32
	Metric  string
	Node    string
	Terrain bool
}

// DefaultSweep is small on purpose: a sweep somebody starts by accident should
// finish while they are still looking at it.
func DefaultSweep(node string) SweepPlan {
	return SweepPlan{
		Arms: []session.SweepArm{
			{Name: "every 2s", EveryMs: 2000},
			{Name: "every 1s", EveryMs: 1000},
			{Name: "every 500ms", EveryMs: 500},
			{Name: "every 250ms", EveryMs: 250},
		},
		Seeds:   []uint64{1, 2, 3, 4, 5, 6},
		UntilMs: 20000,
		Metric:  "transmissions",
		Node:    node,
	}
}

// runSweep executes the plan, reporting progress, and returns the matrix.
func runSweep(s *session.Sim, ctx context.Context, p SweepPlan,
	progress func(done, total int)) *state.Matrix {

	m := &state.Matrix{
		Metric: p.Metric,
		Arms:   make([]string, 0, len(p.Arms)),
		Seeds:  p.Seeds,
		Values: make([]float64, len(p.Arms)*len(p.Seeds)),
	}
	for _, a := range p.Arms {
		m.Arms = append(m.Arms, a.Name)
	}
	total := len(p.Arms) * len(p.Seeds)
	done := 0
	for r, arm := range p.Arms {
		for c, seed := range p.Seeds {
			select {
			case <-ctx.Done():
				// Cells not reached stay NaN, which the matrix draws as
				// absence rather than as zero - a cancelled sweep must not
				// look like a sweep that measured nothing.
				for i := done; i < total; i++ {
					m.Values[i] = math.NaN()
				}
				return m
			default:
			}
			m.Values[r*len(p.Seeds)+c] = runCell(s, ctx, p, arm, seed)
			done++
			if progress != nil {
				progress(done, total)
			}
		}
	}
	return m
}

// runCell is one arm at one seed, on its own engine.
func runCell(s *session.Sim, ctx context.Context, p SweepPlan, arm session.SweepArm, seed uint64) float64 {
	eng := engine.New(s.Terrain(), engine.Config{
		FreqMHz: 869.618, SF: 10, BandwidthHz: 250e3, CodingRate: 1,
		NoiseFigDB: 6, StepMs: 10, Seed: seed,
	})
	defer func() { _ = eng.Close() }()
	at := 0
	for i, n := range s.Nodes() {
		// The arm's firmware, pinned per node before the engine sees it. Set
		// on a copy: the scenario is shared by every cell, and an arm that
		// edited it would leave its version behind for the next one - which is
		// the silent failure the firmware-A/B tool was written to prevent.
		n = session.WithFirmware(n, arm)
		eng.Add(n, nil)
		if n.Name == p.Node {
			at = i
		}
	}
	next := uint32(0)
	for eng.NowMs() < p.UntilMs {
		if ctx.Err() != nil {
			break
		}
		if eng.NowMs() >= next {
			eng.Inject(at, []byte("msim-sweep"))
			next = eng.NowMs() + arm.EveryMs
		}
		if err := eng.Step(ctx); err != nil {
			break
		}
	}
	return metricOf(eng, p.Metric)
}

func metricOf(eng *engine.Engine, metric string) float64 {
	sent, heard, delivered, redundant := 0, 0, 0, 0
	for _, v := range eng.Scoreboard() {
		sent += v.Sent
		heard += v.Heard
		delivered += v.UniqueDelivery
		redundant += v.RedundantRelay
	}
	switch metric {
	case "receptions":
		return float64(heard)
	case "delivered":
		return float64(delivered)
	case "redundant":
		return float64(redundant)
	}
	return float64(sent)
}

// FirstCompanion is the sweep's originator when nothing is selected.
func FirstCompanion(nodes []scenario.Node) string {
	for _, n := range nodes {
		if string(n.Kind) == "companion" {
			return n.Name
		}
	}
	if len(nodes) > 0 {
		return nodes[0].Name
	}
	return ""
}
