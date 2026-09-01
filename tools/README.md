# tools

The programs that build, generate and drive things around MeshBench, but are
not part of the binary. Kept here rather than in `cmd/` because none of them
ships in a release: each is run by hand, by CI, or by `go generate`.

Reviewed for #243. Every tool below is still earning its place — either CI or
the build depends on it, or it is a firmware/hardware bring-up tool run by hand
that "not referenced from Go" does not make dead. Nothing was removed. If you
know one of these has actually fallen out of use, it is a one-line deletion plus
its row here.

## Generators — the build or CI depends on these

| tool | what it does | who runs it |
|---|---|---|
| `clientgen` | emits the closed enum sets both clients need, from the one place that defines them (`sets_gen.go`, `sets.py`) | CI checks they are current |
| `verbdoc` | regenerates `docs/scripting-verbs.md` from the verbs the tree registers | CI checks it is current |
| `flagdoc` | regenerates `docs/cli-reference.md` by building the binary and asking it for every command's flags | CI checks it is current |
| `licgen` | assembles the licence inventory the workbench embeds and the build checks | CI, and the build fails without it |
| `mockup` | renders the UX wireframes in `docs/ux` to PNG | by hand, when the wireframes change |
| `render` | produces the figures in `docs/output` from the **real** engine, not a mockup | by hand, when the figures change |
| `envgen` | turns building footprints into MeshBench environment tiles | by hand, when the environment data changes |
| `goldencap` | from a wideband capture to chip-rate baseband, and the analysis on it | by hand, for waveform validation |

## Firmware & emulator bring-up — run by hand

These build or drive firmware. They are shell/Python/Lua, not Go, and are not
called from anywhere in the tree on purpose: they are what a developer runs
against a MeshCore checkout or an emulator.

| tool | what it does |
|---|---|
| `native` | builds and runs the native host firmware backend |
| `armfw` | builds the SoftDevice-free nRF52840 image the Renode backend boots |
| `renode` | boots MeshCore's published binaries under Renode, the cross-check on the native build (ADR-0010) |
| `esp32` | the ESP32 half of the emulated backend — Espressif's QEMU fork, which Renode has no platform for |
| `platformio` | a PlatformIO post-build script that hands a freshly built image to a running MeshBench over the control socket; copied into a MeshCore checkout, so nothing here references it |
| `firmware-ab` | drives an A/B comparison of two firmware builds against the same traffic |

## Drivers & interop — run by hand

| tool | what it does |
|---|---|
| `soak` | drives a running workbench through sustained companion traffic and judges the receptions against the real ScotMesh network |
| `headless` | a harness for driving a headless session |
| `ble` | exposes a simulated node as a **real** BLE peripheral, so an unmodified companion app connects to it as it would to hardware |
| `dissector` | the Wireshark Lua dissector for MeshCore frames (and MeshBench's metadata layer beside it) |

## `tools/internal`

Shared code the tools use, said structurally — not a runnable tool.
