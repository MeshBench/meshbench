// One node's cost and traffic, in a shape the socket can carry.
//
// state.NodeStat is what the panels read and carries things a wire cannot -
// a screen's raw bits, the chip's register set - so this is the subset a
// caller outside the window can act on, and nothing else. A client that wants
// the picture asks board.screen; a client that wants the registers asks
// node.radio.
package session

import "github.com/MeshBench/meshbench/internal/app/state"

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
			"pid": n.PID, "rss_bytes": n.RSSBytes, "cpu_ms": n.CPUms,
			"cpu_pct": n.CPUPct,
			"sent":    n.Sent, "heard": n.Heard,
			"last_sent_ms": n.LastSentMs, "last_heard_ms": n.LastHeardMs,
			"last_sent_to": n.LastSentTo, "last_heard_from": n.LastHeardFrom,
			// The chip's own counters: the only way to tell a busy mesh from
			// a radio that cries busy too readily.
			"irq_reads": n.IRQReads, "busy_reads": n.BusyReads,
			"busy_ms": n.BusyMs, "spurious": n.Spurious,
		})
	}
	return out
}
