"""Fill the FICR region with plausible nRF52840 factory values.

Renode's monitor `sysbus WriteDoubleWord` calls, generated rather than
hand-listed so the reasoning stays visible.

Values are chosen to be *plausible and non-zero*, not to impersonate a specific
die. Zero is what breaks the firmware; a realistic constant is what fixes it.
"""

FICR = 0x10000000

REGS = {
    # Device identity — must be non-zero, and is what the firmware derives its
    # BLE address and node identity from.
    0x060: 0xDEADBEEF,  # DEVICEID[0]
    0x064: 0xCAFEF00D,  # DEVICEID[1]

    # Encryption root / identity root: 4 words each, factory random.
    **{0x080 + 4 * i: 0x11111111 * (i + 1) for i in range(4)},   # ER[0..3]
    **{0x090 + 4 * i: 0x22222222 * (i + 1) for i in range(4)},   # IR[0..3]

    0x0A0: 0x00000000,  # DEVICEADDRTYPE — public
    0x0A4: 0xA1B2C3D4,  # DEVICEADDR[0]
    0x0A8: 0x0000E5F6,  # DEVICEADDR[1]

    # INFO block. PART is the one a driver is most likely to branch on.
    0x100: 0x00052840,  # INFO.PART   — nRF52840
    0x104: 0x41414141,  # INFO.VARIANT — "AAAA"
    0x108: 0x00002004,  # INFO.PACKAGE — aQFN73
    0x10C: 0x00000040,  # INFO.RAM   — 256 kB
    0x110: 0x00000100,  # INFO.FLASH — 1 MB

    # The two the MeshCore boot path actually read, observed in the Renode log
    # before the fault. Left non-zero deliberately: zero is what broke it.
    0x130: 0x00000001,
    0x134: 0x00000001,
}


def main():
    for off, val in sorted(REGS.items()):
        print(f"sysbus WriteDoubleWord 0x{FICR + off:08X} 0x{val:08X}")


if __name__ == "__main__":
    main()
