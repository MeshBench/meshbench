package session

import "testing"

// A peripheral that exits after its node was served again must not take the
// newer run's name with it, and serving or dropping a node forgets the name.
func TestABLEPeripheralExitingLateClearsOnlyItsOwnName(t *testing.T) {
	s := &Sim{}
	old := s.setBLE("R1", "MSIM-R1")
	s.setBLE("R1", "MSIM-R1-again")
	s.clearBLE("R1", old)
	if got, ok := s.bleName("R1"); !ok || got != "MSIM-R1-again" {
		t.Fatalf("the old run cleared the new one: %q, %v", got, ok)
	}
	s.clearBLE("R1", 0)
	if _, ok := s.bleName("R1"); ok {
		t.Fatal("serving again left the node advertised over BLE")
	}
}
