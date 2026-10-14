package nodeview

import (
	"testing"

	"github.com/MeshBench/meshbench/internal/app/state"
)

// A chip that does not keep the suppressed count says so, rather than reading
// as a firmware that enabled every interrupt it needed.
func TestASuppressedCountNobodyKeptIsNotAZero(t *testing.T) {
	row := suppressedRow(state.NodeStat{IRQSuppressed: 0})
	if row.value != "not counted" {
		t.Errorf("a chip without the count reads %q", row.value)
	}
	if row.warn {
		t.Error("a chip without the count is marked as worth a second look")
	}
}

// A counted figure is shown as it is, and anything above zero is worth a
// second look: an event the firmware never enabled is one it cannot act on.
func TestASuppressedCountIsShownAndMarked(t *testing.T) {
	none := suppressedRow(state.NodeStat{SuppressedReported: true})
	if none.value != "0" || none.warn {
		t.Errorf("a chip that kept nothing out reads %q, warn %v", none.value, none.warn)
	}
	some := suppressedRow(state.NodeStat{IRQSuppressed: 14, SuppressedReported: true})
	if some.value != "14" || !some.warn {
		t.Errorf("fourteen suppressed events read %q, warn %v", some.value, some.warn)
	}
}
