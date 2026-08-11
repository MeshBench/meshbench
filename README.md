# MeshBench

An RF-accurate MeshCore network simulator.

It runs **real MeshCore firmware** — the actual C++ from `meshcore-dev/MeshCore`,
compiled for the host, driven through **its own radio driver over real
RadioLib** — against a **sample-accurate LoRa baseband channel** with real
noise.

The channel does not decide whether a packet arrives. It sums waveforms, applies
path loss over real terrain, adds thermal noise, and lets each receiver's
demodulator find out. Capture effect, partial collisions and sensitivity are
*emergent*, not rules someone wrote down.

## Where the real ends and the simulation begins

The claim "it runs real firmware" is worth being exact about, because the
interesting bugs live on the boundary.

| Layer | What runs |
|---|---|
| MeshCore application — repeater, companion, room server | **real**, unmodified |
| MeshCore mesh logic — `Mesh`, `Dispatcher`, forwarding policy, CLI, preferences | **real**, unmodified |
| MeshCore radio driver — `CustomSX1262`, `RadioLibWrapper` | **real**, unmodified |
| RadioLib 7.6.0 | **real**, vendored, unmodified |
| The SX1262 chip | **ours** — a virtual chip answering SPI |
| Arduino core, board, flash filesystem, RTC, sensors, RNG | **ours** — the platform |
| The air — modulation, path loss, noise, capture effect | **ours** — this is the simulator |

**Nothing in MeshCore is patched.** The build points at a MeshCore checkout and
compiles it as it stands. What is substituted is the platform beneath it, which
is hardware rather than firmware — and, at the bottom, the air, which is the
entire point of the exercise.

The line moved recently and it mattered. The radio driver used to be ours too: a
hand-written stand-in for the whole stack. That made every listen-before-talk
decision the simulator's rather than the firmware's, and two MeshCore versions
differing *only* in their radio driver produced bit-identical results across
twelve runs — which read as "the change does nothing" when it actually meant
"the changed code never ran". See `docs/virtual-sx1262.md`.

### What the boundary still costs

- **A virtual chip is a model of a chip.** Everything above the SPI pins is the
  software that runs on hardware. What answers those pins is our best
  understanding of an SX1262, so real errata and a real preamble detector's
  behaviour under interference remain invisible.
- **We compile as an nRF52.** Where MeshCore takes a platform-specific path, we
  take that one. A bug that only appears on ESP32 will not appear here — which
  is what the emulated backend below exists to close, at the cost of about a
  core per node.
- **The simulator is kinder than the air.** No multipath, no body loss, no
  oscillator error. Every result is a best case — which is what makes it usable:
  if it does not work here, it will not work outdoors.

## Two ways to run a node: native and emulated

Every node runs one of two backends. They run the *same* MeshCore, against the
*same* channel — what changes is how much of the hardware beneath it is real.

**Native** compiles MeshCore for this machine. Each node is an ordinary
operating-system process with its own storage, identity and clock. Everything
from the application down to RadioLib is the firmware's own code; below that is
our model.

```
  the stack a packet passes through, top to bottom       NATIVE

  +----------------------------------------------------+
  | MeshCore application                               |   real
  |  examples/simple_repeater, unmodified              |
  +----------------------------------------------------+
  | MeshCore radio wrapper                             |   real
  |  CustomSX1262 / CustomSX1262Wrapper                |
  +----------------------------------------------------+
  | RadioLib 7.6.0                                     |   real
  |  the actual SX126x driver, unmodified              |
  +====================================================+   the firmware believes
  : RadioLibHal                                        :   simulated
  :  SimHal: pins, SPI, timing                         :
  :....................................................:
  : SX1262 silicon                                     :   simulated
  :  VirtualSX1262: opcodes, registers, IRQs           :
  :....................................................:
  : The air                                            :   simulated
  :  MeshBench engine: path loss, terrain, noise       :
  '....................................................'
```

**Figure 1: where the real firmware ends and the model begins.** Above the line
is MeshCore's and RadioLib's own code, unmodified, making its own decisions.
Below it is a model: a virtual SX1262 answering SPI opcodes and raising
interrupts, a HAL supplying pins and simulated time, and a channel that sums
waveforms and lets the demodulator decide what survives.

