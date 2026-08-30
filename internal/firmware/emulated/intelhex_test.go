package emulated

import (
	"bytes"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// hexRecord builds one well-formed Intel HEX line, checksum included, so a
// test can say what a record means rather than what its bytes are.
func hexRecord(n int, addr uint16, typ byte, data []byte) string {
	raw := []byte{byte(n), byte(addr >> 8), byte(addr), typ}
	raw = append(raw, data...)
	var sum byte
	for _, b := range raw {
		sum += b
	}
	raw = append(raw, byte(-sum))
	return ":" + strings.ToUpper(hex.EncodeToString(raw))
}

func writeHex(t *testing.T, lines ...string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "softdevice.hex")
	body := strings.Join(lines, "\n") + "\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// The SoftDevice arrives from Nordic as a .hex; a reader that cannot get the
// plain case right cannot be trusted with the malformed one.
func TestReadIntelHexParsesDataRecords(t *testing.T) {
	path := writeHex(t,
		hexRecord(4, 0x0000, hexData, []byte{0xDE, 0xAD, 0xBE, 0xEF}),
		hexRecord(0, 0x0000, hexEOF, nil),
	)
	regions, err := ReadIntelHex(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(regions) != 1 || regions[0].Addr != 0 || !bytes.Equal(regions[0].Data, []byte{0xDE, 0xAD, 0xBE, 0xEF}) {
		t.Errorf("unexpected regions: %+v", regions)
	}
}

// An extended-address record with fewer than the two bytes it declares used
// to index straight into an empty slice and take the process down with it -
// exactly the shape a truncated download leaves behind, and per issue #326
// that download was never checked against a digest before landing here.
func TestReadIntelHexRejectsTruncatedExtendedAddressRecords(t *testing.T) {
	for _, typ := range []byte{hexExtendedLin, hexExtendedSeg} {
		for n := 0; n <= 1; n++ {
			data := make([]byte, n)
			path := writeHex(t, hexRecord(n, 0x0000, typ, data))
			if _, err := ReadIntelHex(path); err == nil {
				t.Errorf("type 0x%02X with %d bytes was accepted", typ, n)
			}
		}
	}
}

// The byte count is trusted nowhere else in the file either: a record that
// claims more or fewer bytes than it carries is refused rather than read
// past its own end.
func TestReadIntelHexRejectsALengthMismatch(t *testing.T) {
	// A data record built for 4 bytes, then trimmed to look like it declared
	// only 2 - the checksum byte now sits where a data byte should be, so this
	// exercises the "carries fewer" shape without help from hexRecord.
	full := hexRecord(4, 0x0000, hexData, []byte{0xDE, 0xAD, 0xBE, 0xEF})
	short := full[:len(full)-4] // drop the last two data bytes and the checksum
	path := writeHex(t, short)
	if _, err := ReadIntelHex(path); err == nil {
		t.Error("a record shorter than its declared length was accepted")
	}
}

// A record whose checksum byte does not match its content is corruption, and
// per issue #326 corruption is exactly what an unverified download can leave
// behind. It must be refused rather than loaded into emulated flash.
func TestReadIntelHexRejectsABadChecksum(t *testing.T) {
	good := hexRecord(4, 0x0000, hexData, []byte{0xDE, 0xAD, 0xBE, 0xEF})
	// Flip the last hex digit of the checksum byte.
	bad := good[:len(good)-1] + flipHexDigit(good[len(good)-1])
	path := writeHex(t, bad)
	if _, err := ReadIntelHex(path); err == nil {
		t.Error("a record with a wrong checksum was accepted")
	}
}

func flipHexDigit(d byte) string {
	if d == '0' {
		return "1"
	}
	return "0"
}

// A file that never places a byte answers nothing, and nothing is not a
// SoftDevice.
func TestReadIntelHexRejectsAFileWithNoData(t *testing.T) {
	path := writeHex(t, hexRecord(0, 0x0000, hexEOF, nil))
	if _, err := ReadIntelHex(path); err == nil {
		t.Error("a file with no data records was accepted")
	}
}
