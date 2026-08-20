package firmware

import (
	"encoding/binary"
	"fmt"
	"os"
)

// UF2 blocks are fixed at 512 bytes, of which at most 476 are payload. The two
// magic words are what distinguish a block from padding: a UF2 file may carry
// anything between blocks, so scanning is by magic rather than by offset.
const (
	uf2Block  = 512
	uf2Magic0 = 0x0A324655
	uf2Magic1 = 0x9E5D5157
)

// Region is one contiguous run of flash carried by a UF2, and where it goes.
//
// A UF2 is not one image. A bootloader package carries the MBR, the bootloader,
// its settings page and UICR as four separate runs, and loading the file as a
// single blob at zero puts three of them at the wrong address - which looks
// exactly like a bootloader that does not work rather than like a misread file.
type Region struct {
	Addr uint32
	Data []byte
}

// SplitUF2 reads a UF2 and returns its regions in file order.
//
// Adjacent blocks are joined, so a plain application image comes back as one
// region at its base address rather than as several thousand.
func SplitUF2(path string) ([]Region, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("firmware: reading the UF2: %w", err)
	}
	var regions []Region
	for off := 0; off+uf2Block <= len(data); off += uf2Block {
		b := data[off : off+uf2Block]
		if binary.LittleEndian.Uint32(b[0:]) != uf2Magic0 ||
			binary.LittleEndian.Uint32(b[4:]) != uf2Magic1 {
			continue
		}
		addr := binary.LittleEndian.Uint32(b[12:])
		n := binary.LittleEndian.Uint32(b[16:])
		if n > uf2Block-32 {
			return nil, fmt.Errorf("firmware: %s: a UF2 block claims %d payload bytes", path, n)
		}
		chunk := b[32 : 32+n]
		if last := len(regions) - 1; last >= 0 &&
			addr == regions[last].Addr+uint32(len(regions[last].Data)) {
			regions[last].Data = append(regions[last].Data, chunk...)
			continue
		}
		regions = append(regions, Region{Addr: addr, Data: append([]byte(nil), chunk...)})
	}
	if len(regions) == 0 {
		return nil, fmt.Errorf("firmware: %s carries no UF2 blocks", path)
	}
	return regions, nil
}
