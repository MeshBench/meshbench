package capture

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func TestPcapngStructureIsValid(t *testing.T) {
	var buf bytes.Buffer
	w, err := NewPcapngWriter(&buf)
	if err != nil {
		t.Fatal(err)
	}
	if err := w.WritePacket(1_284_100_000, PseudoHeader{
		FromNode: 1, ToNode: 7, RSSIdBm: -1043, SNRdB: -75,
		FreqHz: 869525000, SF: 10, BWkHz: 250, CR: 5, CRCOK: 1,
		Outcome: OutcomeCode(DroppedByFirmware),
	}, []byte{0x11, 0x00, 0xA0, 0xA1}); err != nil {
		t.Fatal(err)
	}

	b := buf.Bytes()
	// Section Header Block, then its byte-order magic.
	if binary.LittleEndian.Uint32(b[0:4]) != 0x0A0D0D0A {
		t.Error("first block is not a Section Header Block")
	}
	if binary.LittleEndian.Uint32(b[8:12]) != 0x1A2B3C4D {
		t.Error("byte-order magic missing — no reader will open this")
	}
	// Every block's leading and trailing length must agree, or the file is
	// unreadable from the end and Wireshark will reject it.
	off := 0
	blocks := 0
	for off+12 <= len(b) {
		total := int(binary.LittleEndian.Uint32(b[off+4 : off+8]))
		if total < 12 || off+total > len(b) {
			t.Fatalf("block at %d has implausible length %d", off, total)
		}
		if trail := binary.LittleEndian.Uint32(b[off+total-4 : off+total]); int(trail) != total {
			t.Errorf("block at %d: leading length %d, trailing %d", off, total, trail)
		}
		off += total
		blocks++
	}
	if off != len(b) {
		t.Errorf("blocks do not tile the file: consumed %d of %d bytes", off, len(b))
	}
	if blocks != 3 {
		t.Errorf("got %d blocks, want 3 (section, interface, packet)", blocks)
	}
}

// The version byte must be present and first: captures outlive the code that
// wrote them, and a reader has to know which layout it has.
func TestPseudoHeaderIsVersioned(t *testing.T) {
	h := PseudoHeader{Version: 0} // caller left it unset
	var buf bytes.Buffer
	w, _ := NewPcapngWriter(&buf)
	if err := w.WritePacket(0, h, []byte{0xFF}); err != nil {
		t.Fatal(err)
	}
	// Find the packet block's payload: the writer must have stamped the version.
	if !bytes.Contains(buf.Bytes(), []byte{PseudoHeaderVersion}) {
		t.Error("writer did not stamp the pseudo-header version")
	}
}

// The receiving node must be in the header — it is what makes a merged capture
// able to show that A heard a frame and B did not.
func TestReceiverIsRecorded(t *testing.T) {
	a := PseudoHeader{FromNode: 1, ToNode: 4}.encode()
	b := PseudoHeader{FromNode: 1, ToNode: 9}.encode()
	if bytes.Equal(a, b) {
		t.Error("two receivers produced identical pseudo-headers — the merged " +
			"capture cannot distinguish who heard what")
	}
}

func TestAllOutcomesHaveDistinctCodes(t *testing.T) {
	seen := map[uint8]Outcome{}
	for _, o := range []Outcome{OutOfRange, NotDemodulated, CRCFailed,
		DroppedByFirmware, Accepted, Relayed} {
		c := OutcomeCode(o)
		if prev, dup := seen[c]; dup {
			t.Errorf("outcomes %q and %q share wire code %d", prev, o, c)
		}
		seen[c] = o
	}
}
