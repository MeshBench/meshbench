package session

import (
	"testing"

	"github.com/MeshBench/meshbench/internal/app/state"
)

// The row carries the node's own airtime, matched by name, and a node with no
// score carries none rather than a zero that reads as "never transmitted".
func TestStatRowsCarryAirtimeByName(t *testing.T) {
	rows := withAirtime(statRows([]state.NodeStat{{Name: "Alpha"}, {Name: "Bravo"}}),
		[]state.Score{{Name: "Alpha", AirtimeMs: 1234.5, DutyCyclePct: 2.5}})
	if rows[0]["airtime_ms"] != 1234.5 || rows[0]["duty_cycle_pct"] != 2.5 {
		t.Errorf("Alpha's row = %+v", rows[0])
	}
	if _, ok := rows[1]["airtime_ms"]; ok {
		t.Errorf("Bravo has no score but its row says %v", rows[1]["airtime_ms"])
	}
}
