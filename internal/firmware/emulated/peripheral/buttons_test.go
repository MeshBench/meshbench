package peripheral

import (
	"fmt"
	"io"
	"net"
	"testing"
	"time"
)

// A reading set before the emulator existed is not lost.
//
// The cell's first reading is known when a node is placed, which is before any
// machine is started to run it. QEMU takes it as a machine argument; Renode has
// no such argument, and without this its converter kept the constant its model
// starts at - so a Heltec reported a cell at about eleven volts and still
// looked like a working battery meter.
func TestAReadingSetBeforeTheEmulatorArrivesReachesIt(t *testing.T) {
	b, err := ListenButtonsTCP()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = b.Close() }()
	b.Preset(2, 0x0BB8)

	c, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", b.Port()))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = c.Close() }()

	if err := c.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
		t.Fatal(err)
	}
	var got [8]byte
	if _, err := io.ReadFull(c, got[:]); err != nil {
		t.Fatalf("nothing arrived on connecting: %v", err)
	}
	if got[0] != 'A' {
		t.Fatalf("the first message is tagged %q, want a converter reading", got[0])
	}
	if ch := int(got[1]); ch != 2 {
		t.Errorf("channel %d, want 2", ch)
	}
	if raw := uint16(got[2]) | uint16(got[3])<<8; raw != 0x0BB8 {
		t.Errorf("raw %d, want %d", raw, 0x0BB8)
	}
}
