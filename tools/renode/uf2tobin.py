#!/usr/bin/env python3
"""Convert a MeshCore .uf2 release to a flat binary for Renode.

UF2 is 512-byte blocks carrying up to 256 bytes of payload each, with the target
address in the header. Blocks are not guaranteed contiguous, so the image is
assembled by address rather than by concatenation.
"""
import struct, sys

def main(src, dst):
    data = open(src, 'rb').read()
    chunks = {}
    for i in range(len(data) // 512):
        b = data[i * 512:(i + 1) * 512]
        magic0, _, _, addr, size, _, _, _ = struct.unpack('<8I', b[:32])
        if magic0 != 0x0A324655:      # "UF2\n"
            continue
        chunks[addr] = b[32:32 + size]
    if not chunks:
        sys.exit(f"{src}: no valid UF2 blocks")
    base, last = min(chunks), max(chunks)
    img = bytearray(last + len(chunks[last]) - base)
    for a, p in chunks.items():
        img[a - base:a - base + len(p)] = p
    open(dst, 'wb').write(img)
    sp, pc = struct.unpack('<II', img[:8])
    print(f"{dst}: {len(img)} bytes at 0x{base:08x}   vector table SP=0x{sp:08x} PC=0x{pc:08x}")

if __name__ == '__main__':
    if len(sys.argv) != 3:
        sys.exit(f"usage: {sys.argv[0]} firmware.uf2 out.bin")
    main(sys.argv[1], sys.argv[2])
