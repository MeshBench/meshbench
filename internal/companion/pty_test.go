package companion

import (
	"encoding/binary"
	"io"
	"os"
	"runtime"
	"testing"
	"time"
)

func TestPTYRoundTrip(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("PTY transport is Linux-only for now")
	}
	n := newFake()
	p, err := OpenPTY(n)
	if err != nil {
		t.Skipf("no PTY available: %v", err)
	}
	defer p.Close()
	t.Logf("node exposed at %s", p.Path)

	slave, err := os.OpenFile(p.Path, os.O_RDWR, 0)
	if err != nil {
		t.Fatalf("open slave: %v", err)
	}
	defer slave.Close()

	// client -> node
	var hdr [2]byte
	payload := []byte{0x11, 0x00, 0xA0, 0xA1}
	binary.BigEndian.PutUint16(hdr[:], uint16(len(payload)))
	if _, err := slave.Write(append(hdr[:], payload...)); err != nil {
		t.Fatal(err)
	}
	select {
	case got := <-n.written:
		if string(got) != string(payload) {
			t.Errorf("node got % x, want % x", got, payload)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("node never received the frame written to the PTY")
	}

	// node -> client
	n.out <- Frame{0x22, 0xBB, 0xCC}
	slave.SetReadDeadline(time.Now().Add(3 * time.Second))
	if _, err := io.ReadFull(slave, hdr[:]); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, binary.BigEndian.Uint16(hdr[:]))
	if _, err := io.ReadFull(slave, buf); err != nil {
		t.Fatal(err)
	}
	if string(buf) != string([]byte{0x22, 0xBB, 0xCC}) {
		t.Errorf("client read % x", buf)
	}
}

// The slave path is what a user types into screen or a config tool, so it must
// be a real device path rather than an opaque handle.
func TestPTYPathIsUsable(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux-only")
	}
	p, err := OpenPTY(newFake())
	if err != nil {
		t.Skipf("no PTY available: %v", err)
	}
	defer p.Close()
	if _, err := os.Stat(p.Path); err != nil {
		t.Errorf("advertised path %s does not exist: %v", p.Path, err)
	}
}
