// The engine, behind the state layer.
//
// The old workbench built an engine and then read it from the frame loop. Here
// the store owns it: verbs ask it for things, the ticker advances it, and the
// renderer only ever sees a snapshot. That is the whole point of P0, and it is
// why the link margins below are computed once when the network changes rather
// than on every frame that draws them.
package main

import (
	"context"
	"math"
	"os"
	"path/filepath"
	"sync/atomic"

	"github.com/A13xB0/meshcoresim/internal/coverage"
	"github.com/A13xB0/meshcoresim/internal/engine"
	"github.com/A13xB0/meshcoresim/internal/gui/state"
	"github.com/A13xB0/meshcoresim/internal/linkbudget"
	"github.com/A13xB0/meshcoresim/internal/scenario"
	"github.com/A13xB0/meshcoresim/internal/terrain"
)

// sim holds the engine and the scenario it was built from.
type sim struct {
	eng     *engine.Engine
	nodes   []scenario.Node
	terr    coverage.Terrain
	warming atomic.Bool
}

// terrainStore is the elevation the engine sees.
//
// The same on-disk cache the rest of the tool fills, and offline: a path loss
// computed while a tile downloads is a path loss nobody asked for at a moment
// nobody chose. Missing tiles answer "no data", which the engine already
// handles - it is bare earth for that profile and says so.
func (s *sim) terrain() coverage.Terrain {
	if s.terr != nil {
		return s.terr
	}
	cache, err := os.UserCacheDir()
	if err != nil {
		s.terr = bareEarth{}
		return s.terr
	}
	st, err := terrain.NewTileStore(filepath.Join(cache, "meshcoresim", "terrain"))
	if err != nil {
		s.terr = bareEarth{}
		return s.terr
	}
	st.Zoom = terrain.DefaultZoom
	s.terr = st
	return s.terr
}

// bareEarth answers for nowhere, which is not the same as answering zero: the
// engine treats "no data" as a profile it cannot use rather than as sea level
// across the Atlantic.
type bareEarth struct{}

func (bareEarth) ElevationM(float64, float64) (float64, bool) { return 0, false }

// build makes an engine for a set of nodes.
func (s *sim) build(nodes []scenario.Node, freqMHz float64) {
	if s.eng != nil {
		_ = s.eng.Close()
	}
	s.nodes = nodes
	s.eng = engine.New(s.terrain(), engine.Config{
		FreqMHz: freqMHz, SF: 10, BandwidthHz: 250e3, CodingRate: 1,
		NoiseFigDB: 6, StepMs: 10, Seed: 9001,
	})
	for _, n := range nodes {
		s.eng.Add(n, nil)
	}
}

// links is every pair that can hear each other, with the weaker direction's
// margin, from the engine's own path loss.
//
// n squared, and on the 311 node fixture that is 48,000 path losses - which is
// why it is a verb that runs once on the store's goroutine and lands in the
// snapshot, rather than something the map does while drawing.
func (s *sim) links() []state.Link {
	if s.eng == nil {
		return nil
	}
	var out []state.Link
	for i := range s.nodes {
		for j := i + 1; j < len(s.nodes); j++ {
			loss, ok := s.eng.PathLossForTest(i, j)
			if !ok {
				continue
			}
			m := linkbudget.MarginDB(s.nodes[i], s.nodes[j], loss)
			// A link that does not close in the weaker direction by a wide
			// margin is not a link anybody wants drawn: below -20 dB the pair
			// is a different part of the country.
			if m < -20 {
				continue
			}
			out = append(out, state.Link{
				A: i, B: j, MarginDB: m, Known: true,
				AtoB: linkbudget.OneWayDB(s.nodes[i], s.nodes[j], loss),
				BtoA: linkbudget.OneWayDB(s.nodes[j], s.nodes[i], loss),
			})
		}
	}
	return out
}

// warm computes the link margins on a worker and hands them to the store.
//
// One at a time: a second warm while the first is running would compute the
// same thing twice and race to publish it.
func (s *sim) warm(st *state.Store, nodes int) {
	if s.eng == nil || s.warming.Swap(true) {
		return
	}
	go func() {
		defer s.warming.Store(false)
		ctx := context.Background()
		total := nodes * (nodes - 1) / 2
		_, _ = st.Do(ctx, "job.progress", state.Job{
			ID: "links", What: "measuring every link", Total: total})

		s.eng.WarmLinks(ctx, func(done, of int) {
			// Not every step: a progress update is a verb, and a verb per
			// path loss would make the queue the slow part.
			if done%2000 != 0 {
				return
			}
			_, _ = st.Do(ctx, "job.progress", state.Job{
				ID: "links", What: "measuring every link", Done: done, Total: of})
		})

		links := s.links()
		_, _ = st.Do(ctx, "links.set", links)
		_, _ = st.Do(ctx, "job.progress", state.Job{
			ID: "links", What: "measuring every link",
			Done: total, Total: total, Finished: true})
	}()
}

