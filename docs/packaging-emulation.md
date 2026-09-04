> **Working note, last true on 17 August 2026.** Kept for the thinking in it, not maintained as a description of the code. Where this disagrees with the tree, the tree is right; the authority is `.github/workflows/package.yml`.

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
| **nRF52, published `.uf2`** | working, one board verified | the bytes off the flasher, on the mesh |
| **nRF52, SoftDevice-free build** | mesh stack runs on ARM | compiler, word size, ARM codegen |

Both published paths are nodes someone can use. `Generic_E22_sx1262` running
v1.17.0 had its advert decoded by 38 nodes of a ScotMesh import; `RAK_4631`
running v1.17.0 boots MBR → SoftDevice → MeshCore and puts a 127-byte advert on
the channel. One board each, of eighty-seven.

The third row is not a fallback for the second. `tools/armfw/` proves the mesh
stack compiles and runs on Cortex-M4; its radio is a stub, so it is not a node
and cannot become one without a real RadioLib build behind it. It earns its keep
as the thing we can hand to someone who has no SoftDevice.

## The pieces, and where each comes from

Four things have to be present. Only two can be shipped.

| piece | source | ship it? | size |
|---|---|---|---|
| QEMU with our SX1262 | `MeshBench/qemu`, branch `meshbench-main` | yes | ~17 MB packed, ~79 MB unpacked |
| `virtual-sx1262` | `MeshBench/virtual-sx1262`, the shared library | yes | ~100 KB |
| Renode with our SEVONPEND fix | `MeshBench/renode`, branch `meshbench` | yes | ~60 MB packed |
| Nordic SoftDevice | Nordic's own site, fetched at runtime | **no — fetched, not bundled** | 155 KB |

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
source - which the public `MeshBench/qemu` fork is. MeshBench itself is
GPL-3.0-or-later (`docs/licence.md`), and QEMU stays a separate process rather
than linked code, so the two sit in one bundle as aggregation.

Only `qemu-system-xtensa` is needed, and it finds its own data files by
resolving `/proc/self/exe`, so a symlink into the tools directory works and a
bare copy of the binary does not.

### The chip model

Small, and the important one architecturally. It owns the chip model that a
native node reaches in process, and both emulators reach it over a socket — one
`VirtualSX1262`, three ways in. Two models of one chip would have to agree for
ever, and the first time they drifted every comparison between an emulated node
and a native one would measure our code rather than MeshCore's.

Builds against `VirtualSX1262.cpp` from `meshcore-native`, no dependencies
beyond a C++17 compiler. It takes a Unix socket path, or `:port` for TCP, and
so does the SX1262 device: it parses whichever it is handed. A Unix socket is
the default where there is one, because it needs no port and cannot collide.
Renode always takes TCP, because Mono's Unix socket support is not worth
betting a node on, and so does QEMU on Windows, which has no Unix socket mingw
can reach.

### Renode

Needed for the nRF52 boards, and it has to be **our** build rather than an
upstream release: SEVONPEND is wrong in stock Renode, and without the fix the
firmware sleeps for ever with its wake condition already true. The fork's CI
publishes a portable package with the .NET runtime inside it, about 60 MB
packed, so nothing has to be installed to run it.

Our peripherals and platform files (`tools/renode/peripherals/*.cs`, `*.repl`)
are loaded at runtime from the tools directory rather than compiled in, which
keeps them in this repository where they are read alongside the boards they
describe.

Five of the six things that had to be fixed were in Renode rather than in the
firmware, and four of those were peripherals it does not model at all. Expect
the next board to need one or two more; `docs/repositories.md` says where the
patches live.

### The SoftDevice

**Decided: not bundled - fetched from Nordic's own site at runtime, and
cached. Not yet built.** Two separate questions used to sit behind this row,
and both are answered now (`docs/licence.md` has the full record, DevZone case
362437): whether *emulating* the SoftDevice for firmware testing is even
permitted at all - confirmed, it is, provided the end product runs on real
Nordic hardware and nothing about the binary is reverse-engineered or modified
- and *how a copy reaches the machine running MeshBench* without MeshBench
itself redistributing it. The answer to the second is the same shape as
everything else in this document: download it from the source at runtime and
cache it, the same way board images and native builds are already fetched two
rows up, rather than carry a copy in our own release archive. Nordic hosts it
themselves for anyone to fetch; MeshBench never becomes the one distributing
the file. The fetcher itself is not written yet - see the checklist below.

This does not replace `tools/armfw/`. A SoftDevice-free build of the same
MeshCore source still catches compiler, word-size and ARM codegen differences a
SoftDevice-linked image cannot isolate, and stays the thing to hand someone who
would rather not fetch anything from Nordic at all.

## How the application finds them

Already implemented, and the reasoning matters for packaging.

`lookupTool` searches, in order:

1. `MESHBENCH_QEMU` / `MESHBENCH_RADIO_SERVER`
2. beside the MeshBench binary
3. `~/.cache/meshbench/tools/`
4. `PATH`

Beside-the-binary is what an installed bundle should use. `PATH` is last and
nearly useless on a desktop: **a desktop application is not launched from a
shell**, so it inherits no useful `PATH` and no environment variables. That was
a real bug — emulation worked from a terminal and failed from the desktop, with
an error that read as a missing package.

The same shape as the native firmware cache, deliberately.

Step 3 is now also where the application *puts* things. The Resources page
carries a row per tool - `virtual-sx1262`, `qemu-system-xtensa` and `renode` -
with its size, its terms and a fetch that downloads from our own forks'
releases, verifies the digest, unpacks into that directory and links the binary
under the name the lookup asks for. The description of that directory as "where
the installer puts anything it downloads after the fact" was the gap: for a
source build no installer ever does.

## What "simple to set up" should mean

In rough order of value:

1. **Ship QEMU and the chip model beside the binary.** Removes every manual step
   for the one path that works. A first run then needs only a download of the
   board image, which the firmware library already does. *Done for a release
   bundle; and for everything else the Resources page now fetches all three
   into the tools directory, which is what a source checkout had no path to at
   all.*
2. **Say what is missing, in the UI, with the fix.** The error already names all
   four search locations rather than saying "not found", because "not found"
   sends people to their package manager for a QEMU build that will not do.
3. **Do not bundle firmware images.** They are per release and per board, and
   the catalogue already fetches and caches them.
4. **Leave Renode to the user for now,** and treat ARM as opt-in until a
   published nRF52 image actually runs. *The Linux portable package is now
   fetchable from the Resources page; macOS is not, because what the fork
   publishes there is a build tree rather than a portable package and nobody
   has started it.*
5. **Fetch the SoftDevice from Nordic's own site and cache it,** the way board
   images already are. Licensing is settled (`docs/licence.md`); this is the
   piece of work that turns "extracting it is not a step a casual user will
   complete" into a download the firmware library does for them.

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

Everything here has been run on Linux, and the other two are shipped rather
than proven:

- **macOS**: the fork cross-compiles an aarch64 build and the bundle carries
  it. Untested by us.
- **Windows**: the fork cross-compiles a mingw build and the zip carries it,
  with Renode's portable package and `libvirtualsx1262.dll`. A node there asks for
  `":0"` and reaches the radio model over TCP for both emulators, because
  Windows has no Unix socket mingw can reach. Untested by us, and what does
  *not* work is the runtime fetch: `internal/app/resource` reads ELF and Mach-O
  headers rather than PE, opens tars rather than zips, and installs by symlink.
- **arm64 Linux**: the fork builds it; nothing here has pinned or started it.
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
