# Packaging emulation

What a released MeshBench has to ship, fetch or ask for so that emulated nodes
work without the person running it knowing any of this. Written while it was
fresh rather than reconstructed later, because most of it is not visible from
the code.

The target is: install MeshBench, pick a board in the firmware library, press
run. No environment variables, no build steps, no toolchains.

## What actually works today

Being exact, because "emulation works" is three different claims.

| path | state | what it proves |
|---|---|---|
| **ESP32, published `.bin`** | working, one board verified | the bytes off the flasher, on the mesh |
| **nRF52, published `.uf2`** | boots to the app's idle loop | the chain runs; not yet a node |
| **nRF52, SoftDevice-free build** | mesh stack runs on ARM | compiler, word size, ARM codegen |

Only the first is a node someone can use. `Generic_E22_sx1262` running published
v1.17.0 had its advert decoded by 38 nodes of a ScotMesh import. Everything
below assumes we ship that and treat the ARM paths as work in progress.

## The pieces, and where each comes from

Four things have to be present. Only two can be shipped.

| piece | source | ship it? | size |
|---|---|---|---|
| QEMU with our SX1262 | `A13xB0/qemu`, branch `meshbench-sx1262` | yes | ~69 MB |
| `radioserver` | `A13xB0/meshcore-native`, `bridge/radioserver.cpp` | yes | ~40 KB |
| Renode + our peripherals | upstream Renode 1.16, plus `tools/renode/` | probably not | ~200 MB |
| Nordic SoftDevice | Nordic, per-user | **no — licence** | 155 KB |

Board images and native builds are *not* in this list. They are downloaded on
demand from GitHub releases and cached, which already works and should stay that
way: they change every release and bundling them would date the installer.

### QEMU

A fork of Espressif's fork. Ours adds an SX1262 SPI device, a working GPIO
implementation (upstream's write handler is empty, and RadioLib drives NSS as an
ordinary GPIO rather than through the SPI controller), and machine properties
for the radio wiring.

Build with **`--enable-gcrypt`** or the `esp32` machine refuses to instantiate
with `unknown type 'misc.esp32.rsa'` — the RSA device is gated on gcrypt while
the machine references it unconditionally.

    ./configure --target-list=xtensa-softmmu --disable-werror --enable-slirp --enable-gcrypt

Licensing is GPLv2, so shipping the binary means shipping or offering the
source. That is ordinary, but it interacts with ADR-0001: MeshBench has no
licence chosen yet, and a GPL binary in the same installer is a decision to make
deliberately rather than discover.

Only `qemu-system-xtensa` is needed, and it finds its own data files by
resolving `/proc/self/exe`, so a symlink into the tools directory works and a
bare copy of the binary does not.

### radioserver

Small, and the important one architecturally. It owns the chip model that a
native node reaches in process, and both emulators reach it over a socket — one
`VirtualSX1262`, three ways in. Two models of one chip would have to agree for
ever, and the first time they drifted every comparison between an emulated node
and a native one would measure our code rather than MeshCore's.

Builds against `VirtualSX1262.cpp` from `meshcore-native`, no dependencies
beyond a C++17 compiler. It takes a Unix socket path, or `:port` for TCP —
QEMU uses the socket, Renode uses TCP because Mono's Unix socket support is not
worth betting a node on.

### Renode

Only needed for ARM, which is not shippable as a node yet. When it is, note that
Renode is a 200 MB Mono application and our contribution is four small files
loaded at runtime (`tools/renode/peripherals/*.cs`, `*.repl`). Bundling Renode
is a large step for a path that currently adds one architecture; asking people
to install Renode themselves and pointing at it is probably the right first
move.

### The SoftDevice

**Cannot be shipped, at all.** Nordic licenses it and does not permit
redistribution. Anyone running published nRF52 firmware supplies their own copy
— which they have, because it is on the board they bought, but extracting it is
not a step a casual user will complete.

This is the strongest argument for keeping `tools/armfw/` alive: a
SoftDevice-free build of the same MeshCore source is something we *can* hand
people, and it still catches compiler, word-size and ARM codegen differences the
host build cannot.

## How the application finds them

Already implemented, and the reasoning matters for packaging.

`lookupTool` searches, in order:

1. `MESHCORESIM_QEMU` / `MESHCORESIM_RADIO_SERVER`
2. beside the MeshBench binary
3. `~/.cache/meshcoresim/tools/`
4. `PATH`

Beside-the-binary is what an installed bundle should use. `PATH` is last and
nearly useless on a desktop: **a desktop application is not launched from a
shell**, so it inherits no useful `PATH` and no environment variables. That was
a real bug — emulation worked from a terminal and failed from the desktop, with
an error that read as a missing package.

The same shape as the native firmware cache, deliberately.

## What "simple to set up" should mean

In rough order of value:

1. **Ship QEMU and `radioserver` beside the binary.** Removes every manual step
   for the one path that works. A first run then needs only a download of the
   board image, which the firmware library already does.
2. **Say what is missing, in the UI, with the fix.** The error already names all
   four search locations rather than saying "not found", because "not found"
   sends people to their package manager for a QEMU build that will not do.
3. **Do not bundle firmware images.** They are per release and per board, and
   the catalogue already fetches and caches them.
4. **Leave Renode to the user for now,** and treat ARM as opt-in until a
   published nRF52 image actually runs.

## What to tell people about cost

This belongs in the installer or first-run notes, not buried in a README,
because the failure mode is silent.

An emulated node is a whole emulator taking **about one core** and ~150 MB.
Measured on twelve cores: eight is comfortable, ten is the practical ceiling, and
memory would allow around seventy — cores run out roughly seven times sooner.
Past the ceiling nothing reports an error. Boots stretch from ten seconds to over
a minute and simulated time falls behind the wall clock, which reads as a mesh
gone quiet rather than a machine that has run out. The CPU and GPU readout in the
menu bar exists for exactly this.

Two further constraints worth stating plainly to anyone designing a scenario:

- An emulated node **runs on wall time**. The engine cannot race the clock ahead
  as it does for native-only scenarios.
- Two runs of one seed **will not** produce identical ledgers. The determinism
  the rest of the simulator guarantees does not hold for a scenario containing
  an emulated node.

So the shape that works is a handful of emulated nodes in an otherwise native
mesh — usually one.

## Platform support

Everything here has only been run on Linux. Before packaging:

- **macOS** — QEMU builds, and the fork is not doing anything exotic; the
  Espressif fork is built for macOS upstream. Untested by us.
- **Windows** — untested. `radioserver` uses Unix sockets for the QEMU side and
  would need the TCP path used throughout, which already exists for Renode.
- **Renode** is cross-platform but heavy everywhere.

## Board coverage is the real long tail

One board working does not make fifty work. Emulation is verified **per board**,
not per MCU: the radio sits on a different SPI controller and different pins
from one board to the next, and a wrong pin does not fail loudly — it produces a
driver reporting no chip, which reads as a broken emulator rather than a wrong
number.

`scenario.EmulationVerified` is the one list to edit as boards are confirmed,
and the firmware library filters what it offers through it, so an unverified
board is never offered and cannot fail at play.

Of MeshCore's 87 variants: 40 ESP32 family, 36 nRF52840, 4 RP2040, 4 STM32, 3
ESP32-C6. We have one.
