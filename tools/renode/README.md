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

That is where it is now, and the reason is identified: **Renode does not
implement SEVONPEND**.

The firmware sets it. `SCR` at `0xE000ED10` reads `0x00000010`, bit 4. With
SEVONPEND set, an interrupt becoming pending wakes `WFE` *even when that
interrupt is disabled in the NVIC* — which is exactly what this loop relies on.
It sleeps, wakes on any pending interrupt, reads ISPR to see which, and handles
it without ever taking the interrupt. That is a normal low-power idiom and it is
why the interrupt is deliberately left disabled.

The machine state at the stall says the rest:

| | |
|---|---|
| `SCR` | `0x00000010` — SEVONPEND set |
| `ISPR0` | `0x00020000` — RTC1 pending |
| `ISER0` | `0x00000001` — RTC1 *not* enabled |
| instructions | identical across a 20 s and a 40 s run |

So the CPU is halted with the wake condition already true. Two things confirm
the diagnosis rather than merely fit it. Forcing RTC1's NVIC enable makes it
service one tick (83 instructions) and go straight back to sleep — because the
handler clears the pending bit, so the ISPR poll then finds nothing, which is
the wrong behaviour for this idiom. And `cpu WfeAndSevAsNop true` turns the same
loop into a busy poll and the instruction count jumps from 233 thousand to 127
million: the CPU runs, so it was the sleep and not the firmware.

`WfeAndSevAsNop` is not a fix, though, because it costs the thing that made
halting useful. When the CPU halts, Renode fast-forwards virtual time to the
next timer event and RTC1 reaches 15,495 ticks in twelve seconds of wall clock.
Busy-polling advances virtual time from instructions executed instead, so the
same twelve seconds gets 128 ticks, and the firmware's one-second tick costs
minutes. It progresses, unusably slowly.

**The fix is SEVONPEND in the CPU**: wake from `WFE` when an interrupt becomes
pending, regardless of its enable bit, and leave the pending bit set. That is a
change to Renode's CPU core rather than a peripheral, which is a different kind
of work from the four register blocks above — but it is bounded, specified by
ARM, and nothing to do with the SoftDevice being proprietary.

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

## The radio is on SPIM3, and that was the whole problem

0x4002F000 is NRF_SPIM3_BASE - the high-speed SPI controller the nRF52840 adds
and the one the RAK4631's radio is wired to. Renode's platform declares spi0..2
and stops there, so every transfer the firmware made went to an address with no
peripheral behind it. Reads returned zero, EVENTS_END never arrived, and it
polled 0x118 for ever.

It was briefly written up here as CryptoCell, because 0x4002F000 is easy to
misread as the crypto block. The register offsets settled it: 0x118 EVENTS_END,
0x304 INTENSET, 0x308 INTENCLR are a SPIM.

With spim3.repl the published RAK4631 firmware configures the chip: **3,320
bytes of SPI into the same VirtualSX1262 a native node uses.** That is real
radio initialisation - packet type, frequency, modulation parameters,
calibration - by the bytes people flash.

## What is missing now: the DIO1 interrupt

After those 3,320 bytes the firmware stops touching SPI, and stays silent for
420 seconds of its own time (RTC1 at 1024 Hz reads 430,213 ticks). It is not
polling the chip for interrupt flags; it is waiting on the DIO1 pin, and nothing
drives it.

The radio model has no way to say so. Its emulator protocol is four tags - chip
select, transfer, and read-busy - with no interrupt channel, because the QEMU
path did not need one. Closing this needs:

  - a read-IRQ tag in radioserver, answering the chip's IRQ line
  - the Renode peripheral polling it and driving a GPIO
  - that GPIO wired to P_LORA_DIO_1 for the board in question

That is the last piece for an nRF52 node, and it is small - but it spans both
repositories and is not something to half-finish.

## Where it stops now, and why a stub would be worse than a stall

After TEMP, the calibration timer, SAADC and TWIM, the published RAK4631 build
runs 661 million instructions and stops here:

    71cd0: ldr.w r2, [r3, #0x118]   ; r3 = 0x4002F000
    71cd4: cmp   r2, #0
    71cd6: beq.n 0x71cd0

Nothing is mapped at 0x4002F000 in Renode's nRF52840 platform, so the read
returns zero for ever. That address is CryptoCell, the hardware crypto engine
MeshCore's Ed25519 and AES use on this part.

**Do not stub it the way the others were stubbed.** TEMP, SAADC and TWIM can
answer plausibly because nothing downstream depends on the value being right - a
temperature, a battery reading, an absent sensor. Crypto is the opposite: a
model that raises "done" without doing the work hands the firmware a wrong
answer, and the firmware has no way to know. A node would come up with a corrupt
identity and sign packets nobody can verify, which looks like a mesh problem and
is not one. Either model CryptoCell properly or do not model it at all.

Two routes from here, and the second is the one we can ship:

1. Model CryptoCell. Large, and only worth it to run the published binaries -
   which still need a SoftDevice we cannot redistribute.
2. Build an nRF52 variant in meshcore-native, the way variants/host works:
   software crypto, real RadioLib over nRF52 SPI, no SoftDevice. Not the flashed
   bytes, but a real ARM build of the same source that can be handed to anyone,
   and the radio path to radioserver is already proven from ARM.

### The pattern worth remembering

Renode's nRF52840 models the legacy half of several peripherals and firmware
drives the EasyDMA half. spi2 declares easyDMA false, so SPIM transfers went
nowhere; NRF52840_I2C implements the legacy TWI registers, so TWIM polled
EVENTS_TXSTARTED for ever. Expect the next stall to be the same shape.
