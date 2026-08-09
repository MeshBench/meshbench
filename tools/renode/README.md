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

## Current state — the SoftDevice was the real blocker

Two hypotheses tested and disproved, one confirmed, all by measuring rather than
reasoning:

| Change | Instructions | Outcome |
|---|---|---|
| baseline | 233,455 | abort at `0xA5A5A5A4` |
| + FICR | **233,455** | identical — FICR was never the cause |
| + PWM0–3 | **233,455** | identical — peripherals were never the cause |
| + SoftDevice s140 6.1.1 | 1.4 billion | **no abort** |

Identical instruction counts are strong evidence: the firmware followed exactly
the same path, so those peripherals never influenced it. My first two diagnoses
were wrong and the counts are what proved it.

### The version pairing is determined by the app's base address

The application makes **119 SVC calls** into the SoftDevice. Without a matching
one it executes the stack-fill pattern, which is what `0xA5A5A5A4` was.

| SoftDevice | ends at | pairs with an app at |
|---|---|---|
| s140 6.1.1 | `0x025DE8` | **`0x026000`** |
| s140 7.2.0 | `0x026634` | `0x027000` |
| s140 7.3.0 | `0x026498` | `0x027000` |

The RAK4631 repeater `.uf2` is based at `0x026000`, so it needs **v6.1.1**.
Loading v7 would overlap the application. `softdevice.py` prints the pairing.

### Where it stops now

`PC = 0xa80`, inside the SoftDevice's early init, with the instruction count
climbing steadily. **That is a busy-wait, not a running mesh** — the large count
is a spin loop, not progress.

The SoftDevice is almost certainly polling for a hardware event Renode does not
raise: `CLOCK.EVENTS_HFCLKSTARTED` is the usual suspect, since the SoftDevice
starts the high-frequency crystal before doing anything else.

**Next step:** disassemble around `0x0a80` to identify exactly which register is
being polled, then model that event. Not another guess — the last two cost a
cycle each.

## What is missing, in order

1. **The event the SoftDevice is waiting on** at `0x0a80` — likely
   `CLOCK.EVENTS_HFCLKSTARTED`. Small once identified.
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
