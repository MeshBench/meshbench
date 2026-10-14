package experiment

import (
	"github.com/MeshBench/meshbench/internal/firmware"
	"github.com/MeshBench/meshbench/internal/sim/engine"
)

// ChipStats is what each node's chip saw during a cell, summed and per node.
//
// Delivery totals say what the mesh did; these say what the firmware was told.
// They are the only way to tell a busy channel from a radio that cries busy,
// and - on a chip that applies IrqMask - a firmware that never enabled an
// interrupt from one that never had cause to see it. Two builds whose radio
// drivers differ and whose totals agree are not shown to behave alike until
// these agree too.
type ChipStats struct {
	IRQReads      uint64 `json:"irq_reads"`
	BusyReads     uint64 `json:"busy_reads"`
	BusyMs        uint64 `json:"busy_ms"`
	Spurious      uint64 `json:"spurious"`
	IRQSuppressed uint64 `json:"irq_suppressed"`
	// Reporting is how many nodes sent a suppressed count at all. A chip that
	// ignores the mask sends none, and its total of zero means unknown.
	Reporting int        `json:"suppressed_reporting"`
	Nodes     []NodeChip `json:"nodes"`
}

// NodeChip is what one node's chip counted over the cell, and how it was
// set when the cell ended.
type NodeChip struct {
	Name          string `json:"name"`
	Kind          string `json:"kind"`
	IRQReads      uint32 `json:"irq_reads"`
	BusyReads     uint32 `json:"busy_reads"`
	BusyMs        uint32 `json:"busy_ms"`
	Spurious      uint32 `json:"spurious"`
	IRQSuppressed uint32 `json:"irq_suppressed"`
	Reported      bool   `json:"suppressed_reported"`
	IRQMask       uint16 `json:"irq_mask"`
	SF            uint8  `json:"sf"`
	BandwidthHz   uint32 `json:"bandwidth_hz"`
}

// chipBaseline is every running node's chip as the cell's own traffic begins.
//
// The chip counts from boot, and the cell's other numbers do not: the ledger
// and the margin counters both start after the adverts have settled. Without
// this, the counters would include the mesh coming up while the delivery
// totals beside them do not, and two arms that booted differently would look
// as though they flooded differently.
func chipBaseline(eng *engine.Engine) map[string]firmware.RadioStats {
	out := map[string]firmware.RadioStats{}
	for _, n := range eng.Nodes() {
		if n.Firmware != nil {
			out[n.Spec().Name] = n.Firmware.Bridge.Stats()
		}
	}
	return out
}

// chipStats reads every running node's chip since before. Called before the
// engine closes, while the bridges still hold the last record each node sent.
func chipStats(eng *engine.Engine, before map[string]firmware.RadioStats) *ChipStats {
	out := &ChipStats{}
	for _, n := range eng.Nodes() {
		if n.Firmware == nil {
			continue
		}
		name := n.Spec().Name
		out.add(name, string(n.Spec().Kind), since(n.Firmware.Bridge.Stats(), before[name]))
	}
	return out
}

// since is end with its counters measured from start. The mask, the modem
// settings and whether a suppressed count is kept are what the chip holds at
// the end, not counts, so they are end's alone.
func since(end, start firmware.RadioStats) firmware.RadioStats {
	end.IRQReads = counted(end.IRQReads, start.IRQReads)
	end.BusyReads = counted(end.BusyReads, start.BusyReads)
	end.BusyMs = counted(end.BusyMs, start.BusyMs)
	end.SpuriousUp = counted(end.SpuriousUp, start.SpuriousUp)
	end.IRQSuppressed = counted(end.IRQSuppressed, start.IRQSuppressed)
	return end
}

// counted is how far a counter moved. One that went backwards was restarted
// with its node, so everything it holds was counted after the baseline;
// subtracting would wrap to four billion.
func counted(end, start uint32) uint32 {
	if end < start {
		return end
	}
	return end - start
}

// add folds one node's radio record into the cell's counters.
func (c *ChipStats) add(name, kind string, r firmware.RadioStats) {
	c.IRQReads += uint64(r.IRQReads)
	c.BusyReads += uint64(r.BusyReads)
	c.BusyMs += uint64(r.BusyMs)
	c.Spurious += uint64(r.SpuriousUp)
	if r.SuppressedReported {
		c.IRQSuppressed += uint64(r.IRQSuppressed)
		c.Reporting++
	}
	c.Nodes = append(c.Nodes, NodeChip{
		Name: name, Kind: kind,
		IRQReads: r.IRQReads, BusyReads: r.BusyReads, BusyMs: r.BusyMs,
		Spurious: r.SpuriousUp, IRQSuppressed: r.IRQSuppressed,
		Reported: r.SuppressedReported, IRQMask: r.IRQMask,
		SF: r.SF, BandwidthHz: r.BandwidthHz,
	})
}
