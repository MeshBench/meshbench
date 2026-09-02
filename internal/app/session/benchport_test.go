package session

import "testing"

// A node keeps the port it was served on, so serving it again lands there.
//
// The port was drawn fresh on every serve, and a companion client is pointed
// at an address by hand or by a script: a second serve that moved left it
// attached to a closed socket with no way to learn where the node had gone.
func TestANodeKeepsThePortItWasServedOn(t *testing.T) {
	s := &Sim{}
	if _, ok := s.servedPort("Cupar"); ok {
		t.Fatal("a node that has never been served already has a port")
	}
	s.rememberPort("Cupar", "0.0.0.0:41234")
	got, ok := s.servedPort("Cupar")
	if !ok {
		t.Fatal("a served node did not keep its port")
	}
	if got != 41234 {
		t.Errorf("kept port %d", got)
	}
	// One node's port is not another's.
	if _, ok := s.servedPort("West Lomond"); ok {
		t.Error("a second node inherited the first one's port")
	}
}

// Port zero is what the operating system was asked, not what it answered, and
// remembering it would draw a fresh port on every serve - the fault itself.
func TestPortZeroIsNotRemembered(t *testing.T) {
	s := &Sim{}
	s.rememberPort("Cupar", "0.0.0.0:0")
	if _, ok := s.servedPort("Cupar"); ok {
		t.Error("port zero was kept as though it were an address")
	}
	s.rememberPort("Cupar", "not-an-address")
	if _, ok := s.servedPort("Cupar"); ok {
		t.Error("an unparseable address was kept as a port")
	}
}

func TestPortOfReadsTheListenersOwnAddress(t *testing.T) {
	if got := PortOf("127.0.0.1:41234"); got != 41234 {
		t.Errorf("read port %d", got)
	}
	if got := PortOf("[::]:43437"); got != 43437 {
		t.Errorf("an IPv6 bind address read port %d", got)
	}
	if got := PortOf("not-an-address"); got != 0 {
		t.Errorf("nonsense read as port %d", got)
	}
}
