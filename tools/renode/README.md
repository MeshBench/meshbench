# Renode emulated backend

Boots MeshCore's **published binaries** — the exact bytes people flash — as the
cross-check on the native host build (ADR-0010).

```bash
curl -sLo /tmp/rak.uf2 https://github.com/meshcore-dev/MeshCore/releases/download/repeater-v1.17.0/RAK_4631_repeater-v1.17.0-727fc05.uf2
tools/renode/uf2tobin.py /tmp/rak.uf2 /tmp/rak.bin
renode tools/renode/rak4631.resc
```

## Status

**Boots and runs real initialisation.** Measured on Renode 1.16.0:

| | |
|---|---|
| Instructions executed | **233,455** |
| Reaches | FICR reads, peripheral configuration at `0x4001C564`–`0x4002D564` |
| Fails at | `0xA5A5A5A4` — the stack-fill pattern |

## What is missing, in order

1. **FICR** (Factory Information Configuration Registers, `0x10000000`). The
   firmware reads device info from `0x10000130`/`0x10000134`, gets zeros because
   Renode's `nrf52840.repl` does not model FICR, and later dereferences something
   derived from it — landing on `0xA5A5A5A5`, the uninitialised-memory fill.
   Small and well-defined: a handful of registers with plausible values.
2. **SX1262 over SPI.** Renode ships no SX126x peripheral. This is the large
   piece, and it is large because `CustomSX1262.h` drops to `readRegister` /
   `writeRegister` / `getIrqFlags`, so the model must satisfy *RadioLib's own
   unmodified driver* — command opcodes, status bytes, IRQ semantics, buffer
   addressing, TX/RX-done timing.

## The trap that cost the most time

Setting only `VectorTableOffset` leaves SP uninitialised, and the firmware
faults within **45 instructions** pushing an exception frame to a bogus stack.
SP and PC must be read from the application's own vector table — the first two
words of the image. `uf2tobin.py` prints both.
