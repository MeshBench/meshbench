package inventory

import (
	"encoding/json"
	"math"
	"testing"

	"github.com/MeshBench/meshbench/internal/app/state"
)

// One unrepresentable number must not cost the whole dump.
//
// A ratio against no noise at all is infinite, and encoding/json refuses it:
// "unsupported value: -Inf" aborted events.dump on the first such row and
// took a hundred thousand good ones with it, mid-write. JSON has null for
// "no value", which is what an infinite SNR honestly is.
func TestEventDumpSurvivesAnInfiniteSNR(t *testing.T) {
	for _, v := range []float64{math.Inf(-1), math.Inf(1), math.NaN()} {
		m := eventAsMap(state.Event{Kind: "miss", From: "a", To: "b", SNRdB: v})
		b, err := json.Marshal(m)
		if err != nil {
			t.Fatalf("SNR %v could not be encoded: %v", v, err)
		}
		var back map[string]any
		if err := json.Unmarshal(b, &back); err != nil {
			t.Fatal(err)
		}
		if back["snr_db"] != nil {
			t.Fatalf("SNR %v encoded as %v, want null", v, back["snr_db"])
		}
	}
	// A real reading still travels as a number.
	m := eventAsMap(state.Event{Kind: "rx", SNRdB: -7.5})
	if got := m["snr_db"]; got != -7.5 {
		t.Fatalf("a measurable SNR came out as %v", got)
	}
}
