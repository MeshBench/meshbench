// Fault injection and recovery (plan §10/12).
//
// "Does the mesh survive losing that repeater, and for how long is it
// degraded?" - answered by generalising the schedule (schedule.add already
// sends from a node at a chosen instant) to also mutate one: a node-down
// fault stops a node's firmware exactly as node.stop does, on demand, but at
// a simulated instant chosen in advance rather than whenever an operator
// clicks. Reachability is measured the instant the fault fires and checked
// again every tick after, so recovery time is when it is actually observed
// to return, not assumed from the restore event alone - a fault the network
// never recovers from says so rather than reporting a number that looks like
// every other one.
package session

import (
	"context"
	"fmt"

	"github.com/MeshBench/meshbench/internal/gui/state"
)

func registerFault(st *state.Store, s *Sim) {
	// schedule.add_fault: a node-down or node-up at a chosen instant, logged
	// to the same schedule schedule.add already writes to - one list, one
	// table, a mutation is just an entry with no command and a Fault kind.
	st.Handle("schedule.add_fault", func(w *state.World, p any) (any, error) {
		node, _ := stringField(p, "node")
		if node == "" {
			return nil, fmt.Errorf("schedule.add_fault needs a node")
		}
		kind, _ := stringField(p, "kind")
		switch kind {
		case "node-down", "node-up", "arrive", "depart":
		default:
			return nil, fmt.Errorf(
				"this build injects node-down, node-up, arrive and depart; %q needs a fault kind not yet built", kind)
		}
		if _, ok := findNode(w.Nodes, node); !ok {
			return nil, fmt.Errorf("no node named %q", node)
		}
		snd := state.Send{Node: node, Fault: kind}
		if v, ok := numField(p, "at_ms"); ok {
			snd.AtMs = uint32(v)
		}
		w.Sends = append(w.Sends, snd)
		w.Say(fmt.Sprintf("%s: %s scheduled at %.1f s", node, kind, float64(snd.AtMs)/1000))
		return map[string]any{"sends": len(w.Sends)}, nil
	})
}

// stepFaults fires any due schedule entries that mutate the scenario, and
// advances every recovery check still open. Called once per engine tick,
// after the events and links a fault's own before/after counts read from
// are current for this instant.
func (s *Sim) stepFaults(ctx context.Context, w *state.World) {
	if s.firedFaults == nil {
		s.firedFaults = map[int]bool{}
	}
	if s.downNodes == nil {
		s.downNodes = map[string]bool{}
	}
	for i := range w.Sends {
		snd := w.Sends[i]
		if snd.Fault == "" || snd.Fault == "move" || s.firedFaults[i] || snd.AtMs > w.NowMs {
			continue // "move" is stepMoves's own entry to fire, not this one's
		}
		s.firedFaults[i] = true
		s.fireFault(ctx, w, snd)
	}
	s.checkRecoveries(w)
}

func (s *Sim) fireFault(ctx context.Context, w *state.World, snd state.Send) {
	idx, ok := nodeIndex(w.Nodes, snd.Node)
	if !ok {
		return
	}
	down := make(map[int]bool, len(s.downNodes))
	for i, n := range w.Nodes {
		if s.downNodes[n.Name] {
			down[i] = true
		}
	}
	outBefore, inBefore := reachCounts(w.Links, idx, down)

	// arrive/depart are node-up/node-down under a name that reads right for
	// a node joining or leaving rather than one merely failing - same
	// mechanism, since this simulator has no notion of a node that was never
	// in the scenario at all: "arrives" means its firmware was never started
	// at play, and this is the schedule entry that starts it.
	var err error
	switch snd.Fault {
	case "node-down", "depart":
		err = s.stopNode(snd.Node)
		if err == nil {
			s.downNodes[snd.Node] = true
		}
	case "node-up", "arrive":
		err = s.startNode(ctx, snd.Node, w.Seed)
		if err == nil {
			delete(s.downNodes, snd.Node)
		}
	}
	if err != nil {
		w.Say(fmt.Sprintf("fault: %s %s failed: %s", snd.Node, snd.Fault, err))
		return
	}

	goesDown := snd.Fault == "node-down" || snd.Fault == "depart"
	down2 := make(map[int]bool, len(down))
	for k, v := range down {
		down2[k] = v
	}
	if goesDown {
		down2[idx] = true
	} else {
		delete(down2, idx)
	}
	outAfter, inAfter := reachCounts(w.Links, idx, down2)

	ev := state.FaultEvent{
		Kind: snd.Fault, Node: snd.Node, AtMs: w.NowMs,
		Total:     len(w.Nodes) - 1,
		OutBefore: outBefore, InBefore: inBefore,
		OutAfter: outAfter, InAfter: inAfter,
	}
	w.Say(fmt.Sprintf("%s: %s at %.1f s - reach %d/%d -> %d/%d",
		snd.Node, snd.Fault, float64(w.NowMs)/1000, outBefore, inBefore, outAfter, inAfter))
	w.FaultLog = append(w.FaultLog, ev)
	if goesDown {
		s.watching = append(s.watching, len(w.FaultLog)-1)
	}
}

// checkRecoveries re-measures reachability for every open node-down fault
// and closes the ones that have returned to their pre-fault value.
func (s *Sim) checkRecoveries(w *state.World) {
	if len(s.watching) == 0 {
		return
	}
	still := s.watching[:0]
	for _, li := range s.watching {
		if li >= len(w.FaultLog) {
			continue
		}
		ev := &w.FaultLog[li]
		idx, ok := nodeIndex(w.Nodes, ev.Node)
		if !ok {
			continue
		}
		down := make(map[int]bool, len(s.downNodes))
		for i, n := range w.Nodes {
			if s.downNodes[n.Name] {
				down[i] = true
			}
		}
		out, in := reachCounts(w.Links, idx, down)
		if out >= ev.OutBefore && in >= ev.InBefore {
			ev.Recovered = true
			ev.RecoveredAtMs = w.NowMs
			ev.UndeliveredCost = undeliveredCost(w.Events, ev.AtMs, ev.RecoveredAtMs)
			continue
		}
		still = append(still, li)
	}
	s.watching = still
}

// undeliveredCost is how many distinct messages transmitted in [fromMs,
// toMs] never appear as received anywhere in the log available - "the cost
// of the outage" the plan asks for, read from the ledger rather than
// inferred.
func undeliveredCost(events []state.Event, fromMs, toMs uint32) int {
	delivered := map[uint64]bool{}
	for _, e := range events {
		if e.Kind == "rx" {
			delivered[e.MessageID] = true
		}
	}
	attempted := map[uint64]bool{}
	for _, e := range events {
		if e.Kind == "tx" && e.AtMs >= fromMs && e.AtMs <= toMs {
			attempted[e.MessageID] = true
		}
	}
	cost := 0
	for id := range attempted {
		if !delivered[id] {
			cost++
		}
	}
	return cost
}

func nodeIndex(nodes []state.Node, name string) (int, bool) {
	for i, n := range nodes {
		if n.Name == name {
			return i, true
		}
	}
	return 0, false
}
