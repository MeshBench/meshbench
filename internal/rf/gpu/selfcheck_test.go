package gpu

import (
	"strings"
	"testing"

	"github.com/MeshBench/meshbench/internal/rf/propagation"
)

// A device that answers correctly must pass its own gate - this is what
// gpuwarm.go trusts before ever handing the device a real coverage map.
func TestSelfCheckPassesOnARealDevice(t *testing.T) {
	d, err := Open()
	if err != nil {
		t.Skipf("no GPU available: %v", err)
	}
	defer d.Close()
	if err := d.SelfCheck(); err != nil {
		t.Fatalf("a correct device failed its own self-check: %v", err)
	}
}

// compareFoldSlots is what checkFold hands a divergent adapter's answer to;
// the d3d12 incident SelfCheck exists for was the coverage kernel disagreeing
// by nearly 10 dB while looking otherwise fine, so this injects the same
// shape of fault directly - a slot the device got wrong by more than noise -
// without needing a driver that actually misbehaves to prove the gate closes.
func TestCompareFoldSlotsCatchesDivergence(t *testing.T) {
	want := []propagation.FoldSlot{
		{MinDB: 4.2, OutDB: 5.0, InDB: 4.2, Station: 0},
		{MinDB: -1.5, OutDB: -1.5, InDB: 3.0, Station: 1},
	}

	t.Run("agreement passes", func(t *testing.T) {
		got := append([]propagation.FoldSlot(nil), want...)
		if err := compareFoldSlots("fold best", got, want); err != nil {
			t.Fatalf("identical slots must not be flagged: %v", err)
		}
	})

	t.Run("noise-sized drift passes", func(t *testing.T) {
		got := append([]propagation.FoldSlot(nil), want...)
		got[0].MinDB += foldTolerance / 2
		if err := compareFoldSlots("fold best", got, want); err != nil {
			t.Fatalf("drift within tolerance must not be flagged: %v", err)
		}
	})

	t.Run("wrong margin is caught", func(t *testing.T) {
		got := append([]propagation.FoldSlot(nil), want...)
		got[0].MinDB += 10 // the d3d12 incident's own order of magnitude
		err := compareFoldSlots("fold best", got, want)
		if err == nil {
			t.Fatal("a 10 dB divergence on the winning margin must be caught")
		}
		if !strings.Contains(err.Error(), "fold best") {
			t.Fatalf("error should name which slot set diverged: %v", err)
		}
	})

	t.Run("wrong station is caught", func(t *testing.T) {
		got := append([]propagation.FoldSlot(nil), want...)
		// Names the wrong station, and clear of a near-tie margin, so this
		// is the fault itself rather than the float-order swap the next
		// case exists to tell it apart from.
		got[1].Station, got[1].MinDB = 0, want[1].MinDB+10
		if err := compareFoldSlots("fold second", got, want); err == nil {
			t.Fatal("a slot naming the wrong station must be caught")
		}
	})

	t.Run("station swap at a genuine tie is not a defect", func(t *testing.T) {
		tied := []propagation.FoldSlot{{MinDB: 4.2, OutDB: 5.0, InDB: 4.2, Station: 0}}
		got := []propagation.FoldSlot{{MinDB: 4.2 + foldTolerance/2, OutDB: 5.0, InDB: 4.2, Station: 1}}
		if err := compareFoldSlots("fold best", got, tied); err != nil {
			t.Fatalf("a rank swap within tolerance of a tie must not be flagged: %v", err)
		}
	})
}
