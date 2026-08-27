package emulated

import (
	"fmt"
	"hash/fnv"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// An nRF52 board boots MBR, then SoftDevice, then the application, and the
// published images carry only the last of those. The rest of this file is what
// stands in for a flashed board.
const (
	nrf52FlashBytes = 1024 * 1024
	nrf52UICRBase   = 0x10001000
	nrf52UICRBytes  = 0x1000
	nrf52FICRBase   = 0x10000000
)

// SoftDeviceHex finds the fetched SoftDevice for a version, or "" if it is not
// there.
//
// The directory layout is the downloader's, and the file is found by extension
// rather than by name: Nordic's own filename is the downloader's business, and
// spelling it in two places is how the two drift apart without either being
// wrong.
func SoftDeviceHex(dir, name, version string) string {
	found, err := filepath.Glob(filepath.Join(dir, name+"-"+version, "*.hex"))
	if err != nil || len(found) == 0 {
		return ""
	}
	sort.Strings(found)
	return found[0]
}

// softDeviceBase maps an application's base address to the SoftDevice it was
// linked above.
//
// The application's base *is* the version: an image at 0x26000 pairs with s140
// v6.1.1, which ends at 0x025DE8; one at 0x27000 pairs with v7.x, which ends at
// 0x026498 and would overlap a 0x26000 application. Pairing them wrongly is not
// a subtle failure - the application calls into the SoftDevice by supervisor
// call, and without a matching one it executes whatever fill pattern is there.
var softDeviceBase = map[uint32]string{
	0x26000: "6.1.1",
	0x27000: "7.x",
}

// renodeFlash returns the monitor commands that put this node's flash where the
// boot chain expects it, and the files those commands read.
//
// Erased flash is not tidiness. The MBR decides whether a bootloader and a
// parameter page exist by testing words against 0xFFFFFFFF, and Renode's memory
// starts at zero - so on a bare machine every such test answers "present, at
// address 0" and it dereferences a null pointer as a structure. That one
// difference is what made the published images look unrunnable.
func (e *EmulatedNode) renodeFlash() (string, error) {
	regions, err := e.flashRegions()
	if err != nil {
		return "", err
	}
	erased := filepath.Join(e.Dir, "erased_flash.bin")
	if err := writeFill(erased, nrf52FlashBytes); err != nil {
		return "", err
	}
	uicr := filepath.Join(e.Dir, "erased_uicr.bin")
	if err := writeFill(uicr, nrf52UICRBytes); err != nil {
		return "", err
	}

	var b strings.Builder
	fmt.Fprintf(&b, "sysbus LoadBinary @%s 0x0\n", erased)
	fmt.Fprintf(&b, "sysbus LoadBinary @%s 0x%X\n", uicr, nrf52UICRBase)
	for _, r := range regions.parts {
		path := r.path
		if path == "" {
			path = filepath.Join(e.Dir, fmt.Sprintf("flash_%08x.bin", r.addr))
			if err := os.WriteFile(path, r.data, 0o644); err != nil {
				return "", fmt.Errorf("firmware: writing a flash region: %w", err)
			}
		}
		fmt.Fprintf(&b, "sysbus LoadBinary @%s 0x%X\n", path, r.addr)
	}

	// SP and PC come from the vector table of whatever boots first, which is
	// the MBR at zero and not the application. Setting only VectorTableOffset
	// leaves SP uninitialised and the firmware faults within 45 instructions
	// pushing an exception frame onto a bogus stack.
	b.WriteString("cpu VectorTableOffset 0x0\n")
	b.WriteString("cpu SP `sysbus ReadDoubleWord 0x0`\n")
	b.WriteString("cpu PC `sysbus ReadDoubleWord 0x4`\n")
	b.WriteString(e.ficrWrites())
	return b.String(), nil
}

type flashRegion struct {
	addr uint32
	data []byte
	path string // set when the bytes are already a file worth loading directly
}

// flashPlan is everything that goes into flash, in the order it is laid down.
type flashPlan struct {
	parts []flashRegion
}

// flashRegions is where the image is taken apart, and where a missing
// SoftDevice is refused by name rather than booted into a fill pattern.
func (e *EmulatedNode) flashRegions() (flashPlan, error) {
	switch strings.ToLower(filepath.Ext(e.Image)) {
	case ".uf2":
	case ".bin":
		// A whole flash image, already at its addresses.
		return flashPlan{parts: []flashRegion{{addr: 0, path: e.Image}}}, nil
	default:
		// A release carries assets that are not flash images at all - a DFU
		// .zip most of all. Loaded at zero one of those is not firmware, and
		// the board that results looks broken rather than misfed.
		return flashPlan{}, fmt.Errorf(
			"firmware: %s is not a flash image (.uf2 or .bin)", e.Image)
	}
	regions, err := SplitUF2(e.Image)
	if err != nil {
		return flashPlan{}, err
	}
	plan := flashPlan{parts: make([]flashRegion, 0, len(regions))}
	for _, r := range regions {
		plan.parts = append(plan.parts, flashRegion{addr: r.Addr, data: r.Data})
	}
	base := plan.parts[0].addr
	if base == 0 {
		return plan, nil
	}
	want, known := softDeviceBase[base]
	if !known {
		return flashPlan{}, fmt.Errorf(
			"firmware: %s starts at 0x%X, which matches no SoftDevice we know", e.Image, base)
	}
	hexPath := ""
	if e.SoftDeviceDir != "" {
		hexPath = SoftDeviceHex(e.SoftDeviceDir, "s140", want)
	}
	if hexPath == "" {
		return flashPlan{}, fmt.Errorf(
			"firmware: %s is linked above SoftDevice s140 v%s, which has not been fetched - "+
				"download it in Resources first", e.Image, want)
	}
	sd, err := ReadIntelHex(hexPath)
	if err != nil {
		return flashPlan{}, err
	}
	// Underneath the application, and first, so the MBR's vector table is what
	// the CPU takes its stack pointer and entry from.
	under := make([]flashRegion, 0, len(sd)+len(plan.parts))
	for _, r := range sd {
		under = append(under, flashRegion{addr: r.Addr, data: r.Data})
	}
	plan.parts = append(under, plan.parts...)
	return plan, nil
}

// ficrWrites fills the factory information registers.
//
// Plausible and non-zero rather than a specific die: zero is what breaks the
// firmware, and a realistic constant is what fixes it. The device ID is derived
// from the node's name because the firmware derives its identity from it - one
// constant here and every emulated node in a network is the same node.
func (e *EmulatedNode) ficrWrites() string {
	h := fnv.New64a()
	_, _ = h.Write([]byte(e.NodeName))
	id := h.Sum64()

	regs := map[uint32]uint32{
		0x060: uint32(id),
		0x064: uint32(id >> 32),
		0x0A0: 0x00000000, // DEVICEADDRTYPE - public
		0x0A4: uint32(id ^ 0xA1B2C3D4),
		0x0A8: uint32(id>>32) & 0xFFFF,
		0x100: 0x00052840, // INFO.PART - nRF52840
		0x104: 0x41414141, // INFO.VARIANT - "AAAA"
		0x108: 0x00002004, // INFO.PACKAGE - aQFN73
		0x10C: 0x00000040, // INFO.RAM - 256 kB
		0x110: 0x00000100, // INFO.FLASH - 1 MB
		// The two the MeshCore boot path reads. Non-zero deliberately.
		0x130: 0x00000001,
		0x134: 0x00000001,
	}
	for i := uint32(0); i < 4; i++ {
		regs[0x080+4*i] = 0x11111111 * (i + 1) // ER[0..3]
		regs[0x090+4*i] = 0x22222222 * (i + 1) // IR[0..3]
	}

	offs := make([]uint32, 0, len(regs))
	for off := range regs {
		offs = append(offs, off)
	}
	sortUint32(offs)

	var b strings.Builder
	for _, off := range offs {
		fmt.Fprintf(&b, "sysbus WriteDoubleWord 0x%08X 0x%08X\n", nrf52FICRBase+off, regs[off])
	}
	return b.String()
}

func sortUint32(x []uint32) {
	for i := 1; i < len(x); i++ {
		for j := i; j > 0 && x[j] < x[j-1]; j-- {
			x[j], x[j-1] = x[j-1], x[j]
		}
	}
}

// writeFill lays down n bytes of 0xFF, which is what erased flash reads as.
func writeFill(path string, n int) error {
	buf := make([]byte, n)
	for i := range buf {
		buf[i] = 0xFF
	}
	if err := os.WriteFile(path, buf, 0o644); err != nil {
		return fmt.Errorf("firmware: writing erased flash: %w", err)
	}
	return nil
}

// SoftDeviceDir is where the downloader caches SoftDevices.
//
// Off the cache root rather than the firmware directory, because that is where
// the thing that fetches them puts them - and a reader that invents its own
// path finds an empty directory and reports a download that did happen as one
// that did not.
func SoftDeviceDir() string {
	dir, err := os.UserCacheDir()
	if err != nil {
		return filepath.Join("meshbench", "softdevice")
	}
	return filepath.Join(dir, "meshbench", "softdevice")
}
