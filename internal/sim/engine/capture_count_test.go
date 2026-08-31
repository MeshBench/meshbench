package engine_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/MeshBench/meshbench/internal/sim/engine"
)

// capturedAndOffered plays one frame from node 0 into a small mesh and reports
// what the capture recorded against what the ledger says actually arrived.
func capturedAndOffered(t *testing.T, mode engine.RFMode) (frames, offered int) {
	t.Helper()
	e := engine.New(flat{100}, engine.Config{StepMs: 10, Seed: 4417, RFMode: mode})
	e.Add(wfNode("a", 0, 22), nil)
	e.Add(wfNode("b", 0.010, 22), nil)
	e.Add(wfNode("c", 0.020, 22), nil)

	if err := e.StartCapture(filepath.Join(t.TempDir(), "run.pcapng")); err != nil {
		t.Fatal(err)
	}
	if err := e.Run(context.Background(), 10); err != nil {
		t.Fatal(err)
	}
	e.InjectFrame(0, []byte("one frame, one row per receiver"))
	if err := e.Run(context.Background(), 5000); err != nil {
		t.Fatal(err)
	}
	_, frames, err := e.StopCapture()
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range e.Ledger.Rows() {
		if r.Offered {
			offered++
		}
	}
	return frames, offered
}

// A capture is one row per receiver per frame, and a soak judges a run by that
// count. Written twice - once on the way out of the verdict and once again as
// the delivery unwound - it reported a mesh carrying twice the traffic it did,
// with every packet apparently received by two of everything.
func TestCaptureRecordsEachReceptionOnce(t *testing.T) {
	for _, mode := range []engine.RFMode{engine.RFCalculated, engine.RFWaveform} {
		frames, offered := capturedAndOffered(t, mode)
		if offered == 0 {
			t.Fatalf("%s: nothing measurable arrived, so the count proves nothing", mode)
		}
		if frames != offered {
			t.Errorf("%s: capture holds %d frames for %d offered receptions",
				mode, frames, offered)
		}
	}
}
