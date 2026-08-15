// Movement, arrival and departure - plan §13, extending plan §12's schedule
// from a fault firing once to a change that plays out over the run.
//
// A move is not a jump: the node's position is interpolated every tick
// between where it was when the move fired and where it is going, over the
// duration given, and each tick's new position goes through
// engine.SetPosition - the narrow invalidation, not engine.InvalidateLinks's
// whole-cache drop, because a run with one moving node recomputing every
// pair on every tick is a run nobody waits for.
package session

import (
	"context"
	"fmt"

	"github.com/MeshBench/meshbench/internal/gui/state"
	"github.com/MeshBench/meshbench/internal/linkbudget"
)

// activeMove is one node's track in flight.
type activeMove struct {
	node                string
	fromLat, fromLon    float64
	toLat, toLon        float64
	startMs, durationMs uint32
}

// connSample is one measurement of how many other nodes a tracked node can
// currently flood-reach, taken cheaply (this node's own pairs only, not the
// whole link matrix) so tracking a moving node does not itself become the
// performance problem this file exists to avoid.
type connSample struct {
	atMs uint32
	n    int
}

func registerMovement(st *state.Store, s *Sim) {
	// schedule.add_move: interpolate a node's position over the run.
	st.Handle("schedule.add_move", func(w *state.World, p any) (any, error) {
		node, _ := stringField(p, "node")
		if node == "" {
			return nil, fmt.Errorf("schedule.add_move needs a node")
		}
		if _, ok := findNode(w.Nodes, node); !ok {
			return nil, fmt.Errorf("no node named %q", node)
		}
		toLat, latOK := numField(p, "to_lat")
		toLon, lonOK := numField(p, "to_lon")
		if !latOK || !lonOK {
			return nil, fmt.Errorf("schedule.add_move needs to_lat and to_lon")
		}
		snd := state.Send{Node: node, Fault: "move", ToLat: toLat, ToLon: toLon}
		if v, ok := numField(p, "at_ms"); ok {
			snd.AtMs = uint32(v)
		}
		if v, ok := numField(p, "duration_ms"); ok {
			snd.DurationMs = uint32(v)
		}
		w.Sends = append(w.Sends, snd)
		w.Say(fmt.Sprintf("%s: move to %.4f,%.4f starting at %.1f s over %.1f s",
			node, toLat, toLon, float64(snd.AtMs)/1000, float64(snd.DurationMs)/1000))
		return map[string]any{"sends": len(w.Sends)}, nil
	})

	// node.track / node.untrack: which nodes' connectivity history is worth
	// paying to keep. Off by default - a network nobody asked about does not
	// need a sample taken of it every second for the length of the run.
	st.Handle("node.track", func(w *state.World, p any) (any, error) {
		name := soleString(p)
		if _, ok := findNode(w.Nodes, name); !ok {
			return nil, fmt.Errorf("no node named %q", name)
		}
		if s.trackedNodes == nil {
			s.trackedNodes = map[string]bool{}
		}
		s.trackedNodes[name] = true
		w.Say("tracking " + name + "'s neighbour count over the run")
		return map[string]any{"tracking": name}, nil
	})

	st.Handle("node.connectivity", func(w *state.World, p any) (any, error) {
		name := soleString(p)
		hist := s.neighbourHistory[name]
		if len(hist) == 0 {
			return nil, fmt.Errorf("no connectivity history for %q - node.track it first, then run", name)
		}
		longestMs, longestAtMs, minN := longestGap(hist)
		samples := make([]map[string]any, 0, len(hist))
		for _, sm := range hist {
			samples = append(samples, map[string]any{"at_ms": sm.atMs, "neighbours": sm.n})
		}
		return map[string]any{
			"node": name, "samples": samples, "min_neighbours": minN,
			"longest_gap_ms": longestMs, "longest_gap_at_ms": longestAtMs,
		}, nil
	})
}

// longestGap is plan §13's own headline number: how long the longest
// stretch with zero neighbours lasted, and when it started - a duration,
// never averaged into a mean, because a four-minute gap and eight two-second
// blips average to the same number and are not the same finding.
func longestGap(hist []connSample) (longestMs, atMs uint32, minN int) {
	minN = hist[0].n
	gapStartMs := uint32(0)
	inGap := false
	for _, sm := range hist {
		if sm.n < minN {
			minN = sm.n
		}
		if sm.n == 0 {
			if !inGap {
				inGap, gapStartMs = true, sm.atMs
			}
			continue
		}
		if inGap {
			if d := sm.atMs - gapStartMs; d > longestMs {
				longestMs, atMs = d, gapStartMs
			}
			inGap = false
		}
	}
	if inGap {
		if last := hist[len(hist)-1].atMs; last-gapStartMs > longestMs {
			longestMs, atMs = last-gapStartMs, gapStartMs
		}
	}
	return longestMs, atMs, minN
}

