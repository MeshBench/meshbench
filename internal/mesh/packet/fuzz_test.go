package packet_test

import (
	"encoding/hex"
	"testing"

	"github.com/MeshBench/meshbench/internal/mesh/packet"
)

// FuzzDissect throws arbitrary bytes at the one parser standing between this
// simulator and every MeshCore frame it is handed - off the air, out of a
// pcap, or replayed from a live capture. None of those sources are trusted:
// a malformed frame must come back Truncated, never panic the goroutine
// reading it.
func FuzzDissect(f *testing.F) {
	f.Add(advertFrame())
	f.Add([]byte{0x01 | (0x04 << 2), 0x08, 0xAA}) // claims 8 path bytes, has 1
	f.Add([]byte{0x00 | (0x05 << 2), 0x34, 0x12, 0x78, 0x56, 0x01, 0x99, 0xAA, 0xBB, 0xCC})
	f.Add([]byte{0x11, 0x02, 0xAA, 0xBB, 0xDE, 0xAD, 0xBE, 0xEF})
	f.Add([]byte{0x11, 0x03, 0xAA, 0xBB, 0xCC, 0xDE, 0xAD})
	f.Add([]byte{0x02 | (0x09 << 2), 0x02, 0x14, 0xEC, 0xEF, 0xBE, 0xAD, 0xDE, 0, 0, 0, 0, 0x00})
	f.Add([]byte{0x01 | (0x07 << 2), 0x00, 0xA7})
	f.Add([]byte{0x01 | (0x0A << 2), 0x00, (3 << 4) | 0x03, 0x11, 0x22})
	f.Add([]byte{0x01 | (0x0B << 2), 0x00, 0x80, 0x01})
	f.Add([]byte{0x01 | (0x02 << 2) | (0x01 << 6), 0x00, 0xAA, 0xBB, 0xCC, 0xDD})
	f.Add([]byte{})
	f.Add([]byte{0x00})

	// The live ScotMesh frame that exposed the variable-hash format: a
	// three-byte-hash path read as a plain byte count claimed 128 bytes of
	// path in a 73-byte frame.
	if raw, err := hex.DecodeString("14c47500008003bf92968994795bd4303e4df3bed4e61ce83504" +
		"eece967eb3907a133494611d225f353299022705755204f1811d04896a21945c79ff43c37bddafbb1371b616e51d49"); err == nil {
		f.Add(raw)
	}

	f.Fuzz(func(t *testing.T, frame []byte) {
		d := packet.Dissect(frame)
		// Both call into the field walk again with whatever the dissector
		// decided about the frame's shape; a panic here is as much a bug as
		// one inside Dissect itself.
		_ = d.Summary()
		_ = d.HopCount()
		for i := 0; i < d.HopCount(); i++ {
			d.Hop(i)
		}
	})
}

// FuzzRewritePath fuzzes the one function that turns a dissected frame back
// into bytes. Its own test only ever exercises a two-byte path; this also
// checks the property the bug report was about directly - a frame
// RewritePath accepts must be one its own Dissect can read back without
// truncation, or RewritePath has built a frame its own parser cannot parse.
func FuzzRewritePath(f *testing.F) {
	f.Add([]byte{0x11, 0x02, 0xAA, 0xBB, 0xDE, 0xAD, 0xBE, 0xEF}, []byte{0x42, 0x42})
	f.Add(advertFrame(), []byte{})
	f.Add(advertFrame(), make([]byte, 63))
	f.Add(advertFrame(), make([]byte, 64))
	f.Add(advertFrame(), make([]byte, 255))
	f.Add([]byte{0x01 | (0x04 << 2), 0x08, 0xAA}, []byte{0x01})

	f.Fuzz(func(t *testing.T, frame, path []byte) {
		out, err := packet.RewritePath(frame, path)
		if err != nil {
			return
		}
		d := packet.Dissect(out)
		if d.Truncated {
			t.Fatalf("RewritePath accepted a %d-byte path but built a frame "+
				"its own Dissect calls truncated: %s", len(path), d.Problem)
		}
	})
}
