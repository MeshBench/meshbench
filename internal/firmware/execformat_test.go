package firmware

import (
	"encoding/binary"
	"runtime"
	"strings"
	"testing"
)

// A build that is complete and wrong is the case a digest cannot catch, and
// the one that reached the operator as every node exiting 1 at once with
// nothing saying why.
func TestABodyThatIsNotAProgramIsNamedForWhatItIs(t *testing.T) {
	for _, tc := range []struct {
		what string
		body []byte
		says string
	}{
		{"an error page served with 200",
			[]byte("<!DOCTYPE html><html><head><title>Not Found</title></head>"),
			"no ELF, Mach-O or PE header"},
		{"an empty body", nil, "too short"},
		{"a truncated header", []byte("\x7fELF\x02\x01"), "too short"},
		{"an MZ stub with no PE after it",
			append([]byte("MZ"), make([]byte, 200)...),
			"no PE signature"},
	} {
		err := checkNativeExecutable("meshcore-simple_repeater", tc.body)
		if err == nil {
			t.Errorf("%s was accepted as a program", tc.what)
			continue
		}
		if !strings.Contains(err.Error(), tc.says) {
			t.Errorf("%s says %q, want it to mention %q", tc.what, err, tc.says)
		}
	}
}

// The message has to carry what arrived, because "not a program" on its own
// sends the reader to the firmware rather than to the download.
func TestTheMessageQuotesWhatArrivedInstead(t *testing.T) {
	err := checkNativeExecutable("meshcore-simple_repeater", []byte(
		"<html><body>403 Forbidden. Rate limit exceeded.</body></html>"))
	if err == nil {
		t.Fatal("an error page was accepted")
	}
	if !strings.Contains(err.Error(), "<html>") {
		t.Errorf("the message does not show what arrived: %v", err)
	}
	if !strings.Contains(err.Error(), "meshcore-simple_repeater") {
		t.Errorf("the message does not name the file: %v", err)
	}
}

// A real build for this machine passes, so the guard does not reject the thing
// it exists to protect.
func TestThisPlatformsOwnHeaderIsAccepted(t *testing.T) {
	body := make([]byte, 64)
	switch runtime.GOOS {
	case "linux":
		copy(body, "\x7fELF")
		switch runtime.GOARCH {
		case "amd64":
			binary.LittleEndian.PutUint16(body[18:20], 0x3E)
		case "arm64":
			binary.LittleEndian.PutUint16(body[18:20], 0xB7)
		default:
			t.Skip("nothing is published for " + runtime.GOARCH)
		}
	case "darwin":
		binary.LittleEndian.PutUint32(body[:4], 0xFEEDFACF)
		switch runtime.GOARCH {
		case "arm64":
			binary.LittleEndian.PutUint32(body[4:8], 0x0100000C)
		case "amd64":
			binary.LittleEndian.PutUint32(body[4:8], 0x01000007)
		default:
			t.Skip("nothing is published for " + runtime.GOARCH)
		}
	case "windows":
		copy(body, "MZ")
		binary.LittleEndian.PutUint32(body[0x3C:0x40], 0x40)
		copy(body[0x40:], "PE\x00\x00")
		switch runtime.GOARCH {
		case "amd64":
			binary.LittleEndian.PutUint16(body[0x44:0x46], 0x8664)
		case "arm64":
			binary.LittleEndian.PutUint16(body[0x44:0x46], 0xAA64)
		default:
			t.Skip("nothing is published for " + runtime.GOARCH)
		}
	default:
		t.Skip("no native builds for " + runtime.GOOS)
	}
	if err := checkNativeExecutable("meshcore-simple_repeater", body); err != nil {
		t.Errorf("a build for this machine was refused: %v", err)
	}
}

// And one for another machine is refused by name, which is the failure the
// toolchain fetch's own comment describes: a right-sized download for the
// wrong platform.
func TestABuildForAnotherMachineSaysWhichOne(t *testing.T) {
	if runtime.GOOS == "darwin" {
		t.Skip("this fixture is the Mach-O one")
	}
	body := make([]byte, 64)
	binary.LittleEndian.PutUint32(body[:4], 0xFEEDFACF)
	binary.LittleEndian.PutUint32(body[4:8], 0x0100000C)
	err := checkNativeExecutable("meshcore-simple_repeater", body)
	if err == nil {
		t.Fatal("a darwin/arm64 build was accepted elsewhere")
	}
	if !strings.Contains(err.Error(), "darwin/arm64") {
		t.Errorf("the message does not name what it got: %v", err)
	}
}
