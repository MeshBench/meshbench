# ESP32 emulation

Renode has no ESP32 platform, so the ESP32 half of the emulated backend uses
**Espressif's QEMU fork**, which ships prebuilt binaries for `esp32` and
`esp32s3`.

```bash
tools/esp32/build-and-run.sh Heltec_v2_repeater esp32   0x1000 40m
tools/esp32/build-and-run.sh Heltec_v3_repeater esp32s3 0x0    80m
```

No sudo needed: download the release tarball, and fetch `libslirp` from an Arch
mirror into a local directory pointed at by `LD_LIBRARY_PATH`.

## Status

Real, unmodified MeshCore firmware boots:

```
ets Jul 29 2019 12:21:46
rst:0x1 (POWERON_RESET),boot:0x12 (SPI_FAST_FLASH_BOOT)
mode:DIO, clock div:2
load:0x3fff0030,len:1184
entry 0x400805e4
```

The ROM bootloader runs, loads the app, and ESP-IDF starts. Then the **task
watchdog fires** — `TG0WDT_SYS_RESET` / `TG1WDT_SYS_RESET`, seven boot attempts
in 120 s.

## Why, and it is the same wall as nRF52

`examples/simple_repeater/main.cpp:63` calls `radio_init()`, which drives an
**SX1262 over SPI**. QEMU has no SX1262, so RadioLib waits on a chip that never
answers and the watchdog resets the board.

Renode hits exactly the same wall on nRF52. **A modelled SX1262 is the single
remaining blocker for the emulated backend on both architectures** — not a
per-chip problem, one peripheral.

That is worth knowing because it means the peripheral model is written once and
unblocks both.

## Two traps already paid for

- **Flash size must match the image header.** Padding to 4 MB when the header
  says 8 MB asserts inside `do_core_init` with a message that names the sizes —
  clear once you read it, baffling if you assume the firmware is at fault.
- **QEMU accepts only 2/4/8/16 MB images.** A merged image is not one of those
  sizes; pad with `0xFF`, the erased-flash value.