// trailsSince turns the engine's events into map trails.
//
// Only "tx" and "rx" - a "miss" is a reception that did not happen, and drawing
// it as traffic would put a line on the map for a packet that never arrived.
// A tx nobody received still gets a trail, with To of -1, because a repeater
// shouting into an empty valley is exactly the thing somebody is looking for.
func (s *sim) trailsSince(fromMs uint32, index map[string]int) []state.Trail {
	if s.eng == nil {
		return nil
	}
	var out []state.Trail
	for _, e := range s.eng.Events() {
		if e.AtMs < fromMs {
			continue
		}
		from, ok := index[e.From]
		if !ok {
			continue
		}
		switch e.Kind {
		case "tx":
			out = append(out, state.Trail{From: from, To: -1, AtMs: e.AtMs})
		case "rx":
			to, ok := index[e.To]
			if !ok {
				continue
			}
			out = append(out, state.Trail{
				From: from, To: to, AtMs: e.AtMs, Delivered: true})
		}
	}
	return out
}

// eventTail is the most recent n events, oldest first, and the total.
func (s *sim) eventTail(n int) ([]state.Event, int) {
	if s.eng == nil {
		return nil, 0
	}
	all := s.eng.Events()
	total := len(all)
	if len(all) > n {
		all = all[len(all)-n:]
	}
	out := make([]state.Event, 0, len(all))
	for _, e := range all {
		out = append(out, state.Event{
			AtMs: e.AtMs, Kind: e.Kind, From: e.From, To: e.To,
			MessageID: e.MessageID, PacketID: e.PacketID,
			SNRdB: e.SNRdB, Detail: e.Detail,
		})
	}
	return out, total
}

// scores is the engine's own scoreboard, projected.
func (s *sim) scores() []state.Score {
	if s.eng == nil {
		return nil
	}
	sb := s.eng.Scoreboard()
	out := make([]state.Score, 0, len(sb))
	for _, v := range sb {
		out = append(out, state.Score{
			Name: v.Name, Sent: v.Sent, Heard: v.Heard,
			AirtimeMs: v.AirtimeMs, DutyCyclePct: v.DutyCyclePct,
			UniqueDelivery: v.UniqueDelivery, RedundantRelay: v.RedundantRelay,
		})
	}
	return out
}

// budgetsFor breaks down the strongest link at a node, both ways.
//
// The strongest rather than a chosen one, because the question a budget panel
// answers when nothing has been picked is "how is this node doing at all",
// and its best link is the honest answer to that.
func (s *sim) budgetsFor(at int, links []state.Link) []state.Budget {
	if s.eng == nil || at < 0 || at >= len(s.nodes) {
		return nil
	}
	best, bestM := -1, math.Inf(-1)
	for _, l := range links {
		if !l.Known {
			continue
		}
		other := -1
		if l.A == at {
			other = l.B
		} else if l.B == at {
			other = l.A
		}
		if other < 0 || l.MarginDB <= bestM {
			continue
		}
		best, bestM = other, l.MarginDB
	}
	if best < 0 {
		return nil
	}
	loss, ok := s.eng.PathLossForTest(at, best)
	if !ok {
		return nil
	}
	a, b := s.nodes[at], s.nodes[best]
	return []state.Budget{
		{From: a.Name, To: b.Name, MarginDB: linkbudget.OneWayDB(a, b, loss),
			Terms: termsOf(linkbudget.Terms(a, b, loss))},
		{From: b.Name, To: a.Name, MarginDB: linkbudget.OneWayDB(b, a, loss),
			Terms: termsOf(linkbudget.Terms(b, a, loss))},
	}
}

func termsOf(in []linkbudget.Term) []state.BudgetTerm {
	out := make([]state.BudgetTerm, 0, len(in))
	for _, t := range in {
		out = append(out, state.BudgetTerm{Name: t.Name, DB: t.DB})
	}
	return out
}
