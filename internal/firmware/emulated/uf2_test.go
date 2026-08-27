package emulated

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

func uf2blk(addr uint32, payload []byte) []byte {
	b := make([]byte, uf2Block)
	binary.LittleEndian.PutUint32(b[0:], uf2Magic0)
	binary.LittleEndian.PutUint32(b[4:], uf2Magic1)
	binary.LittleEndian.PutUint32(b[12:], addr)
	binary.LittleEndian.PutUint32(b[16:], uint32(len(payload)))
	copy(b[32:], payload)
	return b
}

// The whole point of splitting is that a bootloader package is several images
// in one file: joined blindly, three of its four regions land at the wrong
// address and the result looks like a bootloader that does not work.
func TestSplitUF2SeparatesDiscontiguousRegions(t *testing.T) {
	var file []byte
	file = append(file, uf2blk(0x26000, []byte{1, 2, 3, 4})...)
	file = append(file, uf2blk(0x26004, []byte{5, 6, 7, 8})...)
	file = append(file, uf2blk(0xF4000, []byte{9})...)

	path := filepath.Join(t.TempDir(), "x.uf2")
	if err := os.WriteFile(path, file, 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := SplitUF2(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d regions, want 2", len(got))
	}
	if got[0].Addr != 0x26000 || len(got[0].Data) != 8 {
		t.Errorf("first region is 0x%X/%d bytes, want 0x26000/8", got[0].Addr, len(got[0].Data))
	}
	if got[1].Addr != 0xF4000 || len(got[1].Data) != 1 {
		t.Errorf("second region is 0x%X/%d bytes, want 0xF4000/1", got[1].Addr, len(got[1].Data))
	}
}

func TestSplitUF2RejectsAFileWithNoBlocks(t *testing.T) {
	path := filepath.Join(t.TempDir(), "x.uf2")
	if err := os.WriteFile(path, make([]byte, uf2Block*2), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := SplitUF2(path); err == nil {
		t.Fatal("a file with no UF2 blocks was accepted")
	}
}
