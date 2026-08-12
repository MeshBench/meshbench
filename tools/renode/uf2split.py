#!/usr/bin/env python3
"""Split a UF2 into one flat file per contiguous region, and print the map.

A bootloader package is not one image. The RAK4631 OTAFIX carries four separate
regions - MBR, bootloader, its settings page, and UICR - and loading them as a
single blob at address zero puts the bootloader and UICR in the wrong place,
which looks exactly like a bootloader that does not work.
"""
import struct
import sys

BLOCK = 512
MAGIC0 = 0x0A324655
MAGIC1 = 0x9E5D5157


def main(path, prefix):
    data = open(path, "rb").read()
    regions = []          # [start, bytearray]
    for off in range(0, len(data) - BLOCK + 1, BLOCK):
        b = data[off:off + BLOCK]
        m0, m1, flags, addr, payload, blkno, numblk, famid = struct.unpack("<8I", b[:32])
        if m0 != MAGIC0 or m1 != MAGIC1:
            continue
        chunk = b[32:32 + payload]
        if regions and addr == regions[-1][0] + len(regions[-1][1]):
            regions[-1][1] += chunk
        else:
            regions.append([addr, bytearray(chunk)])

    for start, buf in regions:
        name = f"{prefix}_{start:08x}.bin"
        open(name, "wb").write(buf)
        print(f"  0x{start:08x}  {len(buf):6d} bytes  -> {name}")


if __name__ == "__main__":
    main(sys.argv[1], sys.argv[2])
