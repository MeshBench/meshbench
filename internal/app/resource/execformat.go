package resource

import (
	"encoding/binary"
	"errors"
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
	peAMD64   execFormat = "a 64-bit x86 Windows executable"
	peARM64   execFormat = "a 64-bit ARM Windows executable"
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

// A PE says almost nothing about itself in its first bytes. "MZ" is the DOS
// stub, which every Windows binary since 1985 begins with and which says
// nothing about architecture; the part that does is a COFF header at an offset
// stored at 0x3C. So this format needs 64 bytes of head and then a read
// somewhere else, which is why formatOf takes the file rather than only its
// first bytes.
const (
	peLfanewAt      = 0x3C
	peMachineAMD64  = 0x8664
	peMachineARM64  = 0xAA64
	peHeaderMinRead = peLfanewAt + 4
)

// checkExecutable reads enough of a file's head to say whether it is what was
// expected, and says what it found instead when it is not.
func checkExecutable(path string, want execFormat) error {
	f, err := os.Open(path) //nolint:gosec // the tool this fetch just wrote
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	// Enough for a PE's e_lfanew, and a short read is not fatal: an ELF says
	// what it is in twenty bytes and a Mach-O in eight, so a file too small to
	// be a PE can still be identified as one of those.
	head := make([]byte, peHeaderMinRead)
	n, err := io.ReadFull(f, head)
	if n < 8 {
		return fmt.Errorf("resource: %s is too short to be %s", path, want)
	}
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) {
		return err
	}
	got, err := formatOf(head[:n], f)
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

// formatOf names what a file says it is.
//
// ELF and Mach-O are answered from the head alone. A PE is not: its
// architecture lives in a COFF header elsewhere in the file, so this takes the
// file as well and reads it.
func formatOf(head []byte, at io.ReaderAt) (execFormat, error) {
	if len(head) >= 20 && string(head[:4]) == "\x7fELF" {
		switch binary.LittleEndian.Uint16(head[18:20]) {
		case elfMachineAMD64:
			return elfAMD64, nil
		case elfMachineARM64:
			return elfARM64, nil
		}
		return "", fmt.Errorf("an ELF executable for an architecture nothing here publishes")
	}
	if len(head) >= 8 && binary.LittleEndian.Uint32(head[0:4]) == machO64LE {
		if binary.LittleEndian.Uint32(head[4:8]) == machCPUARM64 {
			return machARM64, nil
		}
		return "", fmt.Errorf("a Mach-O executable for an architecture nothing here publishes")
	}
	if len(head) >= peHeaderMinRead && head[0] == 'M' && head[1] == 'Z' {
		return peFormat(head, at)
	}
	return "", fmt.Errorf("no ELF, Mach-O or PE header")
}

// peFormat follows the DOS stub to the COFF header and reads the machine word.
func peFormat(head []byte, at io.ReaderAt) (execFormat, error) {
	off := int64(binary.LittleEndian.Uint32(head[peLfanewAt : peLfanewAt+4]))
	// A signature and a machine word: "PE\0\0" then two bytes. Bounded because
	// e_lfanew is attacker-controlled in a file we just downloaded, and a wild
	// offset should read as a malformed executable rather than a large read.
	if off < peHeaderMinRead || off > 1<<20 {
		return "", fmt.Errorf("a PE executable whose header offset (%d) is not credible", off)
	}
	var coff [6]byte
	if _, err := at.ReadAt(coff[:], off); err != nil {
		return "", fmt.Errorf("a PE executable with no COFF header where it said one was")
	}
	if string(coff[:4]) != "PE\x00\x00" {
		return "", fmt.Errorf("a DOS executable rather than a Windows one")
	}
	switch binary.LittleEndian.Uint16(coff[4:6]) {
	case peMachineAMD64:
		return peAMD64, nil
	case peMachineARM64:
		return peARM64, nil
	}
	return "", fmt.Errorf("a PE executable for an architecture nothing here publishes")
}
