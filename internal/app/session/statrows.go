// One node's cost and traffic, in a shape the socket can carry.
//
// state.NodeStat is what the panels read and carries things a wire cannot -
// a screen's raw bits, the chip's register set - so this is the subset a
// caller outside the window can act on, and nothing else. A client that wants
// the picture asks board.screen; a client that wants the registers asks
// node.radio.
package session

import (
	"strconv"

	"github.com/MeshBench/meshbench/internal/app/state"
)

func statRows(stats []state.NodeStat) []map[string]any {
	out := make([]map[string]any, 0, len(stats))
	for _, n := range stats {
		// State rather than only Running, because a boolean cannot say
		// "changing firmware" and a row that goes blank while it happens
		// looks like a node that has died.
		st := n.State
		if st == "" {
			st = "stopped"
			if n.Running {
				st = "running"
			}
		}
		out = append(out, map[string]any{
			"name": n.Name, "backend": n.Backend, "firmware": n.Firmware,
			"running": n.Running, "state": st, "board": n.Board,
			"pid": n.PID,
			// Absent rather than zero where nothing measured them. A caller
			// reading a number believes it, and 0 CPU for every node on a Mac
			// is not a light mesh, it is a sampler that reads /proc on a
			// platform that has none.
			"rss_bytes":     costOrNil(n.CostMeasured, float64(n.RSSBytes)),
			"cpu_ms":        costOrNil(n.CostMeasured, float64(n.CPUms)),
			"cpu_pct":       costOrNil(n.CostMeasured, n.CPUPct),
			"cost_measured": n.CostMeasured,
			"sent":          n.Sent, "heard": n.Heard,
			"last_sent_ms": n.LastSentMs, "last_heard_ms": n.LastHeardMs,
			"last_sent_to": n.LastSentTo, "last_heard_from": n.LastHeardFrom,
			// The chip's own counters: the only way to tell a busy mesh from
			// a radio that cries busy too readily.
			"irq_reads": n.IRQReads, "busy_reads": n.BusyReads,
			"busy_ms": n.BusyMs, "spurious": n.Spurious,
			// Absent from a chip that ignores IrqMask, for the same reason
			// the costs above are: it did not count, so there is no zero.
			"irq_suppressed": suppressedOrNil(n),
		})
		// The board's lamps, by pin, where something watches them. Absent
		// rather than empty where nothing does, because "every lamp off" and
		// "not watched" are different answers.
		if n.Lamps != nil {
			lamps := make(map[string]any, len(n.Lamps))
			for pin, l := range n.Lamps {
				lamps[strconv.Itoa(pin)] = map[string]any{"lit": l.Lit, "changes": l.Changes}
			}
			out[len(out)-1]["lamps"] = lamps
		}
	}
	return out
}

// withAirtime adds each node's time on the air, as the Scores panel shows it.
//
// The engine has always counted it, and the panel has always drawn it, but a
// script could only get the whole mesh's total out of an experiment cell - so
// "which repeater is over its duty cycle" had an answer on screen and none
// outside it. airtime_ms is cumulative over the run, so a caller wanting a
// window subtracts two readings; duty_cycle_pct is over the whole run, boot
// included, exactly as the panel's column is. A node the engine has no score
// for has neither, rather than a zero.
func withAirtime(rows []map[string]any, scores []state.Score) []map[string]any {
	by := make(map[string]state.Score, len(scores))
	for _, sc := range scores {
		by[sc.Name] = sc
	}
	for _, r := range rows {
		name, _ := r["name"].(string)
		if sc, ok := by[name]; ok {
			r["airtime_ms"], r["duty_cycle_pct"] = sc.AirtimeMs, sc.DutyCyclePct
		}
	}
	return rows
}

// suppressedOrNil is the chip's masked-event count, or nothing from a chip
// that does not keep one.
func suppressedOrNil(n state.NodeStat) any {
	if !n.SuppressedReported {
		return nil
	}
	return n.IRQSuppressed
}

// costOrNil is a reading, or nothing where there was no reading to take.
func costOrNil(measured bool, v float64) any {
	if !measured {
		return nil
	}
	return v
}
