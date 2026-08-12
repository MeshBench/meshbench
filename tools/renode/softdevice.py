#!/usr/bin/env python3
"""Convert a Nordic SoftDevice .hex to a flat binary, and report its extent.

The extent matters more than the conversion: **the SoftDevice version is
determined by where the application starts.** A MeshCore .uf2 based at 0x26000
pairs with s140 v6.1.1 (which ends at 0x025DE8); one based at 0x27000 pairs with
v7.x (which ends at 0x026498 and would overlap a 0x26000 app).

Getting this wrong is not a subtle failure — the app makes 119 SVC calls into
the SoftDevice, and without a matching one it executes the stack-fill pattern.
"""
import struct
import sys


def load_hex(path):
    mem, ext = {}, 0
    for line in open(path):
        line = line.strip()
        if not line.startswith(':'):
            continue
        n = int(line[1:3], 16)
        off = int(line[3:7], 16)
        typ = int(line[7:9], 16)
        if typ == 4:
            ext = int(line[9:13], 16) << 16
        elif typ == 0:
            for i, b in enumerate(bytes.fromhex(line[9:9 + n * 2])):
                mem[ext + off + i] = b
    return mem


def main(src, dst):
    mem = load_hex(src)
    lo, hi = min(mem), max(mem)
    # Gaps are erased flash, which reads 0xFF. Filling them with zero is what
    # made this look like an unrunnable binary for a fortnight: the MBR decides
    # whether a bootloader or a parameter page exists by testing words against
    # 0xFFFFFFFF, so a zeroed gap at 0x0FF8 answers "present, at address 0" and
    # it dereferences a null pointer as a structure and spins there for ever.
    img = bytearray(b'\xff' * (hi - lo + 1))
    for a, b in mem.items():
        img[a - lo] = b
    open(dst, 'wb').write(img)
    sp, pc = struct.unpack('<II', img[:8])
    app = ((hi + 1 + 0xFFF) // 0x1000) * 0x1000
    print(f"{dst}: 0x{lo:06X}..0x{hi:06X} ({len(img)} bytes)")
    print(f"  vector table  SP=0x{sp:08X}  PC=0x{pc:08X}")
    print(f"  an application paired with this SoftDevice starts at 0x{app:06X}")


if __name__ == '__main__':
    if len(sys.argv) != 3:
        sys.exit(f"usage: {sys.argv[0]} softdevice.hex out.bin")
    main(sys.argv[1], sys.argv[2])
