> Moved out of `README.md` so the front page can be read in a minute.
> This is the detail behind it, and it is maintained.

# Real firmware: native and emulated

*Where the real ends and the simulation begins, and the two ways a node
can run.*


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

**From a release you do not have to do any of this.** The tarball, the dmg and
the Windows zip carry all three binaries beside the application, where it looks
for them first.

Building from source is the case below. An emulated node needs binaries that
are on no distribution: a QEMU carrying our SX1262 device for the ESP32 boards,
a Renode carrying the SEVONPEND fix for the nRF52 ones, and the radio model
both talk to.

**The Resources page will fetch all three.** They are rows there like
everything else the application downloads, with a size, a licence to read
first, and a state that says whether this platform has a build at all. What
lands goes into `~/.cache/meshbench/tools/`, which is where the search below
looks, so nothing else has to be set. From a script:

```
resource.list                                       # what is here, and what could be
resource.fetch {"kind":"toolchain","name":"radioserver"}
```

Doing it by hand still works, and is what to do on a platform the page says has
no build:

```bash
mkdir -p ~/.cache/meshbench/tools
ln -sf /path/to/qemu-system-xtensa ~/.cache/meshbench/tools/
cp /path/to/radioserver ~/.cache/meshbench/tools/
ln -sf /path/to/renode ~/.cache/meshbench/tools/          # nRF52 only
```

The Renode build comes from our fork's CI, which publishes a portable package
with the .NET runtime inside it, so nothing has to be installed to run it. Its
peripherals and platform files live in `tools/renode/` and are loaded from that
same tools directory.

A symlink is right for QEMU: it finds its own data files by resolving its real
path, so a bare copy of the binary will not run. `radioserver` builds from
`meshcore-native` with `./build.sh radioserver out` — it wants neither a
MeshCore checkout nor Crypto, only the chip model beside it.

Then open the firmware library, download a board image, and set a node's role to
it. The library only offers boards with verified wiring, so anything it lists
will start.

MeshBench searches, in order: `MESHBENCH_QEMU` and `MESHBENCH_RADIO_SERVER`,
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
| nRF52840 (Cortex-M4) | 36 | the published `.uf2`, byte for byte, under Renode | working, one board verified |
| RP2040 (Cortex-M0+) | 4 | — | not started |
| STM32 (Cortex-M4) | 4 | — | not started |
| ESP32-C6 (RISC-V) | 3 | — | not started |

**The nRF52 published binaries do run**, and getting there took a fork of the
emulator. Worth reading before anyone tries to reproduce it, because five of the
six faults were in things that looked like they already worked.

They are linked above a Nordic SoftDevice and make 119 SVC calls into it, and
the first mistake was ours: **erased flash**. Real nRF52 flash erases to `0xFF`
and Renode's memory starts at `0x00`, as did our own hex-to-binary converter.
The MBR decides whether a bootloader exists by testing words against
`0xFFFFFFFF`, so every such test answered "present, at address 0" and it
dereferenced a null pointer — 2.4 billion instructions going nowhere. MeshCore's
own OTAFIX bootloader package supplies the other half, because it carries the
UICR the MBR reads that address from.

Then four peripherals Renode does not model at all — TEMP, the CLOCK
calibration timer, SAADC and TWIM — each a handful of registers, each stalling
the boot somewhere different. And one it models incorrectly: **SEVONPEND**.
Firmware sets it so that an interrupt entering the pending state wakes `WFE`
even while that interrupt is disabled, then reads ISPR and handles the source in
thread mode. Renode asked whether a pending interrupt could be *taken*, which a
disabled one never can. That fix is in our fork; see `docs/repositories.md`.

The last one was chip select. Renode's SPI model calls `Transmit()` per byte and
never calls `FinishTransmission()`, so the chip never saw a transaction
boundary — and the chip model executes a command when chip select is *released*.
It took 3,320 bytes and executed none of them, the same count to the byte on
every run. RadioLib drives NSS as an ordinary GPIO, so the boundary was
available all along on P1.10, and the radio is on SPIM3 at `0x4002F000`, which
stock Renode does not model either.

With those, a RAK4631 boots MBR → SoftDevice → MeshCore, configures its radio,
and puts a 127-byte advert on the channel.

**The SoftDevice is fetched from Nordic's own site at runtime, not shipped by
us.** Nordic has confirmed in writing that emulating it for firmware testing is
not a licensing problem, provided the end product runs on real Nordic hardware
and the binary is neither reverse-engineered nor modified — `docs/licence.md`
has the full record. `tools/armfw/` stays regardless: a SoftDevice-free build
of the same MeshCore source is what we can hand to someone who would rather not
fetch anything from Nordic at all. It is not the flashed bytes and is not
described as if it were, and its radio is a stub, so it proves the mesh stack
compiles and runs on Cortex-M4 rather than that a node works.

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
