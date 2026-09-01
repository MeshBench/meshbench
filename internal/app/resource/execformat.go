package resource

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"
)

// What a downloaded executable has to be, checked before it is put where the
// emulator lookup would find it.
//
// A build for the wrong architecture is the failure worth catching here. It
// downloads cleanly, unpacks cleanly, and then produces "exec format error"
// from a process nobody is watching, which reaches the operator as an emulated
// board that did not come up - a sentence that sends people to look at the
// board, the firmware and the radio wiring before the file. The digest catches
// a truncated download; this catches a right-sized one for the wrong machine.
type execFormat string

const (
	elfAMD64  execFormat = "a 64-bit x86 ELF executable"
	elfARM64  execFormat = "a 64-bit ARM ELF executable"
	machARM64 execFormat = "a 64-bit ARM Mach-O executable"
)

// ELF's e_machine values, and Mach-O's cputype, for the two architectures
// anything here is published for.
const (
	elfMachineAMD64 = 0x3E
	elfMachineARM64 = 0xB7
	machCPUARM64    = 0x0100000C
)

// machO64LE is the 64-bit Mach-O magic as it appears on disk, little-endian.
const machO64LE = 0xFEEDFACF

// checkExecutable reads enough of a file's head to say whether it is what was
// expected, and says what it found instead when it is not.
func checkExecutable(path string, want execFormat) error {
	f, err := os.Open(path) //nolint:gosec // the tool this fetch just wrote
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	var head [20]byte
	if _, err := io.ReadFull(f, head[:]); err != nil {
		return fmt.Errorf("resource: %s is too short to be %s", path, want)
	}
	got, err := formatOf(head)
	if err != nil {
		return fmt.Errorf("resource: %s is not an executable at all: %w", path, err)
	}
	if got != want {
		return fmt.Errorf("resource: %s is %s, and this machine needs %s - "+
			"the wrong build was published for it, or the wrong one was asked for",
			path, got, want)
	}
	return nil
}

// formatOf names what a file's first bytes say it is.
func formatOf(head [20]byte) (execFormat, error) {
	if string(head[:4]) == "\x7fELF" {
		switch binary.LittleEndian.Uint16(head[18:20]) {
		case elfMachineAMD64:
			return elfAMD64, nil
		case elfMachineARM64:
			return elfARM64, nil
		}
		return "", fmt.Errorf("an ELF executable for an architecture nothing here publishes")
	}
	if binary.LittleEndian.Uint32(head[0:4]) == machO64LE {
		if binary.LittleEndian.Uint32(head[4:8]) == machCPUARM64 {
			return machARM64, nil
		}
		return "", fmt.Errorf("a Mach-O executable for an architecture nothing here publishes")
	}
	return "", fmt.Errorf("no ELF or Mach-O header")
}
