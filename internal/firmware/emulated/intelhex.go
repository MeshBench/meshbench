package emulated

import (
	"bufio"
	"encoding/hex"
	"fmt"
	"os"
	"sort"
	"strings"
)

// Intel HEX record types. Only these four appear in a Nordic SoftDevice, and a
// fifth would mean the file is not what we think it is - so an unknown one is
// an error rather than something to skip.
const (
	hexData          = 0x00
	hexEOF           = 0x01
	hexExtendedSeg   = 0x02
	hexExtendedLin   = 0x04
	hexStartLinAddr  = 0x05
	hexStartSegAddr  = 0x03
	hexMinRecordLen  = 11 // ":" + len + addr + type + checksum
	hexAddressStride = 16
)

// ReadIntelHex reads an Intel HEX file and returns its contiguous regions.
//
// The SoftDevice arrives from Nordic as a .hex, and what the emulator loads is
// flat bytes at an address, so somebody has to do this. It is here rather than
// beside the downloader because this package is the one below both the engine
// and the interface, and both need the answer.
//
// Gaps are filled with 0xFF within a region, which is what erased flash reads
// as. Filling with zero is not a detail: the MBR decides whether things exist
// by testing words against 0xFFFFFFFF, and zeroed gaps answer "present, at
// address zero".
func ReadIntelHex(path string) ([]Region, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("firmware: reading the SoftDevice: %w", err)
	}
	defer func() { _ = f.Close() }()

	bytesAt := map[uint32]byte{}
	var upper uint32
	sc := bufio.NewScanner(f)
	for line := 1; sc.Scan(); line++ {
		rec := strings.TrimSpace(sc.Text())
		if rec == "" {
			continue
		}
		if !strings.HasPrefix(rec, ":") || len(rec) < hexMinRecordLen {
			return nil, fmt.Errorf("firmware: %s:%d is not an Intel HEX record", path, line)
		}
		raw, err := hex.DecodeString(rec[1:])
		if err != nil {
			return nil, fmt.Errorf("firmware: %s:%d: %w", path, line, err)
		}
		n, offset, typ := int(raw[0]), uint32(raw[1])<<8|uint32(raw[2]), raw[3]
		if len(raw) < 5+n {
			return nil, fmt.Errorf("firmware: %s:%d claims %d bytes and carries fewer", path, line, n)
		}
		data := raw[4 : 4+n]
		switch typ {
		case hexData:
			for i, b := range data {
				bytesAt[upper+offset+uint32(i)] = b
			}
		case hexExtendedLin:
			upper = (uint32(data[0])<<8 | uint32(data[1])) << 16
		case hexExtendedSeg:
			upper = (uint32(data[0])<<8 | uint32(data[1])) << 4
		case hexEOF, hexStartLinAddr, hexStartSegAddr:
			// Nothing to place: the start-address records name an entry point,
			// and the boot chain here takes its entry from the vector table.
		default:
			return nil, fmt.Errorf("firmware: %s:%d has record type 0x%02X, which we do not read",
				path, line, typ)
		}
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("firmware: reading the SoftDevice: %w", err)
	}
	if len(bytesAt) == 0 {
		return nil, fmt.Errorf("firmware: %s carries no data records", path)
	}
	return regionsFrom(bytesAt), nil
}

// regionsFrom joins scattered bytes into as few runs as it can.
//
// A run is broken only by a gap wider than one flash line: the SoftDevice's hex
// is nearly contiguous, and splitting on every missing byte would turn one
// region into thousands of one-byte loads.
func regionsFrom(bytesAt map[uint32]byte) []Region {
	addrs := make([]uint32, 0, len(bytesAt))
	for a := range bytesAt {
		addrs = append(addrs, a)
	}
	sort.Slice(addrs, func(i, j int) bool { return addrs[i] < addrs[j] })

	var out []Region
	for _, a := range addrs {
		if last := len(out) - 1; last >= 0 {
			end := out[last].Addr + uint32(len(out[last].Data))
			if a >= end && a-end <= hexAddressStride {
				for fill := end; fill < a; fill++ {
					out[last].Data = append(out[last].Data, 0xFF)
				}
				out[last].Data = append(out[last].Data, bytesAt[a])
				continue
			}
		}
		out = append(out, Region{Addr: a, Data: []byte{bytesAt[a]}})
	}
	return out
}
