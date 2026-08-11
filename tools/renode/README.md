# Renode emulated backend

Boots MeshCore's **published binaries** — the exact bytes people flash — as the
cross-check on the native host build (ADR-0010).

```bash
curl -sLo /tmp/rak.uf2 https://github.com/meshcore-dev/MeshCore/releases/download/repeater-v1.17.0/RAK_4631_repeater-v1.17.0-727fc05.uf2
tools/renode/uf2tobin.py /tmp/rak.uf2 /tmp/rak.bin
renode tools/renode/rak4631.resc
```

## Status

The published RAK4631 repeater binary now boots the whole chain:

    MBR -> SoftDevice s140 6.1.1 -> MeshCore application

The SoftDevice initialises fully and enables its own interrupts. The application
starts, the low-frequency clock runs from the crystal, and FreeRTOS ticks at
1024 Hz off RTC1. It then sits in its idle loop with no task ready to run.

    713d0: wfe
    713d2: ldr.w r0, [r3, #0x100]   ; NVIC ISPR0
    713da: orrs  r1, r0
    713dc: beq.n 0x713d0

That is where it is now. It is not a crash and not a stall in the SoftDevice —
the instruction count is identical across a twenty and a forty second run
(233,634 both times), so the CPU is halted in WFE rather than spinning, and
forcing RTC1's NVIC enable makes it service a tick (83 instructions) and go
straight back to idle. Something in the application's startup is blocked on an
event nobody raises.

Reproduce with `tools/renode/rak4631-app.resc`.

### What the earlier conclusion got wrong

This file used to say the published binaries were a dead end: a proprietary
SoftDevice, open-ended effort, no guaranteed end. That was wrong, and the
measurements behind it were taken on a machine missing things a real board has.

**Erased flash was most of it.** Real nRF52 flash erases to 0xFF; Renode's
memory starts at 0x00, and `softdevice.py` filled gaps in the hex with zero as
well. The MBR decides whether a bootloader and a parameter page exist by testing
words against 0xFFFFFFFF — so every such test answered "present, at address 0",
and it dereferenced a null pointer as a structure and spun there for 2.4 billion
instructions at 0x438. `softdevice.py` now fills with 0xFF.

**UICR was the other half.** The MBR reads `UICR.NRFFW[0]` to find the
bootloader. `uicr.repl` existed and had never been loaded, and nothing had the
real contents to put in it. MeshCore's own OTAFIX bootloader package carries
them: MBR at 0, bootloader at 0xF4000, its settings page, and UICR. Split it
with `uf2split.py`.

**Then two missing peripherals, each a handful of registers.** TEMP is not
modelled at all, and the SoftDevice starts a temperature measurement during
initialisation and waits on `EVENTS_DATARDY` — 28,000 instructions and then
silence. Renode's `NRF_CLOCK` has no calibration timer, and the SoftDevice stops
that timer and waits for `EVENTS_CTSTOPPED`. Both are in `peripherals/`.

Progress, measured at each step, because two earlier diagnoses on this file were
wrong and identical instruction counts are what proved it:

| PC | instructions | waiting for |
|---|---|---|
| `0x438` | 2.4 billion | nothing — null pointer from zeroed flash |
| `0x1604c` | 28 thousand | `EVENTS_DATARDY` (TEMP) |
| `0xbc5c` | 43 million | `EVENTS_CTSTOPPED` (calibration timer) |
| `0xf54c2` | 111 million | USB, in the bootloader's DFU loop |
| `0x713d2` | 233 thousand | an application event, in the idle loop |

So it is a chain of partially modelled peripherals rather than a proprietary
binary refusing to run. Each step so far has been small. Whether the chain is
short is not something we can claim — only that the reason for stopping before
was not the reason we gave.

### The bootloader is optional, and better left out

The OTAFIX bootloader boots, and then waits in DFU for USB that Renode does not
model, feeding the watchdog for ever. A board flashed over SWD goes MBR to
SoftDevice to application, which is what a simulated node wants: no USB, no DFU
window, no double-tap. Leave UICR erased and do not load the bootloader regions.

### The SoftDevice still cannot be shipped

Nordic licenses it and it cannot be redistributed, so anyone running this
supplies their own copy. That is a reason to keep the SoftDevice-free ARM build
(`tools/armfw/`) as the path we can actually give people, whatever happens here.

## The trap that cost the most time

Setting only `VectorTableOffset` leaves SP uninitialised, and the firmware
faults within **45 instructions** pushing an exception frame to a bogus stack.
SP and PC must be read from the vector table of whatever boots first — the MBR's
at 0, not the application's. `uf2tobin.py` prints both.
