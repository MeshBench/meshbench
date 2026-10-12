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

// NodeChip is one node's counters at the end of the cell.
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

// chipStats reads every running node's chip. Called before the engine closes,
// while the bridges still hold the last record each node sent.
func chipStats(eng *engine.Engine) *ChipStats {
	out := &ChipStats{}
	for _, n := range eng.Nodes() {
		if n.Firmware == nil {
			continue
		}
		out.add(n.Spec().Name, string(n.Spec().Kind), n.Firmware.Bridge.Stats())
	}
	return out
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
