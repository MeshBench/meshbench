package main

import (
	"testing"

	"github.com/A13xB0/meshcoresim/internal/gui/state"
)

// A row with fewer cells than there are columns does not fail: it shifts.
//
// One missing cell put every value one column to its left, so memory read as a
// dash, receptions were counted under "tx", and the default sort ordered the
// network by a column holding something else entirely. Nothing errored and
// every number looked plausible.
func TestEveryRowFillsEveryColumn(t *testing.T) {
	n := state.NodeStat{
		Name: "Abernethy Repeater", Backend: "native",
		Firmware: "repeater-v1.17.0", Running: true,
		RSSBytes: 4_194_304, CPUms: 1234, CPUPct: 1.5,
		Sent: 3, Heard: 9, Spurious: 1,
	}
	cols, cells := nodeColumns(), nodeCells(n, "running")
	if len(cells) != len(cols) {
		t.Fatalf("%d cells for %d columns: every value after the gap sits under the wrong heading",
			len(cells), len(cols))
	}
	for i, c := range cols {
		switch c.Title {
		case "memory":
			if cells[i] != "4.2 MB" {
				t.Errorf("memory column holds %q, want 4.2 MB", cells[i])
			}
		case "tx":
			if cells[i] != "3" {
				t.Errorf("tx column holds %q, want 3 - it is showing another column's number", cells[i])
			}
		case "rx":
			if cells[i] != "9" {
				t.Errorf("rx column holds %q, want 9", cells[i])
			}
		}
	}
}
