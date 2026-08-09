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

## FICR: added, and it was not the cause

`ficr.repl` + `ficr.py` add the FICR region Renode's stock `nrf52840.repl`
omits — worth having regardless, since reads there previously returned zeros.

**But it did not fix the boot, and the experiment was clean:** instruction count
is *identical* at 0x38FEF (233,455) with and without FICR, and the abort is at
the same address. Identical counts mean the firmware follows exactly the same
path, so FICR values never influenced the trajectory. Hypothesis disproved
rather than assumed away.

The remaining suspects, from the pre-fault log, are writes to
`0x4001C564`, `0x40021564`, `0x40022564`, `0x4002D564` — the same `0x564`
offset across four unmodelled peripheral regions, which looks like the firmware
walking a peripheral table. `PC=0xA5A5A5A4` means it is *executing* the
stack-fill pattern, i.e. it returned through a corrupted frame or an unpopulated
function pointer.

Next step is an execution trace to find the last good PC, not another guess.

## What is missing, in order

1. **The four `0x…564` peripherals** above — identify them from the nRF52840
   memory map and model enough to satisfy the writes.
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
