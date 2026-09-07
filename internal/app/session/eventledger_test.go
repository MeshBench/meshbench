package session

import (
	"testing"

	"github.com/MeshBench/meshbench/internal/sim/engine"
)

// events.dump is the documented way to get a run's whole ledger out for
// analysis, and it used to write the snapshot's bounded tail: a run of 2732
// events produced a file of 2000 lines. The tail exists to bound what the
// tables draw; a file has no drawing budget to protect.
func TestTheLedgerIsNotTheReadoutTail(t *testing.T) {
	eng := engine.New(nil, engine.Config{StepMs: 10})
	defer func() { _ = eng.Close() }()

	const many = readoutTail + 500
	for i := 0; i < many; i++ {
		eng.RecordForTest(engine.Event{Kind: "tx", From: "a", AtMs: uint32(i)})
	}
	s := &Sim{eng: eng}

	if _, total := s.eventTail(readoutTail); total != many {
		t.Fatalf("the engine holds %d events, the tail reports %d", many, total)
	}
	got, total := s.EventLedger()
	if total != many {
		t.Errorf("the ledger reports %d events, want %d", total, many)
	}
	if len(got) != many {
		t.Errorf("the ledger returned %d events, want all %d", len(got), many)
	}
}
