#!/usr/bin/env python3
"""Build a flash image for Renode from a SoftDevice and a MeshCore .uf2.

The point of this script is the fill byte.

Renode's MappedMemory is zero-filled. Real unprogrammed flash reads 0xFF, and
the Nordic MBR checks for exactly that pattern to decide whether a bootloader
and an MBR parameter page exist. Load the images individually and the gaps
between them read as zeros, the MBR believes in a bootloader at address 0, and
the firmware never reaches its application — with no fault, no message and no
output. Building one pre-filled image and loading that instead is the whole fix.

    merge-nrf52.py s140_nrf52_6.1.1_softdevice.hex firmware.uf2 merged.bin
"""
import struct
import subprocess
import sys
import tempfile

FLASH_SIZE = 0x100000
ERASED = 0xFF


def uf2_chunks(path):
    """Address -> payload for each valid UF2 block."""
    data = open(path, "rb").read()
    out = {}
    for i in range(0, len(data), 512):
        b = data[i:i + 512]
        if len(b) < 32:
            continue
        magic0, magic1, _flags, addr, payload, *_ = struct.unpack("<8I", b[:32])
        if magic0 != 0x0A324655 or magic1 != 0x9E5D5157:
            continue
        out[addr] = b[32:32 + payload]
    return out


def main(sd_hex, uf2, out):
    img = bytearray(bytes([ERASED]) * FLASH_SIZE)

    # --gap-fill matters here too: the SoftDevice hex has a hole between the
    # MBR and the SoftDevice proper, and that hole is flash the MBR reads.
    with tempfile.NamedTemporaryFile(suffix=".bin") as sd_bin:
        subprocess.run(["arm-none-eabi-objcopy", "-I", "ihex", "-O", "binary",
                        "--gap-fill", hex(ERASED), sd_hex, sd_bin.name], check=True)
        sd = open(sd_bin.name, "rb").read()
    img[0:len(sd)] = sd

    chunks = uf2_chunks(uf2)
    if not chunks:
        sys.exit(f"{uf2}: no UF2 blocks")
    for addr, payload in chunks.items():
        img[addr:addr + len(payload)] = payload

    open(out, "wb").write(img)
    base = min(chunks)
    print(f"softdevice {len(sd)} bytes, application {sum(map(len, chunks.values()))} "
          f"bytes at 0x{base:X} -> {out}")


if __name__ == "__main__":
    if len(sys.argv) != 4:
        sys.exit(__doc__)
    main(*sys.argv[1:])