**Emulated** runs the image people actually flash — the merged `.bin` from
[meshcore.io/flasher](https://meshcore.io/flasher), byte for byte — inside a
QEMU with an SX1262 attached to its SPI bus. Nothing is compiled. The boundary
moves down by four layers.

This works today for **ESP32 only**, and for one board. That matters, because
ESP32 is not the whole ecosystem: of MeshCore's 87 board variants, 40 are ESP32
family, **36 are nRF52840** — RAK4631, Heltec T114, LilyGo T-Echo, XIAO nRF52,
T1000-E, ThinkNode, Wio Tracker — and the rest are RP2040, STM32 and ESP32-C6.
Half the hardware people own is ARM. See *Architectures* below for where each
one stands.

```
  the same stack, running the image off the flasher     EMULATED

  +----------------------------------------------------+
  | MeshCore application                               |   real
  |  the published .bin, byte for byte                 |
  +----------------------------------------------------+
  | MeshCore radio wrapper                             |   real
  |  CustomSX1262 / CustomSX1262Wrapper                |
  +----------------------------------------------------+
  | RadioLib 7.6.0                                     |   real
  |  the actual SX126x driver, unmodified              |
  +----------------------------------------------------+
  | Arduino core, ESP-IDF, FreeRTOS, bootloader        |   real
  |  none of which the native build ever runs          |
  +----------------------------------------------------+
  | ESP32 machine code on an Xtensa LX6                |   real
  |  every instruction the chip would execute          |
  +----------------------------------------------------+
  | SPI2 peripheral, GPIO matrix, chip select          |   real
  |  QEMU: the wires, clocked a byte at a time         |
  +====================================================+   the boundary moved down
  : SX1262 silicon                                     :   simulated
  :  the same VirtualSX1262, over a socket             :
  :....................................................:
  : The air                                            :   simulated
  :  the same MeshBench engine                         :
  '....................................................'
```

**Figure 2: the same stack, with the hardware put back.** The bootloader,
ESP-IDF, FreeRTOS, the Arduino core and every Xtensa instruction are now real,
and the SPI peripheral clocks the chip select and the bytes exactly as the
silicon would. The model starts at the far side of the SPI wire rather than at
the driver. Below that line, both backends share the same virtual chip and the
same channel — which is what makes a native run and an emulated one comparable
at all.

### Setting one up today

Until this is packaged, an emulated node needs two binaries that are not on any
distribution: a QEMU carrying our SX1262 device, and the radio model itself.
Put them where the application looks and nothing else is needed — no environment
variables, no flags.

```bash
mkdir -p ~/.cache/meshcoresim/tools
ln -sf /path/to/qemu-system-xtensa ~/.cache/meshcoresim/tools/
cp /path/to/radioserver ~/.cache/meshcoresim/tools/
```

A symlink is right for QEMU: it finds its own data files by resolving its real
path, so a bare copy of the binary will not run. `radioserver` builds from
`meshcore-native` against `VirtualSX1262.cpp` with no other dependencies.

Then open the firmware library, download a board image, and set a node's role to
it. The library only offers boards with verified wiring, so anything it lists
will start.

MeshBench searches, in order: `MESHCORESIM_QEMU` and `MESHCORESIM_RADIO_SERVER`,
then beside its own binary, then that tools directory, then `PATH`. `PATH` is
last because a desktop application is not launched from a shell and inherits
nothing useful from one — which is why emulation used to work from a terminal
and fail from the desktop.

`docs/packaging-emulation.md` has the rest: what a release has to ship, what it
cannot ship and why, and what to tell people about the cost.

### Architectures

Emulation is per architecture *and* per board, and the honest position differs
by tier. A board joins the verified list when someone has watched its published
image come up, not when its MCU is supported.

| architecture | boards | what runs | state |
|---|---|---|---|
| ESP32 family (Xtensa) | 40 | the published `.bin`, byte for byte, under QEMU | working, one board verified |
| nRF52840 (Cortex-M4) | 36 | our own SoftDevice-free ARM build, under Renode | mesh stack runs; radio being wired |
| RP2040 (Cortex-M0+) | 4 | — | not started |
| STM32 (Cortex-M4) | 4 | — | not started |
| ESP32-C6 (RISC-V) | 3 | — | not started |

**The nRF52 published binaries do not run yet, and we have stopped trying.**
That is a judgement about cost, not a proof of impossibility — worth being exact
about, because the two are easy to confuse.

They are linked above a Nordic SoftDevice and make 119 SVC calls into it.
Without one they execute the stack-fill pattern and die at `0xA5A5A5A4`. Adding
FICR, then PWM0–3, left the instruction count at *exactly* 233,455 both times;
identical counts prove those peripherals were never on the path, and disproved
two of our own diagnoses.

With the real s140 6.1.1 supplied there is **no abort** — 1.4 billion
instructions, and then an idle loop at `PC = 0xa80`: `WFE`, branch to self, and
an indirect dispatch. That is a CPU waiting on an interrupt that never arrives,
which is a missing emulated event rather than a wall. Raising
`EVENTS_HFCLKSTARTED` and `EVENTS_LFCLKSTARTED` changed nothing and Renode does
model `NRF_CLOCK`, so that guess was wrong; the untested suspects are the
peripherals the SoftDevice owns — RTC0, the SWI/EGU software interrupts its
scheduler runs on, and POWER events — of which Renode models a subset.

Someone could pick that up. We did not, for two reasons. Past this point it is
reverse-engineering a proprietary binary that Renode does not claim to emulate
faithfully, with no way to bound the effort. And the SoftDevice is licensed by
Nordic and cannot be redistributed, so even a working result would need every
user to supply their own copy.

So the ARM path runs a **SoftDevice-free build of the same MeshCore source**
instead. It is not the flashed bytes and is not described as if it were — but it
is real `Mesh`, `Dispatcher`, `Packet`, Ed25519 and AES compiled for Cortex-M4,
which catches compiler, word-size and codegen differences the host build cannot.
`simple_repeater` contains no BLE code at all; the SoftDevice comes from the
Adafruit core's linker layout, not from anything the repeater calls.

All three paths share one chip model. `VirtualSX1262` runs in process for a
native node, and `radioserver` puts the same object behind a socket for QEMU and
Renode. That is deliberate: two models of one chip must agree for ever, and the
first time they drift, every comparison between an ARM node and an ESP32 node
measures our code rather than MeshCore's.

### Use native unless you need what emulation buys

Emulation closes a real gap. The native build compiles as an nRF52, so a bug
that only appears on ESP32 cannot appear there; an emulated node runs the
published ESP32 image, including all the platform code the native build never
executes. It is also the only way to test a firmware you have no source for.

It costs about a core per node, and the ceiling arrives without an error.
Measured on a twelve-core machine:

| emulated nodes | CPU each | total load | boot time | simulated time |
|---|---|---|---|---|
| 1 | ~100% of a core | 3.9 | ~10 s | keeps up |
| 4 | ~100% of a core | 5.1 | ~15 s | keeps up |
| 8 | 92–105% of a core | 15.0 | over 60 s | keeps up, but at the edge |

Memory is not the constraint — around 150 MB each, so 11 GB would hold about
seventy. Cores run out roughly seven times sooner. On twelve cores, **eight is
comfortable and ten is the practical ceiling**; a native scenario runs hundreds
of nodes on the same machine.

The failure mode deserves stating plainly, because nothing reports it: past the
ceiling there is no error. Boots stretch, and simulated time quietly falls
behind the wall clock — which reads as a mesh that has gone quiet rather than a
machine that has run out. The CPU and GPU figures in the menu bar are there to
make that visible before it bites.

Two further constraints follow from an emulator being in the loop. An emulated
node **runs on wall time**, so the engine cannot race the clock ahead as it does
for a native-only scenario. And two runs of one seed will not produce identical
ledgers, so the determinism the rest of the simulator guarantees does not hold
for a scenario containing one.

So: **native for anything about the mesh** — coverage, routing, loop detection,
sweeps, anything needing many nodes or repeatable numbers. **Emulated for
questions about the firmware as shipped** — does this published build work, does
this board's platform code behave, does a version we cannot compile still relay.
A scenario can mix the two, and the useful shape is usually one emulated node in
a native mesh.

## What it is for

Answering **why**, not just whether:

- Why did that packet miss? — terrain cut-through with the Fresnel zone and each
  diffracting edge's individual loss.
- What did the air actually look like? — waterfall, IQ, and a dechirped symbol
  view showing which of two colliding frames captured.
- Did that commit break relaying? — run half the repeaters on one firmware build
  and half on another, same traffic, and diff.
- Does my app work on a 40-node mesh? — attach a real companion client over TCP.

## Status

Early. Design settled, implementation starting. Decisions live in Plane project
**MSIM** as `ADR-0001` … `ADR-0009`, with a UX spec and the baseband technical
design alongside them.

## Building and running

Needs a GPU and a display. **It does not run on the dev VM** (virtual VGA, no
display) — develop there, run on a Mac. CI exercises the CPU reference path,
which per ADR-0004 is a maintained oracle rather than a fallback.

```bash
go test ./...        # CPU reference path
go run ./cmd/meshcoresim
```

## Honesty about the model

The simulator is **kinder than the air**. It does not model multipath or fading,
Doppler, oscillator ppm error, body loss, or non-LoRa interference beyond a flat
floor. Every one of those makes real links worse than simulated ones, so the bias
is one-directional — and it is stated in the UI, not buried here.

Sensitivity is validated against Semtech's published SX1262 figures: simulated
1% PER must land within ~2 dB across SF7–SF12. If that test fails, the chain is
wrong however convincing the waterfall looks.

## Wireshark

Captures carry every receiver's view of every frame, live over loopback UDP or
saved as pcapng. The MeshCore protocol itself is dissected by
[aaronb/wireshark-meshcore](https://github.com/aaronb/wireshark-meshcore)
(GPL-2.0-only), vendored in `tools/dissector/` beside our own metadata layer —
see `tools/dissector/README.md`.