// stepMoves advances every move in flight and takes a connectivity sample
// for every tracked node - called from the same tick stepFaults is, so a
// scheduled change of any kind (fault or move) fires from one place.
func (s *Sim) stepMoves(ctx context.Context, w *state.World) {
	if s.firedFaults == nil {
		s.firedFaults = map[int]bool{}
	}
	for i := range w.Sends {
		snd := w.Sends[i]
		if snd.Fault != "move" || s.firedFaults[i] || snd.AtMs > w.NowMs {
			continue
		}
		s.firedFaults[i] = true
		idx, ok := nodeIndex(w.Nodes, snd.Node)
		if !ok {
			continue
		}
		s.moves = append(s.moves, activeMove{
			node: snd.Node, fromLat: w.Nodes[idx].Lat, fromLon: w.Nodes[idx].Lon,
			toLat: snd.ToLat, toLon: snd.ToLon,
			startMs: snd.AtMs, durationMs: snd.DurationMs,
		})
	}

	still := s.moves[:0]
	for _, mv := range s.moves {
		lat, lon, done := interpolate(mv, w.NowMs)
		if idx, ok := nodeIndex(w.Nodes, mv.node); ok {
			w.Nodes[idx].Lat, w.Nodes[idx].Lon = lat, lon
		}
		if s.eng != nil {
			s.eng.SetPosition(mv.node, lat, lon)
		}
		if !done {
			still = append(still, mv)
		}
	}
	s.moves = still

	s.sampleConnectivity(w)
}

// interpolate is a pure function of the track and the clock - same seed,
// same simulated time, same position, with nothing in it that could depend
// on wall time or goroutine order. done is true once the track has reached
// toLat/toLon; a caller keeps calling until it does, and every call after is
// the endpoint, never an overshoot.
func interpolate(mv activeMove, nowMs uint32) (lat, lon float64, done bool) {
	if mv.durationMs == 0 || nowMs-mv.startMs >= mv.durationMs {
		return mv.toLat, mv.toLon, true
	}
	frac := float64(nowMs-mv.startMs) / float64(mv.durationMs)
	return mv.fromLat + (mv.toLat-mv.fromLat)*frac, mv.fromLon + (mv.toLon-mv.fromLon)*frac, false
}

// connSampleIntervalMs is how often a tracked node's neighbour count is
// measured - a second of simulated time, not every tick: fine enough to
// place a gap to within a second, cheap enough that tracking several nodes
// through a long run does not itself cost what this file exists to save.
const connSampleIntervalMs = 1000

func (s *Sim) sampleConnectivity(w *state.World) {
	if len(s.trackedNodes) == 0 || s.eng == nil {
		return
	}
	if s.sampledOnce && w.NowMs-s.lastSampleAt < connSampleIntervalMs {
		return
	}
	s.sampledOnce, s.lastSampleAt = true, w.NowMs
	if s.neighbourHistory == nil {
		s.neighbourHistory = map[string][]connSample{}
	}
	for name := range s.trackedNodes {
		idx, ok := nodeIndex(w.Nodes, name)
		if !ok || idx >= len(s.nodes) {
			continue
		}
		n := 0
		for j := range s.nodes {
			if j == idx {
				continue
			}
			if loss, ok := s.eng.PathLossForTest(idx, j); ok {
				if linkbudget.MarginDB(s.nodes[idx], s.nodes[j], loss) > 0 {
					n++
				}
			}
		}
		s.neighbourHistory[name] = append(s.neighbourHistory[name], connSample{atMs: w.NowMs, n: n})
	}

	w.Connectivity = w.Connectivity[:0]
	for name, hist := range s.neighbourHistory {
		longestMs, atMs, minN := longestGap(hist)
		w.Connectivity = append(w.Connectivity, state.ConnectivitySummary{
			Node: name, Samples: len(hist), MinNeighbours: minN,
			LongestGapMs: longestMs, LongestGapAtMs: atMs,
		})
	}
}
