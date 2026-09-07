package session

import (
	"testing"

	"github.com/MeshBench/meshbench/internal/app/state"
)

// 0 CPU and 0 RSS for every node on a Mac is not a light mesh, it is a sampler
// reading /proc on a platform that has none. A caller reading a number
// believes it, and there was no way to tell that zero from a node which
// genuinely costs nothing.
func TestAnUnmeasuredCostIsNotZero(t *testing.T) {
	rows := statRows([]state.NodeStat{{
		Name: "unmeasured", PID: 42, CostMeasured: false,
	}, {
		Name: "measured", PID: 43, CostMeasured: true,
		RSSBytes: 4096, CPUms: 120, CPUPct: 2.5,
	}})
	if len(rows) != 2 {
		t.Fatalf("got %d rows", len(rows))
	}

	if got := rows[0]["rss_bytes"]; got != nil {
		t.Errorf("an unmeasured RSS came back as %v, want nothing", got)
	}
	if got := rows[0]["cpu_pct"]; got != nil {
		t.Errorf("an unmeasured CPU came back as %v, want nothing", got)
	}
	if rows[0]["cost_measured"] != false {
		t.Error("the row does not say its cost was never measured")
	}

	if got := rows[1]["rss_bytes"]; got != float64(4096) {
		t.Errorf("a measured RSS came back as %v", got)
	}
	if got := rows[1]["cpu_pct"]; got != 2.5 {
		t.Errorf("a measured CPU came back as %v", got)
	}
	// A node that genuinely costs nothing still reports a number, which is
	// the distinction this exists to keep.
	zero := statRows([]state.NodeStat{{Name: "idle", PID: 44, CostMeasured: true}})
	if got := zero[0]["cpu_pct"]; got != float64(0) {
		t.Errorf("a measured idle node came back as %v, want 0", got)
	}
}
