// Turning the engine's own record into what the panels read: trails, the
// event tail and its counts, the scoreboard, and the budget for a selection.
//
// All read-only, all shaped for a snapshot. The tail matters: asking for the
// whole log per tick copies the whole log, which is quadratic over a run.
package session

import (
	"math"

	"github.com/MeshBench/meshbench/internal/app/state"
	"github.com/MeshBench/meshbench/internal/sim/engine"
	"github.com/MeshBench/meshbench/internal/study/linkbudget"
)

// trailsSince turns the engine's events into map trails.
//
// Only "tx" and "rx" - a "miss" is a reception that did not happen, and drawing
// it as traffic would put a line on the map for a packet that never arrived.
// A tx nobody received still gets a trail, with To of -1, because a repeater
// shouting into an empty valley is exactly the thing somebody is looking for.
func (s *Sim) trailsSince(fromMs uint32, index map[string]int) []state.Trail {
	if s.liveEngine() == nil {
		return nil
	}
	var out []state.Trail
	for _, e := range s.liveEngine().EventsSince(fromMs) {
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
func (s *Sim) eventTail(n int) ([]state.Event, int) {
	if s.liveEngine() == nil {
		return nil, 0
	}
	all, total := s.liveEngine().EventsTail(n)
	out := make([]state.Event, 0, len(all))
	for _, e := range all {
		out = append(out, state.Event{
			AtMs: e.AtMs, Kind: e.Kind, From: e.From, To: e.To,
			MessageID: e.MessageID, PacketID: e.PacketID,
			SNRdB: e.SNRdB, Detail: e.Detail,
			Class: string(engine.EventClass(e)),
		})
	}
	return out, total
}

// eventCounts is the whole run's events by class, in the snapshot's shape.
func (s *Sim) eventCounts() state.EventCounts {
	if s.liveEngine() == nil {
		return state.EventCounts{}
	}
	c := s.liveEngine().EventCounts()
	return state.EventCounts{
		Sent: c[engine.ClassSent], Received: c[engine.ClassReceived],
		HalfDuplex:   c[engine.ClassHalfDuplex],
		Interference: c[engine.ClassInterference],
		Collision:    c[engine.ClassCollision],
		ReceiverBusy: c[engine.ClassReceiverBusy],
		Floor:        c[engine.ClassFloor],
		Unclassified: c[engine.ClassUnclassified],
	}
}

// scores is the engine's own scoreboard, projected.
func (s *Sim) scores() []state.Score {
	if s.liveEngine() == nil {
		return nil
	}
	sb := s.liveEngine().Scoreboard()
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
func (s *Sim) budgetsFor(at int, links []state.Link) []state.Budget {
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
