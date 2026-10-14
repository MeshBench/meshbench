package experiment

import (
	"encoding/json"
	"testing"

	"github.com/MeshBench/meshbench/internal/firmware"
)

// A cell sums every node's radio counters and keeps one row per node, so two
// arms whose delivery totals agree can still be told apart by what each
// firmware was told.
func TestACellSumsEveryNodesChipAndKeepsEachRow(t *testing.T) {
	var c ChipStats
	c.add("A", "companion", firmware.RadioStats{
		IRQReads: 100, BusyReads: 10, BusyMs: 500, IRQMask: 0x0272,
		IRQSuppressed: 7, SuppressedReported: true, SF: 8, BandwidthHz: 62500,
	})
	c.add("B", "simple-repeater", firmware.RadioStats{
		IRQReads: 50, BusyReads: 5, BusyMs: 250, IRQMask: 0x0276,
		IRQSuppressed: 3, SuppressedReported: true, SF: 8, BandwidthHz: 62500,
	})
	if c.IRQReads != 150 || c.BusyReads != 15 || c.BusyMs != 750 {
		t.Errorf("sums: reads %d busy %d busy_ms %d, want 150 15 750", c.IRQReads, c.BusyReads, c.BusyMs)
	}
	if c.IRQSuppressed != 10 || c.Reporting != 2 {
		t.Errorf("suppressed %d from %d nodes, want 10 from 2", c.IRQSuppressed, c.Reporting)
	}
	if len(c.Nodes) != 2 || c.Nodes[1].Name != "B" || c.Nodes[1].IRQMask != 0x0276 {
		t.Errorf("per-node rows: %+v", c.Nodes)
	}
}

// A chip that does not count suppressed events adds nothing to the total and
// is not counted as reporting, so a total of zero is not mistaken for a count
// of zero.
func TestAChipThatDoesNotCountIsNotCountedAsReporting(t *testing.T) {
	var c ChipStats
	c.add("old", "simple-repeater", firmware.RadioStats{IRQReads: 20, IRQSuppressed: 9})
	if c.IRQSuppressed != 0 || c.Reporting != 0 {
		t.Errorf("suppressed %d from %d nodes, want 0 from 0", c.IRQSuppressed, c.Reporting)
	}
	if c.Nodes[0].Reported {
		t.Error("the row says the chip reported a count it does not keep")
	}
}

// The counters reach the export under "chip", beside the cell's other numbers.
func TestTheExportCarriesEachCellsChipCounters(t *testing.T) {
	r := Result{Arm: "a", Chip: &ChipStats{IRQReads: 1, Nodes: []NodeChip{{Name: "A"}}}}
	raw, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	var back map[string]any
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatal(err)
	}
	chip, ok := back["chip"].(map[string]any)
	if !ok || chip["irq_reads"] != float64(1) {
		t.Errorf("export row: %s", raw)
	}
}

// What the chip counted while the mesh came up stays out of the cell, as the
// boot chatter stays out of its delivery totals; what the chip holds at the end
// is reported as it stands.
func TestACellCountsTheChipFromItsBaselineNotFromBoot(t *testing.T) {
	start := firmware.RadioStats{
		IRQReads: 40, BusyReads: 4, BusyMs: 200, SpuriousUp: 1, IRQSuppressed: 6,
		SuppressedReported: true, IRQMask: 0x0272, SF: 8, BandwidthHz: 62500,
	}
	end := firmware.RadioStats{
		IRQReads: 100, BusyReads: 10, BusyMs: 500, SpuriousUp: 3, IRQSuppressed: 9,
		SuppressedReported: true, IRQMask: 0x0276, SF: 8, BandwidthHz: 62500,
	}
	got := since(end, start)
	if got.IRQReads != 60 || got.BusyReads != 6 || got.BusyMs != 300 ||
		got.SpuriousUp != 2 || got.IRQSuppressed != 3 {
		t.Errorf("counters %+v, want 60 6 300 2 3 counted from the baseline", got)
	}
	if got.IRQMask != 0x0276 || got.SF != 8 || got.BandwidthHz != 62500 || !got.SuppressedReported {
		t.Errorf("state %+v, want the chip as it ended", got)
	}
}

// A node that restarted mid-cell starts its counters again from zero, so
// everything it holds came after the baseline. Subtracting the larger figure
// from before would wrap to four billion.
func TestANodeThatRestartedMidCellReportsWhatItCountedSince(t *testing.T) {
	got := since(firmware.RadioStats{IRQReads: 15}, firmware.RadioStats{IRQReads: 40})
	if got.IRQReads != 15 {
		t.Errorf("irq_reads %d, want 15", got.IRQReads)
	}
}
