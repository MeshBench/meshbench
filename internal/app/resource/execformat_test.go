package resource

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// What a downloaded emulator has to be before it is put where a node would find
// it, and what each failure has to say.
//
// The case this exists for is the right-sized download for the wrong machine.
// It unpacks cleanly and then produces "exec format error" from a process
// nobody is watching, which reaches the operator as a board that did not come
// up - a sentence that sends people to the firmware and the radio wiring before
// the file.

// elf builds the first bytes of an ELF for a given e_machine.
func elf(machine uint16) []byte {
	b := make([]byte, 20)
	copy(b, "\x7fELF")
	binary.LittleEndian.PutUint16(b[18:20], machine)
	return b
}

// macho builds the first bytes of a 64-bit Mach-O for a given cputype.
func macho(cpu uint32) []byte {
	b := make([]byte, 20)
	binary.LittleEndian.PutUint32(b[0:4], machO64LE)
	binary.LittleEndian.PutUint32(b[4:8], cpu)
	return b
}

// pe builds a PE: a DOS stub carrying the offset of a COFF header, and the COFF
// header itself further in. lfanew is a parameter because a wild one is a case
// worth testing, being attacker-controlled in a file that was just downloaded.
func pe(machine uint16, lfanew uint32) []byte {
	size := int(lfanew) + 8
	if size < 64 {
		size = 64
	}
	b := make([]byte, size)
	b[0], b[1] = 'M', 'Z'
	binary.LittleEndian.PutUint32(b[peLfanewAt:peLfanewAt+4], lfanew)
	if int(lfanew)+6 <= len(b) {
		copy(b[lfanew:], "PE\x00\x00")
		binary.LittleEndian.PutUint16(b[lfanew+4:lfanew+6], machine)
	}
	return b
}

func written(t *testing.T, b []byte) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "tool")
	if err := os.WriteFile(p, b, 0o755); err != nil { //nolint:gosec // a stand-in in a temp dir
		t.Fatal(err)
	}
	return p
}

func TestEveryPublishedFormatIsRecognised(t *testing.T) {
	for _, c := range []struct {
		name string
		file []byte
		want execFormat
	}{
		{"linux x86", elf(elfMachineAMD64), elfAMD64},
		{"linux arm", elf(elfMachineARM64), elfARM64},
		{"macos arm", macho(machCPUARM64), machARM64},
		{"windows x86", pe(peMachineAMD64, 0x80), peAMD64},
		{"windows arm", pe(peMachineARM64, 0x80), peARM64},
	} {
		t.Run(c.name, func(t *testing.T) {
			if err := checkExecutable(written(t, c.file), c.want); err != nil {
				t.Errorf("a %s was refused: %v", c.want, err)
			}
		})
	}
}

// The whole point: the wrong architecture is refused, and the message says both
// what arrived and what was wanted, because "exec format error" hours later
// does not.
func TestTheWrongArchitectureIsRefusedAndSaysWhat(t *testing.T) {
	err := checkExecutable(written(t, pe(peMachineARM64, 0x80)), peAMD64)
	if err == nil {
		t.Fatal("an ARM Windows build passed as an x86 one")
	}
	for _, want := range []string{string(peARM64), string(peAMD64)} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the error does not name %q: %v", want, err)
		}
	}
}

// A PE is the one format that cannot be judged from its first bytes, so the
// ways it can lie about where its real header is all have to land somewhere
// sensible rather than on a large read or a panic.
func TestAPEThatLiesAboutItsHeader(t *testing.T) {
	for _, c := range []struct {
		name string
		file []byte
		says string
	}{
		{"an offset past any credible file", pe(peMachineAMD64, 1<<24), "not credible"},
		{"an offset inside its own stub", pe(peMachineAMD64, 4), "not credible"},
		{"a DOS binary with no PE header at all", func() []byte {
			b := pe(peMachineAMD64, 0x80)
			copy(b[0x80:], "NE\x00\x00") // a 16-bit Windows binary, which this is not
			return b
		}(), "DOS executable"},
	} {
		t.Run(c.name, func(t *testing.T) {
			err := checkExecutable(written(t, c.file), peAMD64)
			if err == nil {
				t.Fatal("accepted")
			}
			if !strings.Contains(err.Error(), c.says) {
				t.Errorf("want the error to mention %q, got: %v", c.says, err)
			}
		})
	}
}

// A short file is still identifiable when the format it claims needs fewer
// bytes than a PE does. Reading 64 bytes for everything must not turn a valid
// ELF into "too short".
func TestAShortFileIsStillReadWhereTheFormatAllowsIt(t *testing.T) {
	if err := checkExecutable(written(t, elf(elfMachineAMD64)), elfAMD64); err != nil {
		t.Errorf("a 20-byte ELF was refused: %v", err)
	}
	if err := checkExecutable(written(t, macho(machCPUARM64)[:8]), machARM64); err != nil {
		t.Errorf("an 8-byte Mach-O head was refused: %v", err)
	}
	if err := checkExecutable(written(t, []byte("MZ")), peAMD64); err == nil {
		t.Error("two bytes were accepted as a Windows executable")
	}
}
