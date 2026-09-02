package firmware

import (
	"encoding/binary"
	"fmt"
	"runtime"
)

// What a downloaded native build has to be before this machine tries to run it.
//
// A build arrives, is written executable, and fifty-six nodes then start and
// exit 1 at once with nothing saying why. The digest paths catch a truncated
// download; nothing caught a whole one that is not a program for this machine -
// an error page served with 200, a proxy interstitial, an asset published for
// the wrong platform. All of them read as a firmware fault.
//
// internal/app/resource/execformat.go does this same job for the emulator
// toolchain, and for the same reason. It is not shared because that package
// sits in the app layer and this one is below it, and a header check is
// cheaper to state twice than a new layer is to justify.

// wantExecutable reports what this platform's binaries begin with, and what to
// call it when the bytes say something else.
func checkNativeExecutable(name string, body []byte) error {
	got, err := nativeFormatOf(body)
	if err != nil {
		return fmt.Errorf("firmware: %s is not a program for this machine: %w", name, err)
	}
	if want := runtime.GOOS + "/" + runtime.GOARCH; got != want {
		return fmt.Errorf("firmware: %s is a %s executable and this machine needs %s",
			name, got, want)
	}
	return nil
}

// nativeFormatOf names what a file's first bytes say it is, in the same
// goos/goarch spelling the caller compares against.
func nativeFormatOf(body []byte) (string, error) {
	if len(body) < 20 {
		return "", fmt.Errorf("only %d bytes, too short to be an executable", len(body))
	}
	switch {
	case string(body[:4]) == "\x7fELF":
		switch binary.LittleEndian.Uint16(body[18:20]) {
		case 0x3E:
			return "linux/amd64", nil
		case 0xB7:
			return "linux/arm64", nil
		}
		return "", fmt.Errorf("an ELF executable for an architecture nothing here publishes")
	case binary.LittleEndian.Uint32(body[:4]) == 0xFEEDFACF:
		switch binary.LittleEndian.Uint32(body[4:8]) {
		case 0x0100000C:
			return "darwin/arm64", nil
		case 0x01000007:
			return "darwin/amd64", nil
		}
		return "", fmt.Errorf("a Mach-O executable for an architecture nothing here publishes")
	case string(body[:2]) == "MZ":
		// The COFF machine word sits at the offset the DOS header names, and a
		// PE that claims an offset past its own end is not one.
		off := int(binary.LittleEndian.Uint32(body[0x3C:0x40]))
		if off+6 > len(body) || string(body[off:off+4]) != "PE\x00\x00" {
			return "", fmt.Errorf("an MZ header with no PE signature after it")
		}
		switch binary.LittleEndian.Uint16(body[off+4 : off+6]) {
		case 0x8664:
			return "windows/amd64", nil
		case 0xAA64:
			return "windows/arm64", nil
		}
		return "", fmt.Errorf("a PE executable for an architecture nothing here publishes")
	}
	return "", fmt.Errorf("no ELF, Mach-O or PE header; it begins %q", head(body))
}

// head is the opening of a body, printable, for a message that has to say what
// arrived instead. An HTML error page served with 200 is the case this names.
func head(body []byte) string {
	n := min(len(body), 40)
	return string(body[:n])
}
