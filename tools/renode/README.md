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

### Where it stops now, and why the SoftDevice is a dead end

`PC = 0xa80` with the instruction count climbing steadily. Decoding the
halfwords by hand (objdump on this box cannot target Thumb):

```
0a7c: bf20   WFE
0a7e: e7fd   B    .-2          <- idle loop
0a80: 4b06   LDR  r3, [pc,#24]
0a82: 4718   BX   r3           <- indirect dispatch
```

Raising `CLOCK.EVENTS_HFCLKSTARTED` and `EVENTS_LFCLKSTARTED` changes nothing;
Renode does model `NRF_CLOCK`, so that was not it either. Beyond this point we
are reverse-engineering a **proprietary Nordic binary**, and Renode does not
claim SoftDevice-level fidelity. That is an open-ended effort with no
guaranteed end.

### The achievable path: build without the SoftDevice

`examples/simple_repeater/main.cpp` contains **no BLE code at all** — the only
conditionals are `ETHERNET_ENABLED` and `ENABLE_ADVERT_ON_BOOT`. Bluetooth
belongs to the *companion* firmware, not the repeater.

The SoftDevice is pulled in by the Adafruit nRF52 Arduino core's linker layout
(`boards/nrf52840_s140_v*.ld` place the app above it), not by anything the
repeater calls. So the published `.uf2` carries 119 SVC call sites it never
needs.

**Therefore: build a repeater image linked at `0x0` with no SoftDevice.** It has
no SVC calls, boots directly, and is still the real MeshCore mesh stack compiled
for real ARM — which is what the emulated backend exists to validate. This is
the same "compile it ourselves" move ADR-0002 already makes for the host,
retargeted at ARM.

That is a firmware build task (MSIM-13), not a Renode task, and it is why the
two tickets should be done in that order.

## What is missing, in order

1. **A SoftDevice-free repeater image** (see above). Unblocks everything.
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
